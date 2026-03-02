package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/opentap/opentap/internal/config"
	"github.com/opentap/opentap/internal/events"
	"github.com/opentap/opentap/internal/models"
	"github.com/opentap/opentap/internal/schemas"
	"github.com/opentap/opentap/internal/segments"
	"github.com/opentap/opentap/internal/users"
	"github.com/opentap/opentap/pkg/cache"
	"github.com/opentap/opentap/pkg/db"
	"github.com/opentap/opentap/pkg/queue"
)

func main() {
	// Init zerolog
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})

	// Load config
	cfg, err := config.Load()
	if err != nil {
		log.Fatal().Err(err).Msg("failed to load config")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Connect PostgreSQL
	pool, err := db.NewPool(ctx, cfg.DatabaseURL, cfg.DBMaxConns, cfg.DBMinConns)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to connect to PostgreSQL")
	}
	defer pool.Close()
	log.Info().Msg("connected to PostgreSQL")

	// Connect Redis
	redisClient, err := cache.NewRedisClient(cfg.RedisURL)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to connect to Redis")
	}
	defer redisClient.Close()
	log.Info().Msg("connected to Redis")

	// Connect NATS
	natsClient, err := queue.NewNATSClient(cfg.NatsURL, cfg.NatsStreamName, cfg.NatsMaxBytes)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to connect to NATS")
	}
	defer natsClient.Close()
	log.Info().Msg("connected to NATS")

	// Init repositories
	eventRepo := events.NewRepository(pool)
	userRepo := users.NewRepository(pool)
	schemaRepo := schemas.NewRepository(pool)
	segmentRepo := segments.NewRepository(pool)

	// Schema buffer
	schemaBuffer := NewSchemaBuffer()

	// Start ingestion worker
	go runIngestionWorker(ctx, cfg, natsClient, eventRepo, userRepo, schemaBuffer)

	// Start schema flusher
	go runSchemaFlusher(ctx, cfg, schemaRepo, schemaBuffer)

	// Start segment worker
	evaluator := segments.NewEvaluator()
	go runSegmentWorker(ctx, cfg, segmentRepo, evaluator, redisClient)

	// Wait for shutdown signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Info().Msg("shutting down worker...")
	cancel()

	// Give goroutines time to finish
	time.Sleep(2 * time.Second)
	log.Info().Msg("worker stopped")
}

// SchemaBuffer accumulates schemas in memory before flushing to DB.
type SchemaBuffer struct {
	mu      sync.Mutex
	schemas map[string]*models.EventSchema // key: project_id:event_name:property_name
}

func NewSchemaBuffer() *SchemaBuffer {
	return &SchemaBuffer{
		schemas: make(map[string]*models.EventSchema),
	}
}

func (sb *SchemaBuffer) Add(schema *models.EventSchema) {
	sb.mu.Lock()
	defer sb.mu.Unlock()

	key := fmt.Sprintf("%s:%s:%s", schema.ProjectID, schema.EventName, schema.PropertyName)
	if existing, ok := sb.schemas[key]; ok {
		existing.DistinctValuesCount++
		if schema.LastSeen.After(existing.LastSeen) {
			existing.LastSeen = schema.LastSeen
		}
	} else {
		sb.schemas[key] = schema
	}
}

func (sb *SchemaBuffer) Flush() []*models.EventSchema {
	sb.mu.Lock()
	defer sb.mu.Unlock()

	result := make([]*models.EventSchema, 0, len(sb.schemas))
	for _, s := range sb.schemas {
		result = append(result, s)
	}
	sb.schemas = make(map[string]*models.EventSchema)
	return result
}

func runIngestionWorker(ctx context.Context, cfg *config.Config, natsClient *queue.NATSClient, eventRepo events.Repository, userRepo users.Repository, schemaBuffer *SchemaBuffer) {
	log.Info().Msg("starting ingestion worker")

	// Subscribe to events.ingest.> with durable consumer
	sub, err := natsClient.JetStream.Subscribe(
		"events.ingest.>",
		func(msg *nats.Msg) {
			handleMessage(ctx, cfg, natsClient, eventRepo, userRepo, schemaBuffer, msg)
		},
		nats.Durable("ingestion-worker"),
		nats.ManualAck(),
		nats.AckWait(30*time.Second),
		nats.MaxDeliver(cfg.DLQMaxRetries),
	)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to subscribe to NATS")
	}
	defer sub.Unsubscribe()

	<-ctx.Done()
	log.Info().Msg("ingestion worker stopped")
}

func handleMessage(ctx context.Context, cfg *config.Config, natsClient *queue.NATSClient, eventRepo events.Repository, userRepo users.Repository, schemaBuffer *SchemaBuffer, msg *nats.Msg) {
	var event models.Event
	if err := json.Unmarshal(msg.Data, &event); err != nil {
		log.Error().Err(err).Msg("failed to unmarshal event")
		// Send to DLQ on unmarshal error
		sendToDLQ(natsClient, msg, "unmarshal_error", err.Error())
		msg.Ack()
		return
	}

	// Validate required fields
	if event.UserID == "" || event.EventName == "" {
		log.Error().Msg("event missing required fields")
		sendToDLQ(natsClient, msg, "validation_error", "missing required fields")
		msg.Ack()
		return
	}

	// Insert event into TimescaleDB
	if err := eventRepo.Insert(ctx, &event); err != nil {
		log.Error().Err(err).Msg("failed to insert event")
		metadata, _ := msg.Metadata()
		if metadata != nil && metadata.NumDelivered >= uint64(cfg.DLQMaxRetries) {
			sendToDLQ(natsClient, msg, "insert_error", err.Error())
			msg.Ack()
		} else {
			msg.Nak()
		}
		return
	}

	// Upsert user
	if err := userRepo.Upsert(ctx, event.ProjectID, event.UserID, event.Timestamp); err != nil {
		log.Error().Err(err).Str("user_id", event.UserID).Msg("failed to upsert user")
	}

	// Increment event count
	if err := userRepo.IncrementEventCount(ctx, event.ProjectID, event.UserID); err != nil {
		log.Error().Err(err).Str("user_id", event.UserID).Msg("failed to increment event count")
	}

	// Accumulate schemas
	if event.Properties != nil {
		now := time.Now().UTC()
		extractedSchemas := schemas.ExtractSchemas(event.ProjectID, event.EventName, event.Properties, now)
		for _, s := range extractedSchemas {
			schemaBuffer.Add(s)
		}
	}

	msg.Ack()
	log.Debug().
		Str("event_id", event.ID.String()).
		Str("event_name", event.EventName).
		Str("user_id", event.UserID).
		Msg("event processed")
}

