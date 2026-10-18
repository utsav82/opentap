package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/opentap/opentap/internal/analytics"
	"github.com/opentap/opentap/internal/auth"
	"github.com/opentap/opentap/internal/config"
	"github.com/opentap/opentap/internal/events"
	"github.com/opentap/opentap/internal/middleware"
	"github.com/opentap/opentap/internal/projects"
	"github.com/opentap/opentap/internal/schemas"
	"github.com/opentap/opentap/internal/segments"
	"github.com/opentap/opentap/internal/users"
	"github.com/opentap/opentap/migrations"
	"github.com/opentap/opentap/pkg/cache"
	"github.com/opentap/opentap/pkg/db"
	"github.com/opentap/opentap/pkg/queue"
	"github.com/opentap/opentap/pkg/response"
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
	if cfg.AdminToken == "" {
		log.Fatal().Msg("ADMIN_TOKEN is required")
	}

	ctx := context.Background()

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

	// Connect NATS (publish only)
	natsClient, err := queue.NewNATSClient(cfg.NatsURL, cfg.NatsStreamName, cfg.NatsMaxBytes)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to connect to NATS")
	}
	defer natsClient.Close()
	log.Info().Msg("connected to NATS")

	// Init repositories
	eventRepo := events.NewRepository(pool)
	userRepo := users.NewRepository(pool)
	segmentRepo := segments.NewRepository(pool)
	schemaRepo := schemas.NewRepository(pool)
	analyticsRepo := analytics.NewRepository(pool)
	projectRepo := projects.NewRepository(pool)
	authRepo := auth.NewRepository(pool, redisClient, cfg.RedisAPIKeyTTL)

	// Init services
	eventSvc := events.NewService(eventRepo, natsClient)
	userSvc := users.NewService(userRepo)
	segmentSvc := segments.NewService(segmentRepo, redisClient)
	schemaSvc := schemas.NewService(schemaRepo)
	analyticsSvc := analytics.NewService(analyticsRepo)
	projectSvc := projects.NewService(projectRepo, authRepo)

	// Init handlers
	eventHandler := events.NewHandler(eventSvc)
	userHandler := users.NewHandler(userSvc, eventSvc)
	segmentHandler := segments.NewHandler(segmentSvc)
	schemaHandler := schemas.NewHandler(schemaSvc)
	analyticsHandler := analytics.NewHandler(analyticsSvc)
	projectHandler := projects.NewHandler(projectSvc)

	// Auth middleware
	authMiddleware := auth.NewMiddleware(authRepo)

	// Rate limiter
	rateLimiter := middleware.NewRateLimiter(redisClient, cfg.APIRateLimit, cfg.APIRateWindow)

	// Build router
	r := chi.NewRouter()

	// Global middleware
	r.Use(middleware.Recovery)
	r.Use(middleware.RequestLogging)
	r.Use(middleware.CORS)
	r.Use(chimiddleware.RequestSize(5 << 20))

	// Health check (no auth)
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		response.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	// Admin routes
	r.Route("/v1/projects", func(r chi.Router) {
		r.Use(auth.AdminOnly(cfg.AdminToken))
		r.Post("/", projectHandler.CreateProject)
		r.Get("/", projectHandler.ListProjects)
		r.Get("/{id}", projectHandler.GetProject)
		r.Post("/{id}/keys", projectHandler.CreateKey)
		r.Get("/{id}/keys", projectHandler.ListKeys)
		r.Delete("/{id}/keys/{key_id}", projectHandler.RevokeKey)
	})

	// Authenticated API routes
	r.Group(func(r chi.Router) {
		r.Use(authMiddleware.Authenticate)
		r.Use(rateLimiter.Limit)

		// Ingestion (any key, including ingest-only keys used by client SDKs)
		r.Post("/v1/events", eventHandler.TrackEvent)
		r.Post("/v1/events/batch", eventHandler.TrackBatch)
		r.Post("/v1/identify", userHandler.Identify)

		r.Group(func(r chi.Router) {
			r.Use(auth.FullAccess)

			// Events
			r.Get("/v1/events", eventHandler.ListEvents)
			r.Get("/v1/events/{id}", eventHandler.GetEvent)

			// Users
			r.Get("/v1/users", userHandler.ListUsers)
			r.Get("/v1/users/{id}", userHandler.GetUser)
			r.Get("/v1/users/{id}/events", userHandler.GetUserEvents)

			// Segments
			r.Post("/v1/segments", segmentHandler.CreateSegment)
			r.Get("/v1/segments", segmentHandler.ListSegments)
			r.Get("/v1/segments/{id}", segmentHandler.GetSegment)
			r.Put("/v1/segments/{id}", segmentHandler.UpdateSegment)
			r.Delete("/v1/segments/{id}", segmentHandler.DeleteSegment)
			r.Get("/v1/segments/{id}/users", segmentHandler.GetSegmentUsers)
			r.Get("/v1/segments/{id}/export", segmentHandler.ExportSegmentUsers)

			// Schemas
			r.Get("/v1/schemas", schemaHandler.ListSchemas)
			r.Get("/v1/schemas/{event_name}", schemaHandler.GetSchema)

			// Analytics
			r.Get("/v1/analytics/events/count", analyticsHandler.EventCountOverTime)
			r.Get("/v1/analytics/events/top", analyticsHandler.TopEvents)
			r.Get("/v1/analytics/users/active", analyticsHandler.DAU)
			r.Get("/v1/analytics/funnel", analyticsHandler.Funnel)
			r.Get("/v1/analytics/retention", analyticsHandler.Retention)
		})
	})

	// Start server
	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.APIPort),
		Handler: r,
	}

	// Graceful shutdown
	go func() {
		log.Info().Int("port", cfg.APIPort).Msg("API server starting")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal().Err(err).Msg("server failed")
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Info().Msg("shutting down server...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Error().Err(err).Msg("server shutdown failed")
	}

	log.Info().Msg("server stopped")
}
