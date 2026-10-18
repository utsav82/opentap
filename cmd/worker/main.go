package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/nats-io/nats.go"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/opentap/opentap/internal/config"
	"github.com/opentap/opentap/internal/events"
	"github.com/opentap/opentap/internal/models"
	"github.com/opentap/opentap/internal/schemas"
	"github.com/opentap/opentap/internal/segments"
	"github.com/opentap/opentap/migrations"
	"github.com/opentap/opentap/pkg/cache"
	"github.com/opentap/opentap/pkg/db"
	"github.com/opentap/opentap/pkg/queue"
)

const maxRetryDelay = 30 * time.Second

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

	if err := db.Migrate(ctx, pool, migrations.FS); err != nil {
		log.Fatal().Err(err).Msg("failed to run migrations")
	}

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
	segmentRepo := segments.NewRepository(pool)
	tracker := schemas.NewTracker(schemas.NewRepository(pool), redisClient)

	if err := eventRepo.SetRetention(ctx, cfg.EventRetentionDays); err != nil {
		log.Fatal().Err(err).Msg("failed to set event retention")
	}

	go runIngestionWorker(ctx, cfg, natsClient, eventRepo, tracker)
	go runSchemaFlusher(ctx, cfg, tracker)
	go runSegmentWorker(ctx, cfg, segmentRepo, redisClient, segments.NewNotifier(cfg.WebhookSecret))

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

func runIngestionWorker(ctx context.Context, cfg *config.Config, natsClient *queue.NATSClient, eventRepo events.Repository, tracker *schemas.Tracker) {
	log.Info().Int("concurrency", cfg.WorkerConcurrency).Msg("starting ingestion worker")

	// Each queue subscription runs its own handler goroutine. Redelivery is unlimited so
	// outages never drop events; handleMessage decides when to dead-letter.
	for range max(cfg.WorkerConcurrency, 1) {
		sub, err := natsClient.JetStream.QueueSubscribe(
			"events.ingest.>",
			"ingestion-worker",
			func(msg *nats.Msg) {
				handleMessage(ctx, cfg, natsClient, eventRepo, tracker, msg)
			},
			nats.Durable("ingestion-worker"),
			nats.ManualAck(),
			nats.AckWait(30*time.Second),
			nats.MaxDeliver(-1),
		)
		if err != nil {
			log.Fatal().Err(err).Msg("failed to subscribe to NATS")
		}
		defer sub.Unsubscribe()
	}

	<-ctx.Done()
	log.Info().Msg("ingestion worker stopped")
}

func handleMessage(ctx context.Context, cfg *config.Config, natsClient *queue.NATSClient, eventRepo events.Repository, tracker *schemas.Tracker, msg *nats.Msg) {
	var event models.Event
	if err := json.Unmarshal(msg.Data, &event); err != nil {
		log.Error().Err(err).Msg("failed to unmarshal event")
		sendToDLQ(natsClient, msg, "unmarshal_error", err.Error())
		msg.Ack()
		return
	}

	if event.UserID == "" || event.EventName == "" {
		log.Error().Msg("event missing required fields")
		sendToDLQ(natsClient, msg, "validation_error", "missing required fields")
		msg.Ack()
		return
	}

	if err := eventRepo.Ingest(ctx, &event); err != nil {
		delivered := uint64(1)
		if meta, _ := msg.Metadata(); meta != nil {
			delivered = meta.NumDelivered
		}
		log.Error().Err(err).Uint64("attempt", delivered).Msg("failed to ingest event")

		if isPermanent(err) && delivered >= uint64(cfg.DLQMaxRetries) {
			sendToDLQ(natsClient, msg, "insert_error", err.Error())
			msg.Ack()
			return
		}
		msg.NakWithDelay(min(time.Second<<min(delivered, 5), maxRetryDelay))
		return
	}

	if err := tracker.Track(ctx, &event); err != nil {
		log.Error().Err(err).Msg("failed to track event schema")
	}

	msg.Ack()
	log.Debug().
		Str("event_id", event.ID.String()).
		Str("event_name", event.EventName).
		Str("user_id", event.UserID).
		Msg("event processed")
}

// isPermanent reports database errors caused by the event's data, which retrying cannot fix.
func isPermanent(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && (strings.HasPrefix(pgErr.Code, "22") || strings.HasPrefix(pgErr.Code, "23"))
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

func runSchemaFlusher(ctx context.Context, cfg *config.Config, tracker *schemas.Tracker) {
	ticker := time.NewTicker(cfg.SchemaFlushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			// Final flush
			flushSchemas(context.Background(), tracker)
			return
		case <-ticker.C:
			flushSchemas(ctx, tracker)
		}
	}
}

func flushSchemas(ctx context.Context, tracker *schemas.Tracker) {
	count, err := tracker.Flush(ctx)
	if err != nil {
		log.Error().Err(err).Int("count", count).Msg("failed to flush schemas")
		return
	}
	if count > 0 {
		log.Info().Int("count", count).Msg("flushed schemas to database")
	}
}

func runSegmentWorker(ctx context.Context, cfg *config.Config, segmentRepo segments.Repository, redisClient *cache.RedisClient, notifier *segments.Notifier) {
	ticker := time.NewTicker(cfg.SegmentInterval)
	defer ticker.Stop()

	log.Info().Dur("interval", cfg.SegmentInterval).Msg("starting segment worker")

	for {
		processSegments(ctx, cfg, segmentRepo, redisClient, notifier)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func processSegments(ctx context.Context, cfg *config.Config, segmentRepo segments.Repository, redisClient *cache.RedisClient, notifier *segments.Notifier) {
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
		change, err := segmentRepo.Recompute(ctx, seg)
		if err != nil {
			log.Error().Err(err).Str("segment", seg.Name).Msg("failed to evaluate segment")
			continue
		}
		redisClient.Set(ctx, segments.CountCacheKey(seg.ID), change.UserCount, cfg.RedisSegmentCacheTTL)
		if err := notifier.Notify(ctx, seg, change); err != nil {
			log.Error().Err(err).Str("segment", seg.Name).Msg("failed to deliver segment webhook")
		}
		log.Info().
			Str("segment", seg.Name).
			Int64("user_count", change.UserCount).
			Int("added", len(change.Added)).
			Int("removed", len(change.Removed)).
			Msg("segment evaluated")
	}

	log.Info().
		Int("segments", len(allSegments)).
		Dur("duration", time.Since(start)).
		Msg("segment evaluation complete")
}
