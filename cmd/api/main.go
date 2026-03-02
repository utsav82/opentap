package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/opentap/opentap/internal/analytics"
	"github.com/opentap/opentap/internal/auth"
	"github.com/opentap/opentap/internal/config"
	"github.com/opentap/opentap/internal/events"
	"github.com/opentap/opentap/internal/middleware"
	"github.com/opentap/opentap/internal/models"
	"github.com/opentap/opentap/internal/projects"
	"github.com/opentap/opentap/internal/schemas"
	"github.com/opentap/opentap/internal/segments"
	"github.com/opentap/opentap/internal/users"
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

	ctx := context.Background()

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
	userSvc := users.NewService(userRepo, redisClient)
	segmentSvc := segments.NewService(segmentRepo, redisClient)
	schemaSvc := schemas.NewService(schemaRepo)
	analyticsSvc := analytics.NewService(analyticsRepo)
	projectSvc := projects.NewService(projectRepo)

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

	// Health check (no auth)
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		response.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	// Admin routes (no API key auth for project creation — bootstrap)
	r.Route("/v1/projects", func(r chi.Router) {
		r.Post("/", projectHandler.CreateProject)
		r.Get("/", projectHandler.ListProjects)
		r.Get("/{id}", projectHandler.GetProject)

		// API key management
		r.Post("/{id}/keys", func(w http.ResponseWriter, r *http.Request) {
			projectID, err := uuid.Parse(chi.URLParam(r, "id"))
			if err != nil {
				response.Error(w, http.StatusBadRequest, "bad_request", "Invalid project ID")
				return
			}

			var req models.CreateAPIKeyRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				response.Error(w, http.StatusBadRequest, "bad_request", "Invalid JSON body")
				return
			}

			rawKey, apiKey, err := authRepo.CreateKey(r.Context(), projectID, req.Label)
			if err != nil {
				response.Error(w, http.StatusInternalServerError, "internal_error", "Failed to create API key")
				return
			}

			response.JSON(w, http.StatusCreated, map[string]interface{}{
				"id":         apiKey.ID,
				"key":        rawKey,
				"key_prefix": apiKey.KeyPrefix,
				"label":      apiKey.Label,
				"created_at": apiKey.CreatedAt,
			})
		})

		r.Delete("/{id}/keys/{key_id}", func(w http.ResponseWriter, r *http.Request) {
			projectID, err := uuid.Parse(chi.URLParam(r, "id"))
			if err != nil {
				response.Error(w, http.StatusBadRequest, "bad_request", "Invalid project ID")
				return
			}
			keyID, err := uuid.Parse(chi.URLParam(r, "key_id"))
			if err != nil {
				response.Error(w, http.StatusBadRequest, "bad_request", "Invalid key ID")
				return
			}

			if err := authRepo.RevokeKey(r.Context(), projectID, keyID); err != nil {
				response.Error(w, http.StatusNotFound, "not_found", "API key not found or already revoked")
				return
			}

			w.WriteHeader(http.StatusNoContent)
		})
	})

	// Authenticated API routes
	r.Group(func(r chi.Router) {
		r.Use(authMiddleware.Authenticate)
		r.Use(rateLimiter.Limit)

		// Events
		r.Post("/v1/events", eventHandler.TrackEvent)
		r.Post("/v1/events/batch", eventHandler.TrackBatch)
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

		// Schemas
		r.Get("/v1/schemas", schemaHandler.ListSchemas)
		r.Get("/v1/schemas/{event_name}", schemaHandler.GetSchema)

		// Analytics
		r.Get("/v1/analytics/events/count", analyticsHandler.EventCountOverTime)
		r.Get("/v1/analytics/events/top", analyticsHandler.TopEvents)
		r.Get("/v1/analytics/users/active", analyticsHandler.DAU)
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