func sendToDLQ(natsClient *queue.NATSClient, msg *nats.Msg, errCode, errMsg string) {
	dlqData := map[string]interface{}{
		"original_subject": msg.Subject,
		"error_code":       errCode,
		"error_message":    errMsg,
		"original_data":    string(msg.Data),
		"timestamp":        time.Now().UTC(),
	}
	data, _ := json.Marshal(dlqData)
	if err := natsClient.Publish("events.dlq", data); err != nil {
		log.Error().Err(err).Msg("failed to publish to DLQ")
	}
}

func runSchemaFlusher(ctx context.Context, cfg *config.Config, schemaRepo schemas.Repository, schemaBuffer *SchemaBuffer) {
	ticker := time.NewTicker(cfg.SchemaFlushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			// Final flush
			flushSchemas(ctx, schemaRepo, schemaBuffer)
			return
		case <-ticker.C:
			flushSchemas(ctx, schemaRepo, schemaBuffer)
		}
	}
}

func flushSchemas(ctx context.Context, schemaRepo schemas.Repository, schemaBuffer *SchemaBuffer) {
	batch := schemaBuffer.Flush()
	if len(batch) == 0 {
		return
	}

	if err := schemaRepo.UpsertBatch(ctx, batch); err != nil {
		log.Error().Err(err).Int("count", len(batch)).Msg("failed to flush schemas")
		return
	}

	log.Info().Int("count", len(batch)).Msg("flushed schemas to database")
}

func runSegmentWorker(ctx context.Context, cfg *config.Config, segmentRepo segments.Repository, evaluator *segments.Evaluator, redisClient *cache.RedisClient) {
	ticker := time.NewTicker(cfg.SegmentInterval)
	defer ticker.Stop()

	log.Info().Dur("interval", cfg.SegmentInterval).Msg("starting segment worker")

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			processSegments(ctx, segmentRepo, evaluator, redisClient)
		}
	}
}

func processSegments(ctx context.Context, segmentRepo segments.Repository, evaluator *segments.Evaluator, redisClient *cache.RedisClient) {
	start := time.Now()
	allSegments, err := segmentRepo.ListAll(ctx)
	if err != nil {
		log.Error().Err(err).Msg("failed to list segments")
		return
	}

	if len(allSegments) == 0 {
		return
	}

	log.Info().Int("count", len(allSegments)).Msg("evaluating segments")

	for _, seg := range allSegments {
		evaluateSegment(ctx, seg, segmentRepo, evaluator, redisClient)
	}

	log.Info().
		Int("segments", len(allSegments)).
		Dur("duration", time.Since(start)).
		Msg("segment evaluation complete")
}

func evaluateSegment(ctx context.Context, seg *models.Segment, segmentRepo segments.Repository, evaluator *segments.Evaluator, redisClient *cache.RedisClient) {
	query, args, err := evaluator.BuildQuery(seg.ProjectID, seg.Conditions)
	if err != nil {
		log.Error().Err(err).Str("segment", seg.Name).Msg("failed to build segment query")
		return
	}

	// We need to use the repository's pool for querying.
	// Since the repo has a pool internally, we'll add a method to execute the evaluator query.
	userIDs, err := executeSegmentQuery(ctx, segmentRepo, query, args)
	if err != nil {
		log.Error().Err(err).Str("segment", seg.Name).Msg("failed to execute segment query")
		return
	}

	// Replace users
	if err := segmentRepo.ReplaceUsers(ctx, seg.ID, userIDs); err != nil {
		log.Error().Err(err).Str("segment", seg.Name).Msg("failed to replace segment users")
		return
	}

	// Update cache
	if redisClient != nil {
		countKey := fmt.Sprintf("segment:%s:count", seg.ID.String())
		countData, _ := json.Marshal(int64(len(userIDs)))
		redisClient.Set(ctx, countKey, string(countData), 10*time.Minute)

		usersKey := fmt.Sprintf("segment:%s:users", seg.ID.String())
		usersData, _ := json.Marshal(userIDs)
		redisClient.Set(ctx, usersKey, string(usersData), 10*time.Minute)
	}

	log.Info().
		Str("segment", seg.Name).
		Int("user_count", len(userIDs)).
		Msg("segment evaluated")
}

// executeSegmentQuery is a workaround to execute the evaluator-generated query.
// We need access to the underlying pool, so we use a type assertion approach.
func executeSegmentQuery(ctx context.Context, segmentRepo segments.Repository, query string, args []interface{}) ([]uuid.UUID, error) {
	// Use the QueryExecutor interface if available
	executor, ok := segmentRepo.(segments.QueryExecutor)
	if !ok {
		return nil, fmt.Errorf("segment repository does not implement QueryExecutor")
	}
	return executor.ExecuteQuery(ctx, query, args)
}
