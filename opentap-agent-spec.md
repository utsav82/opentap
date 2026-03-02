# OpenTap — AI Agent Build Specification

> You are building OpenTap, an open-source, self-hosted event analytics platform. This document is the complete specification. Do not deviate from it. Build exactly what is described, in the order specified.

---

## TABLE OF CONTENTS

1. Project Overview
2. Tech Stack & Versions
3. Project Structure
4. Environment Variables
5. Docker Compose (Dev)
6. Database Schema (SQL)
7. NATS Configuration
8. Golang Packages & Dependencies
9. Internal Package Contracts (Interfaces)
10. API Server (`cmd/api`)
11. Worker (`cmd/worker`)
12. API Endpoints — Full Specification
13. Redis Cache Keys & Strategy
14. Error Handling
15. Build Order (Milestones with exact steps)
16. Testing Requirements
17. Docker Compose (Production)
18. Dashboard (React)

---

## 1. PROJECT OVERVIEW

OpenTap tracks user events from any HTTP client, builds user profiles, lets operators define audience segments, and visualizes everything in a React dashboard. It is self-hosted — the operator owns all data.

**Core flows:**

- **Write path:** HTTP Client → Golang API → NATS JetStream → Ingestion Worker → TimescaleDB + PostgreSQL
- **Segmentation path:** Segment Worker (scheduled) → queries TimescaleDB → writes `segment_users` → caches in Redis
- **Read path:** Dashboard → Golang API → TimescaleDB + Redis → response

There is no SDK. The REST API is the only interface for ingesting data.

---

## 2. TECH STACK & VERSIONS

| Dependency | Version | Notes |
|---|---|---|
| Go | 1.22+ | Use Go modules |
| PostgreSQL | 16 | Base relational store |
| TimescaleDB | 2.14+ (pg16) | Extension on same PostgreSQL instance |
| NATS Server | 2.10+ | With JetStream enabled |
| Redis | 7+ | Cache only, not persistent storage |
| Caddy | 2.7+ | Reverse proxy, auto HTTPS |
| React | 18+ | Dashboard frontend |
| Node.js | 20 LTS | For dashboard build |
| Docker | 24+ | Containerization |

### Go Dependencies (go.mod)

```
github.com/jackc/pgx/v5          // PostgreSQL driver + pool
github.com/nats-io/nats.go       // NATS client
github.com/redis/go-redis/v9     // Redis client
github.com/go-chi/chi/v5         // HTTP router
github.com/google/uuid           // UUID generation
github.com/rs/zerolog             // Structured logging
github.com/go-playground/validator/v10  // Struct validation
```

Do NOT use `database/sql`. Use `pgx` directly with `pgxpool`.
Do NOT use Gin, Echo, or Fiber. Use `chi`.
Do NOT use logrus or zap. Use `zerolog`.

---

## 3. PROJECT STRUCTURE

```
opentap/
├── cmd/
│   ├── api/
│   │   └── main.go                 # API server entrypoint
│   └── worker/
│       └── main.go                 # Worker entrypoint (ingestion + segment)
├── internal/
│   ├── config/
│   │   └── config.go               # Env var loading into Config struct
│   ├── models/
│   │   └── models.go               # All struct definitions
│   ├── events/
│   │   ├── repository.go           # DB queries for events
│   │   ├── service.go              # Business logic
│   │   └── handler.go              # HTTP handlers
│   ├── users/
│   │   ├── repository.go
│   │   ├── service.go
│   │   └── handler.go
│   ├── segments/
│   │   ├── repository.go
│   │   ├── service.go
│   │   ├── handler.go
│   │   └── evaluator.go            # Segment condition evaluation engine
│   ├── analytics/
│   │   ├── repository.go
│   │   ├── service.go
│   │   └── handler.go
│   ├── schemas/
│   │   ├── repository.go
│   │   ├── service.go
│   │   └── handler.go
│   ├── projects/
│   │   ├── repository.go
│   │   ├── service.go
│   │   └── handler.go
│   ├── auth/
│   │   ├── middleware.go            # API key auth middleware
│   │   └── repository.go           # API key lookup (Redis → PostgreSQL fallback)
│   └── middleware/
│       ├── ratelimit.go            # Redis sliding window rate limiter
│       ├── logging.go              # Request/response logging
│       └── recovery.go             # Panic recovery
├── pkg/
│   ├── db/
│   │   └── postgres.go             # pgxpool init + health check
│   ├── queue/
│   │   └── nats.go                 # NATS connect + JetStream stream/consumer setup
│   ├── cache/
│   │   └── redis.go                # Redis client init + generic Get/Set/Delete helpers
│   └── response/
│       └── json.go                 # Standardized JSON response helpers
├── migrations/
│   ├── 001_create_projects.sql
│   ├── 002_create_users.sql
│   ├── 003_create_events.sql
│   ├── 004_create_event_schemas.sql
│   ├── 005_create_segments.sql
│   └── 006_create_segment_users.sql
├── dashboard/                       # React app (Milestone 6)
├── docker-compose.yml               # Production
├── docker-compose.dev.yml           # Dev (dependencies only)
├── Dockerfile.api
├── Dockerfile.worker
├── Makefile
├── .env.example
└── README.md
```

Every domain package (`events`, `users`, `segments`, `analytics`, `schemas`, `projects`) follows the same pattern: `repository.go` (data access), `service.go` (business logic), `handler.go` (HTTP layer). No exceptions.

---

## 4. ENVIRONMENT VARIABLES

```env
# PostgreSQL
DATABASE_URL=postgres://opentap:opentap@localhost:5432/opentap?sslmode=disable
DB_MAX_CONNS=25
DB_MIN_CONNS=5

# NATS
NATS_URL=nats://localhost:4222
NATS_STREAM_NAME=EVENTS
NATS_MAX_BYTES=1073741824

# Redis
REDIS_URL=redis://localhost:6379/0
REDIS_API_KEY_TTL=5m
REDIS_USER_CACHE_TTL=60s
REDIS_SEGMENT_CACHE_TTL=10m

# API Server
API_PORT=8080
API_RATE_LIMIT=100
API_RATE_WINDOW=60s

# Worker
SEGMENT_INTERVAL=5m
SCHEMA_FLUSH_INTERVAL=30s
DLQ_MAX_RETRIES=5

# Data Retention
EVENT_RETENTION_DAYS=90

# Caddy (production only)
DOMAIN=opentap.example.com
```

Load these in `internal/config/config.go` into a typed struct:

```go
type Config struct {
    DatabaseURL         string        `env:"DATABASE_URL,required"`
    DBMaxConns          int32         `env:"DB_MAX_CONNS" envDefault:"25"`
    DBMinConns          int32         `env:"DB_MIN_CONNS" envDefault:"5"`
    NatsURL             string        `env:"NATS_URL" envDefault:"nats://localhost:4222"`
    NatsStreamName      string        `env:"NATS_STREAM_NAME" envDefault:"EVENTS"`
    NatsMaxBytes        int64         `env:"NATS_MAX_BYTES" envDefault:"1073741824"`
    RedisURL            string        `env:"REDIS_URL" envDefault:"redis://localhost:6379/0"`
    RedisAPIKeyTTL      time.Duration `env:"REDIS_API_KEY_TTL" envDefault:"5m"`
    RedisUserCacheTTL   time.Duration `env:"REDIS_USER_CACHE_TTL" envDefault:"60s"`
    RedisSegmentCacheTTL time.Duration `env:"REDIS_SEGMENT_CACHE_TTL" envDefault:"10m"`
    APIPort             int           `env:"API_PORT" envDefault:"8080"`
    APIRateLimit        int           `env:"API_RATE_LIMIT" envDefault:"100"`
    APIRateWindow       time.Duration `env:"API_RATE_WINDOW" envDefault:"60s"`
    SegmentInterval     time.Duration `env:"SEGMENT_INTERVAL" envDefault:"5m"`
    SchemaFlushInterval time.Duration `env:"SCHEMA_FLUSH_INTERVAL" envDefault:"30s"`
    DLQMaxRetries       int           `env:"DLQ_MAX_RETRIES" envDefault:"5"`
    EventRetentionDays  int           `env:"EVENT_RETENTION_DAYS" envDefault:"90"`
}
```

---

## 5. DOCKER COMPOSE (DEV)

This runs dependencies only. The Go API and worker run on the host during development.

```yaml
# docker-compose.dev.yml
version: "3.9"
services:
  postgres:
    image: timescale/timescaledb:latest-pg16
    ports:
      - "5432:5432"
    environment:
      POSTGRES_USER: opentap
      POSTGRES_PASSWORD: opentap
      POSTGRES_DB: opentap
    volumes:
      - pgdata:/var/lib/postgresql/data

  nats:
    image: nats:2.10-alpine
    ports:
      - "4222:4222"
      - "8222:8222"
    command: ["--jetstream", "--store_dir", "/data", "--http_port", "8222"]
    volumes:
      - natsdata:/data

  redis:
    image: redis:7-alpine
    ports:
      - "6379:6379"

volumes:
  pgdata:
  natsdata:
```

---

## 6. DATABASE SCHEMA

Run these migrations in order. Each file is idempotent (uses `IF NOT EXISTS`).

### 001_create_projects.sql

```sql
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

CREATE TABLE IF NOT EXISTS projects (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name VARCHAR(255) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS api_keys (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    key_hash VARCHAR(64) NOT NULL UNIQUE,
    key_prefix VARCHAR(8) NOT NULL,
    label VARCHAR(255),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    revoked_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_api_keys_key_hash ON api_keys(key_hash);
CREATE INDEX IF NOT EXISTS idx_api_keys_project_id ON api_keys(project_id);
```

**API key design:** Generate a random 32-byte key, return the raw key to the user once. Store SHA-256 hash in `key_hash`. Store first 8 chars in `key_prefix` for identification. On auth, hash the incoming key and look up `key_hash`. A revoked key has `revoked_at IS NOT NULL`.

### 002_create_users.sql

```sql
CREATE TABLE IF NOT EXISTS users (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    user_id VARCHAR(255) NOT NULL,
    properties JSONB DEFAULT '{}',
    first_seen TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_seen TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    event_count BIGINT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(project_id, user_id)
);

CREATE INDEX IF NOT EXISTS idx_users_project_user ON users(project_id, user_id);
CREATE INDEX IF NOT EXISTS idx_users_project_last_seen ON users(project_id, last_seen DESC);
```

**`user_id`** is an opaque string provided by the caller. It is NOT a UUID — it can be an email, a username, a database ID, anything. The `id` column is the internal primary key. The `(project_id, user_id)` pair is the unique business key.

### 003_create_events.sql

```sql
CREATE TABLE IF NOT EXISTS events (
    id UUID DEFAULT uuid_generate_v4(),
    project_id UUID NOT NULL,
    user_id VARCHAR(255) NOT NULL,
    event_name VARCHAR(255) NOT NULL,
    properties JSONB DEFAULT '{}',
    timestamp TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    received_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

SELECT create_hypertable('events', 'timestamp', if_not_exists => TRUE);

CREATE INDEX IF NOT EXISTS idx_events_project_name_time
    ON events (project_id, event_name, timestamp DESC);

CREATE INDEX IF NOT EXISTS idx_events_project_user_time
    ON events (project_id, user_id, timestamp DESC);

-- Retention policy
SELECT add_retention_policy('events', INTERVAL '90 days', if_not_exists => TRUE);
```

**`timestamp`** is when the event occurred (client-provided or defaults to now). **`received_at`** is when the server received it. The hypertable partitions on `timestamp`. Do NOT add a foreign key from `events.project_id` to `projects.id` — it's a hypertable and FK constraints are not supported on the partitioning column.

### 004_create_event_schemas.sql

```sql
CREATE TABLE IF NOT EXISTS event_schemas (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    event_name VARCHAR(255) NOT NULL,
    property_name VARCHAR(255) NOT NULL,
    property_type VARCHAR(50) NOT NULL DEFAULT 'string',
    distinct_values_count INT NOT NULL DEFAULT 0,
    first_seen TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_seen TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(project_id, event_name, property_name)
);

CREATE INDEX IF NOT EXISTS idx_event_schemas_project_event
    ON event_schemas(project_id, event_name);
```

**`property_type`** is one of: `string`, `number`, `boolean`, `object`, `array`, `null`. Inferred from the JSONB value by the worker. **`distinct_values_count`** is approximate — the worker increments it when it sees a new value. If `distinct_values_count <= 20`, the dashboard shows a dropdown. Otherwise, it shows a text input.

### 005_create_segments.sql

```sql
CREATE TABLE IF NOT EXISTS segments (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    description TEXT,
    conditions JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_segments_project ON segments(project_id);
```

**`conditions`** JSONB format — this is the contract the segment evaluator must implement:

```json
{
  "operator": "AND",
  "rules": [
    {
      "type": "event",
      "event_name": "purchase",
      "time_window": "30d",
      "property_filters": [
        { "property": "amount", "operator": "gt", "value": 100 }
      ],
      "count_operator": "gte",
      "count_value": 1
    },
    {
      "type": "user_property",
      "property": "plan",
      "operator": "eq",
      "value": "pro"
    }
  ]
}
```

Supported rule types: `event`, `user_property`.
Supported top-level operators: `AND`, `OR`.
Supported comparison operators: `eq`, `neq`, `gt`, `gte`, `lt`, `lte`, `contains`, `not_contains`, `is_set`, `is_not_set`.
Supported time windows: `1h`, `24h`, `7d`, `30d`, `90d`, `all`.

### 006_create_segment_users.sql

```sql
CREATE TABLE IF NOT EXISTS segment_users (
    segment_id UUID NOT NULL REFERENCES segments(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    entered_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (segment_id, user_id)
);

CREATE INDEX IF NOT EXISTS idx_segment_users_segment ON segment_users(segment_id);
CREATE INDEX IF NOT EXISTS idx_segment_users_user ON segment_users(user_id);
```

The segment worker **deletes all rows for a segment and re-inserts** on each run. This is simpler than diffing.

---

## 7. NATS CONFIGURATION

### Stream

```
Stream Name:    EVENTS
Subjects:       events.ingest.>
Storage:        File
Max Bytes:      1GB (configurable)
Retention:      WorkQueue (each message delivered to one consumer only)
Discard:        Old (drop oldest when full)
```

### Consumer (Ingestion Worker)

```
Consumer Name:  ingestion-worker
Durable:        Yes
Ack Policy:     Explicit
Max Deliver:    5 (then message goes to DLQ)
Ack Wait:       30s
Filter Subject: events.ingest.>
```

### Dead Letter

When a message exceeds `Max Deliver`, the worker publishes it to `events.dlq` with metadata (original subject, error, retry count) before acknowledging.

### Message Format

The API publishes this JSON to `events.ingest.{project_id}`:

```json
{
  "id": "uuid-v4",
  "project_id": "uuid-v4",
  "user_id": "string",
  "event_name": "string",
  "properties": {},
  "timestamp": "2026-01-15T10:30:00Z",
  "received_at": "2026-01-15T10:30:01Z"
}
```

---

## 8. GOLANG PACKAGES — INITIALIZATION PATTERNS

### pkg/db/postgres.go

```go
package db

import (
    "context"
    "github.com/jackc/pgx/v5/pgxpool"
)

func NewPool(ctx context.Context, databaseURL string, maxConns, minConns int32) (*pgxpool.Pool, error) {
    config, err := pgxpool.ParseConfig(databaseURL)
    if err != nil {
        return nil, err
    }
    config.MaxConns = maxConns
    config.MinConns = minConns
    pool, err := pgxpool.NewWithConfig(ctx, config)
    if err != nil {
        return nil, err
    }
    return pool, pool.Ping(ctx)
}
```

### pkg/queue/nats.go

```go
package queue

import (
    "github.com/nats-io/nats.go"
)

type NATSClient struct {
    Conn      *nats.Conn
    JetStream nats.JetStreamContext
}

func NewNATSClient(url, streamName string, maxBytes int64) (*NATSClient, error) {
    nc, err := nats.Connect(url)
    if err != nil {
        return nil, err
    }
    js, err := nc.JetStream()
    if err != nil {
        return nil, err
    }
    // Create or update stream
    _, err = js.AddStream(&nats.StreamConfig{
        Name:      streamName,
        Subjects:  []string{"events.ingest.>", "events.dlq"},
        Storage:   nats.FileStorage,
        MaxBytes:  maxBytes,
        Retention: nats.WorkQueuePolicy,
        Discard:   nats.DiscardOld,
    })
    if err != nil {
        return nil, err
    }
    return &NATSClient{Conn: nc, JetStream: js}, nil
}

func (c *NATSClient) Publish(subject string, data []byte) error {
    _, err := c.JetStream.Publish(subject, data)
    return err
}

func (c *NATSClient) Close() {
    c.Conn.Close()
}
```

### pkg/cache/redis.go

```go
package cache

import (
    "context"
    "time"
    "github.com/redis/go-redis/v9"
)

type RedisClient struct {
    Client *redis.Client
}

func NewRedisClient(url string) (*RedisClient, error) {
    opts, err := redis.ParseURL(url)
    if err != nil {
        return nil, err
    }
    client := redis.NewClient(opts)
    if err := client.Ping(context.Background()).Err(); err != nil {
        return nil, err
    }
    return &RedisClient{Client: client}, nil
}

func (r *RedisClient) Get(ctx context.Context, key string) (string, error) {
    return r.Client.Get(ctx, key).Result()
}

func (r *RedisClient) Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
    return r.Client.Set(ctx, key, value, ttl).Err()
}

func (r *RedisClient) Delete(ctx context.Context, key string) error {
    return r.Client.Del(ctx, key).Err()
}

func (r *RedisClient) Close() error {
    return r.Client.Close()
}
```

### pkg/response/json.go

```go
package response

import (
    "encoding/json"
    "net/http"
)

type ErrorBody struct {
    Code    string `json:"code"`
    Message string `json:"message"`
}

type ErrorResponse struct {
    Error ErrorBody `json:"error"`
}

type PaginatedResponse struct {
    Data       interface{} `json:"data"`
    NextCursor string      `json:"next_cursor,omitempty"`
    HasMore    bool        `json:"has_more"`
}

func JSON(w http.ResponseWriter, status int, data interface{}) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(status)
    json.NewEncoder(w).Encode(data)
}

func Error(w http.ResponseWriter, status int, code, message string) {
    JSON(w, status, ErrorResponse{
        Error: ErrorBody{Code: code, Message: message},
    })
}
```

---

## 9. INTERNAL PACKAGE CONTRACTS

Every domain package implements these interface patterns. The handler depends on the service, the service depends on the repository. Never skip a layer.

### internal/models/models.go

```go
package models

import (
    "time"
    "github.com/google/uuid"
)

type Project struct {
    ID        uuid.UUID `json:"id"`
    Name      string    `json:"name"`
    CreatedAt time.Time `json:"created_at"`
    UpdatedAt time.Time `json:"updated_at"`
}

type APIKey struct {
    ID        uuid.UUID  `json:"id"`
    ProjectID uuid.UUID  `json:"project_id"`
    KeyPrefix string     `json:"key_prefix"`
    Label     string     `json:"label,omitempty"`
    CreatedAt time.Time  `json:"created_at"`
    RevokedAt *time.Time `json:"revoked_at,omitempty"`
}

type User struct {
    ID         uuid.UUID              `json:"id"`
    ProjectID  uuid.UUID              `json:"project_id"`
    UserID     string                 `json:"user_id"`
    Properties map[string]interface{} `json:"properties"`
    FirstSeen  time.Time              `json:"first_seen"`
    LastSeen   time.Time              `json:"last_seen"`
    EventCount int64                  `json:"event_count"`
    CreatedAt  time.Time              `json:"created_at"`
    UpdatedAt  time.Time              `json:"updated_at"`
}

type Event struct {
    ID         uuid.UUID              `json:"id"`
    ProjectID  uuid.UUID              `json:"project_id"`
    UserID     string                 `json:"user_id"`
    EventName  string                 `json:"event_name"`
    Properties map[string]interface{} `json:"properties"`
    Timestamp  time.Time              `json:"timestamp"`
    ReceivedAt time.Time              `json:"received_at"`
}

type EventSchema struct {
    ID                  uuid.UUID `json:"id"`
    ProjectID           uuid.UUID `json:"project_id"`
    EventName           string    `json:"event_name"`
    PropertyName        string    `json:"property_name"`
    PropertyType        string    `json:"property_type"`
    DistinctValuesCount int       `json:"distinct_values_count"`
    FirstSeen           time.Time `json:"first_seen"`
    LastSeen            time.Time `json:"last_seen"`
}

type Segment struct {
    ID          uuid.UUID              `json:"id"`
    ProjectID   uuid.UUID              `json:"project_id"`
    Name        string                 `json:"name"`
    Description string                 `json:"description,omitempty"`
    Conditions  map[string]interface{} `json:"conditions"`
    CreatedAt   time.Time              `json:"created_at"`
    UpdatedAt   time.Time              `json:"updated_at"`
}

type SegmentUser struct {
    SegmentID uuid.UUID `json:"segment_id"`
    UserID    uuid.UUID `json:"user_id"`
    EnteredAt time.Time `json:"entered_at"`
}

// --- Request/Response types ---

type TrackEventRequest struct {
    UserID     string                 `json:"user_id" validate:"required,max=255"`
    EventName  string                 `json:"event_name" validate:"required,max=255"`
    Properties map[string]interface{} `json:"properties"`
    Timestamp  *time.Time             `json:"timestamp"`
}

type TrackBatchRequest struct {
    Events []TrackEventRequest `json:"events" validate:"required,min=1,max=100,dive"`
}

type CreateProjectRequest struct {
    Name string `json:"name" validate:"required,max=255"`
}

type CreateSegmentRequest struct {
    Name        string                 `json:"name" validate:"required,max=255"`
    Description string                 `json:"description"`
    Conditions  map[string]interface{} `json:"conditions" validate:"required"`
}

type UpdateSegmentRequest struct {
    Name        *string                `json:"name" validate:"omitempty,max=255"`
    Description *string                `json:"description"`
    Conditions  map[string]interface{} `json:"conditions"`
}

type CursorParams struct {
    Cursor string
    Limit  int
}

type AnalyticsQuery struct {
    EventName  string
    TimeWindow string
    Interval   string // "hour", "day", "week", "month"
}
```

### Repository Interfaces

Each domain package defines its own repository interface. Here are the key ones:

```go
// internal/events/repository.go
type Repository interface {
    Insert(ctx context.Context, event *models.Event) error
    InsertBatch(ctx context.Context, events []*models.Event) error
    GetByID(ctx context.Context, projectID, eventID uuid.UUID) (*models.Event, error)
    List(ctx context.Context, projectID uuid.UUID, params models.CursorParams) ([]*models.Event, string, error)
    ListByUser(ctx context.Context, projectID uuid.UUID, userID string, params models.CursorParams) ([]*models.Event, string, error)
    CountByName(ctx context.Context, projectID uuid.UUID, query models.AnalyticsQuery) ([]TimeSeriesPoint, error)
    TopEvents(ctx context.Context, projectID uuid.UUID, timeWindow string, limit int) ([]EventCount, error)
}

// internal/users/repository.go
type Repository interface {
    Upsert(ctx context.Context, projectID uuid.UUID, userID string, timestamp time.Time) error
    IncrementEventCount(ctx context.Context, projectID uuid.UUID, userID string) error
    GetByUserID(ctx context.Context, projectID uuid.UUID, userID string) (*models.User, error)
    GetByID(ctx context.Context, projectID, id uuid.UUID) (*models.User, error)
    List(ctx context.Context, projectID uuid.UUID, params models.CursorParams) ([]*models.User, string, error)
}

// internal/segments/repository.go
type Repository interface {
    Create(ctx context.Context, segment *models.Segment) error
    GetByID(ctx context.Context, projectID, segmentID uuid.UUID) (*models.Segment, error)
    List(ctx context.Context, projectID uuid.UUID, params models.CursorParams) ([]*models.Segment, string, error)
    Update(ctx context.Context, segment *models.Segment) error
    Delete(ctx context.Context, projectID, segmentID uuid.UUID) error
    ListAll(ctx context.Context) ([]*models.Segment, error) // For segment worker
    ReplaceUsers(ctx context.Context, segmentID uuid.UUID, userIDs []uuid.UUID) error
    GetUsers(ctx context.Context, segmentID uuid.UUID, params models.CursorParams) ([]*models.User, string, error)
}

// internal/schemas/repository.go
type Repository interface {
    Upsert(ctx context.Context, schema *models.EventSchema) error
    UpsertBatch(ctx context.Context, schemas []*models.EventSchema) error
    ListByProject(ctx context.Context, projectID uuid.UUID) ([]*models.EventSchema, error)
    ListByEvent(ctx context.Context, projectID uuid.UUID, eventName string) ([]*models.EventSchema, error)
}

// internal/analytics/repository.go
type Repository interface {
    EventCountOverTime(ctx context.Context, projectID uuid.UUID, query models.AnalyticsQuery) ([]TimeSeriesPoint, error)
    TopEvents(ctx context.Context, projectID uuid.UUID, timeWindow string, limit int) ([]EventCount, error)
    DAU(ctx context.Context, projectID uuid.UUID, timeWindow string) ([]TimeSeriesPoint, error)
}

// internal/auth/repository.go
type Repository interface {
    ValidateKey(ctx context.Context, rawKey string) (*models.APIKey, error) // checks Redis, falls back to DB
    CreateKey(ctx context.Context, projectID uuid.UUID, label string) (rawKey string, apiKey *models.APIKey, error)
    RevokeKey(ctx context.Context, projectID, keyID uuid.UUID) error // also deletes from Redis
}
```

### Analytics Return Types

```go
type TimeSeriesPoint struct {
    Timestamp time.Time `json:"timestamp"`
    Count     int64     `json:"count"`
}

type EventCount struct {
    EventName string `json:"event_name"`
    Count     int64  `json:"count"`
}
```

---

## 10. API SERVER (`cmd/api/main.go`)

```go
func main() {
    // 1. Load config
    // 2. Init zerolog
    // 3. Connect pgxpool
    // 4. Connect Redis
    // 5. Connect NATS (for publishing only — the API server does NOT consume)
    // 6. Init repositories
    // 7. Init services
    // 8. Init handlers
    // 9. Build chi router with middleware stack
    // 10. Start HTTP server with graceful shutdown

    // Middleware stack (order matters):
    // - recovery
    // - request logging
    // - rate limiting
    // - API key auth (skip for health check)

    // Routes — see Section 12 for full specification
}
```

Graceful shutdown: listen for `SIGINT`/`SIGTERM`, call `server.Shutdown(ctx)`, close DB pool, close NATS, close Redis.

---

## 11. WORKER (`cmd/worker/main.go`)

```go
func main() {
    // 1. Load config
    // 2. Init zerolog
    // 3. Connect pgxpool (separate pool from API)
    // 4. Connect Redis
    // 5. Connect NATS
    // 6. Init repositories (events, users, schemas, segments)
    // 7. Start ingestion worker goroutine
    // 8. Start segment worker goroutine (ticker-based)
    // 9. Block on signal, graceful shutdown
}
```

### Ingestion Worker Logic

```
subscribe to "events.ingest.>" with durable consumer "ingestion-worker"

on each message:
    1. Unmarshal JSON into Event struct
    2. Validate required fields
    3. BEGIN transaction
    4. Insert event into TimescaleDB
    5. Upsert user (update last_seen, increment event_count)
    6. COMMIT transaction
    7. Accumulate property schemas in memory buffer
    8. msg.Ack()

on error:
    if retries < max:
        msg.Nak() (NATS will redeliver)
    else:
        publish to events.dlq with error metadata
        msg.Ack() (remove from main stream)

every SCHEMA_FLUSH_INTERVAL:
    flush accumulated schemas to event_schemas table (batch upsert)
    clear memory buffer
```

### Segment Worker Logic

```
every SEGMENT_INTERVAL:
    1. Load all segment definitions from PostgreSQL
    2. For each segment:
        a. Parse conditions JSONB
        b. Build SQL query from conditions
        c. Execute query against TimescaleDB + users table
        d. DELETE all existing segment_users for this segment
        e. INSERT new segment_users
        f. Update Redis cache key "segment:{segment_id}:users" with user IDs
    3. Log results (segment name, user count, duration)
```

---

## 12. API ENDPOINTS — FULL SPECIFICATION

### Health

**`GET /health`** — No auth required.

Response `200`:
```json
{ "status": "ok" }
```

---

### Events

**`POST /v1/events`** — Ingest a single event.

Request:
```json
{
  "user_id": "user_123",
  "event_name": "page_viewed",
  "properties": { "page": "/pricing", "referrer": "google" },
  "timestamp": "2026-01-15T10:30:00Z"
}
```

- `user_id` required, max 255 chars
- `event_name` required, max 255 chars
- `properties` optional, defaults to `{}`
- `timestamp` optional, defaults to server time

Processing: validate → assign UUID + `received_at` → serialize to JSON → publish to `events.ingest.{project_id}` → return `202 Accepted`.

Response `202`:
```json
{
  "id": "event-uuid",
  "status": "accepted"
}
```

The API does NOT wait for the worker to process the event. It returns as soon as NATS accepts the message.

---

**`POST /v1/events/batch`** — Ingest multiple events.

Request:
```json
{
  "events": [
    { "user_id": "user_123", "event_name": "page_viewed", "properties": {} },
    { "user_id": "user_456", "event_name": "button_clicked", "properties": {} }
  ]
}
```

- Max 100 events per batch
- Each event validated individually
- Each event published as a separate NATS message

Response `202`:
```json
{
  "accepted": 2,
  "ids": ["uuid-1", "uuid-2"]
}
```

---

**`GET /v1/events`** — List events.

Query params: `cursor`, `limit` (default 50, max 100), `event_name` (optional filter), `user_id` (optional filter), `start` (ISO8601), `end` (ISO8601).

Response `200`:
```json
{
  "data": [ { ...event } ],
  "next_cursor": "encoded-cursor-string",
  "has_more": true
}
```

Cursor is a base64-encoded `timestamp:id` pair.

---

**`GET /v1/events/:id`** — Get single event.

Response `200`: single event object.
Response `404`: `{ "error": { "code": "not_found", "message": "Event not found" } }`

---

### Users

**`GET /v1/users`** — List users.

Query params: `cursor`, `limit` (default 50, max 100).

Response `200`:
```json
{
  "data": [ { ...user } ],
  "next_cursor": "...",
  "has_more": true
}
```

---

**`GET /v1/users/:id`** — Get user profile. `:id` is the internal UUID.

Response `200`: single user object with all fields.

---

**`GET /v1/users/:id/events`** — Get events for a user.

Query params: `cursor`, `limit`, `event_name`, `start`, `end`.

Response `200`: paginated event list.

---

### Segments

**`POST /v1/segments`** — Create segment.

Request:
```json
{
  "name": "High Value Users",
  "description": "Users who purchased more than $100 in 30 days",
  "conditions": {
    "operator": "AND",
    "rules": [
      {
        "type": "event",
        "event_name": "purchase",
        "time_window": "30d",
        "property_filters": [
          { "property": "amount", "operator": "gt", "value": 100 }
        ],
        "count_operator": "gte",
        "count_value": 1
      }
    ]
  }
}
```

Validate the conditions structure before saving. Response `201`: created segment object.

---

**`GET /v1/segments`** — List segments. Paginated.

**`GET /v1/segments/:id`** — Get segment with user count (from cache or computed).

Response `200`:
```json
{
  "id": "...",
  "name": "High Value Users",
  "conditions": { ... },
  "user_count": 1523,
  "last_computed_at": "2026-01-15T10:30:00Z",
  "created_at": "...",
  "updated_at": "..."
}
```

**`PUT /v1/segments/:id`** — Update segment. Partial update allowed.

**`DELETE /v1/segments/:id`** — Delete segment and all `segment_users` rows (cascades).

**`GET /v1/segments/:id/users`** — Get users in segment. Paginated. Reads from `segment_users` join `users`.

---

### Analytics

**`GET /v1/analytics/events/count`** — Event counts over time.

Query params: `event_name` (optional — all events if omitted), `time_window` (`1h`, `24h`, `7d`, `30d`, `90d`), `interval` (`hour`, `day`, `week`, `month`).

Response `200`:
```json
{
  "data": [
    { "timestamp": "2026-01-15T00:00:00Z", "count": 1234 },
    { "timestamp": "2026-01-16T00:00:00Z", "count": 1456 }
  ]
}
```

Uses TimescaleDB `time_bucket` function.

---

**`GET /v1/analytics/events/top`** — Top events by volume.

Query params: `time_window` (default `7d`), `limit` (default 10, max 50).

Response `200`:
```json
{
  "data": [
    { "event_name": "page_viewed", "count": 45230 },
    { "event_name": "button_clicked", "count": 12340 }
  ]
}
```

---

**`GET /v1/analytics/users/active`** — DAU over time.

Query params: `time_window`, `interval`.

Response `200`: same shape as event count — `[{ timestamp, count }]`.

Uses `COUNT(DISTINCT user_id)` with `time_bucket`.

---

### Schemas

**`GET /v1/schemas`** — List known event types.

Response `200`:
```json
{
  "data": [
    { "event_name": "page_viewed", "property_count": 5, "first_seen": "...", "last_seen": "..." },
    { "event_name": "purchase", "property_count": 8, "first_seen": "...", "last_seen": "..." }
  ]
}
```

This is an aggregation of `event_schemas` grouped by `event_name`.

---

**`GET /v1/schemas/:event_name`** — Get properties for an event type.

Response `200`:
```json
{
  "event_name": "purchase",
  "properties": [
    { "name": "amount", "type": "number", "distinct_values_count": 150 },
    { "name": "currency", "type": "string", "distinct_values_count": 5 },
    { "name": "item_name", "type": "string", "distinct_values_count": 1200 }
  ]
}
```

---

### Admin — Projects

**`POST /v1/projects`** — Create project.

Request: `{ "name": "My App" }`
Response `201`: project object.

**`GET /v1/projects`** — List projects.

**`POST /v1/projects/:id/keys`** — Generate API key.

Request: `{ "label": "Production Key" }`
Response `201`:
```json
{
  "id": "key-uuid",
  "key": "ot_a1b2c3d4e5f6...",
  "key_prefix": "ot_a1b2c",
  "label": "Production Key",
  "created_at": "..."
}
```

The `key` field is returned **only once** at creation time. It is never stored or returned again.

Key format: prefix `ot_` + 43 random base62 characters = 46 chars total.

**`DELETE /v1/projects/:id/keys/:key_id`** — Revoke API key. Sets `revoked_at`, deletes from Redis cache.

---

## 13. REDIS CACHE KEYS

| Key Pattern | Value | TTL |
|---|---|---|
| `apikey:{sha256_hash}` | JSON `{ "id": "...", "project_id": "..." }` | 5m |
| `user:{project_id}:{user_id}` | JSON user object | 60s |
| `segment:{segment_id}:users` | JSON array of user UUIDs | 10m |
| `segment:{segment_id}:count` | integer count | 10m |
| `ratelimit:{project_id}:{window}` | integer counter | matches window duration |

---

## 14. ERROR HANDLING

Every error response uses this format:

```json
{
  "error": {
    "code": "string_error_code",
    "message": "Human readable message"
  }
}
```

| HTTP Status | Code | When |
|---|---|---|
| 400 | `bad_request` | Malformed JSON, missing required fields |
| 400 | `validation_error` | Field validation failure (include field details in message) |
| 401 | `unauthorized` | Missing or invalid API key |
| 403 | `forbidden` | API key valid but revoked |
| 404 | `not_found` | Resource doesn't exist |
| 409 | `conflict` | Duplicate resource (e.g., project name) |
| 429 | `rate_limited` | Rate limit exceeded. Include `Retry-After` header. |
| 500 | `internal_error` | Unexpected server error. Log full error, return generic message. |

Never expose internal error details (stack traces, SQL errors) in the response.

---

## 15. BUILD ORDER

### Milestone 1: Event Pipeline

**Goal:** `POST /v1/events` → NATS → TimescaleDB. End to end.

Steps:
1. Create `docker-compose.dev.yml` and `docker compose up -d`
2. Run migration `001_create_projects.sql` manually
3. Run migration `003_create_events.sql` manually
4. Implement `pkg/db/postgres.go`
5. Implement `pkg/queue/nats.go`
6. Implement `pkg/response/json.go`
7. Implement `internal/config/config.go`
8. Implement `internal/models/models.go` (just `Event` and `TrackEventRequest` for now)
9. Implement `internal/events/repository.go` (just `Insert`)
10. Implement `internal/events/service.go` (validate + publish to NATS)
11. Implement `internal/events/handler.go` (just `POST /v1/events`)
12. Implement `cmd/api/main.go` (hardcoded API key, single route)
13. Implement worker: subscribe to NATS, unmarshal, insert into TimescaleDB
14. Implement `cmd/worker/main.go`
15. Test: `curl POST /v1/events` → check TimescaleDB row exists

**Hardcode the API key** as a constant. No Redis, no auth middleware, no rate limiting yet. Just the pipeline.

---

### Milestone 2: User Profiles

**Goal:** Users auto-created on event ingestion. User APIs work.

Steps:
1. Run migration `002_create_users.sql`
2. Implement `internal/users/repository.go` (Upsert, IncrementEventCount, GetByID, GetByUserID, List)
3. Implement `internal/users/service.go`
4. Implement `internal/users/handler.go`
5. Update worker: after inserting event, call `users.Repository.Upsert` and `IncrementEventCount`
6. Add routes: `GET /v1/users`, `GET /v1/users/:id`, `GET /v1/users/:id/events`
7. Test: send events → verify user created → query user APIs

---

### Milestone 3: Event Schema Registry

**Goal:** Worker auto-tracks event properties. Schema APIs work.

Steps:
1. Run migration `004_create_event_schemas.sql`
2. Implement `internal/schemas/repository.go` (UpsertBatch, ListByProject, ListByEvent)
3. Implement `internal/schemas/service.go`
4. Implement `internal/schemas/handler.go`
5. Add schema buffer to worker (in-memory map, flush every 30s)
6. Worker: on each event, extract property names/types → accumulate in buffer
7. Worker: on flush timer, batch upsert to `event_schemas`
8. Add routes: `GET /v1/schemas`, `GET /v1/schemas/:event_name`
9. Test: send events with varied properties → verify schemas populated → query API

---

### Milestone 4: Segmentation Engine

**Goal:** Segment CRUD + background evaluation.

Steps:
1. Run migrations `005_create_segments.sql`, `006_create_segment_users.sql`
2. Implement `internal/segments/repository.go`
3. Implement `internal/segments/evaluator.go` — parses conditions JSONB, builds SQL
4. Implement `internal/segments/service.go`
5. Implement `internal/segments/handler.go`
6. Implement segment worker loop in `cmd/worker/main.go`
7. Add routes: full segments CRUD + `GET /v1/segments/:id/users`
8. Test: create segment → wait for worker run → verify users matched

---

### Milestone 5: Analytics APIs + Auth + Redis

**Goal:** Analytics endpoints. Real API key auth. Redis caching. Rate limiting.

Steps:
1. Implement `pkg/cache/redis.go`
2. Implement `internal/auth/repository.go` (Redis → DB fallback)
3. Implement `internal/auth/middleware.go`
4. Implement `internal/middleware/ratelimit.go`
5. Implement `internal/middleware/logging.go`
6. Implement `internal/middleware/recovery.go`
7. Implement `internal/projects/repository.go`, `service.go`, `handler.go`
8. Implement `internal/analytics/repository.go` (TimescaleDB time_bucket queries)
9. Implement `internal/analytics/service.go`, `handler.go`
10. Wire up all middleware in `cmd/api/main.go`
11. Add admin routes: projects + API keys
12. Add analytics routes
13. Add Redis caching to segment worker and user lookups
14. Test: full auth flow, rate limiting, analytics queries

---

### Milestone 6: React Dashboard

**Goal:** Working dashboard with charts, user list, segment builder.

Pages:
1. **Dashboard home** — DAU chart, top events chart, total users/events counters
2. **Events** — event list with filters (name, user, date range), event detail view
3. **Users** — user list with search, user detail with profile + event timeline
4. **Segments** — segment list, segment builder (condition editor), segment detail with user list
5. **Schemas** — event type list, property explorer per event type
6. **Settings** — project info, API key management

Use: React 18, React Router, Recharts for charts, Tailwind CSS for styling, fetch (no axios).

---

### Milestone 7: Production Docker + Docs

Steps:
1. Write `Dockerfile.api` (multi-stage Go build)
2. Write `Dockerfile.worker` (multi-stage Go build)
3. Write `docker-compose.yml` with Caddy, API, Worker, NATS, PostgreSQL, Redis
4. Write `Caddyfile` with auto HTTPS + reverse proxy to API
5. Write `README.md` — setup guide, architecture diagram, API reference
6. Write `CONTRIBUTING.md`
7. Test: `docker-compose up` on a clean machine → everything works

---

## 16. TESTING REQUIREMENTS

### Unit Tests

Every repository method, service method, and handler gets a test. Use:
- `testing` stdlib
- `github.com/stretchr/testify/assert` for assertions
- Table-driven tests for validation logic

### Integration Tests

Use `testcontainers-go` to spin up PostgreSQL + TimescaleDB + NATS + Redis in tests.

Test the full pipeline:
1. `POST /v1/events` → verify NATS message published
2. Run worker → verify event in TimescaleDB + user upserted
3. Query `GET /v1/events` → verify event returned
4. Create segment → run segment worker → verify users matched

### Naming Convention

```
internal/events/repository_test.go
internal/events/service_test.go
internal/events/handler_test.go
```

---

## 17. MAKEFILE

```makefile
.PHONY: dev api worker migrate test lint

dev:
	docker compose -f docker-compose.dev.yml up -d

api:
	go run cmd/api/main.go

worker:
	go run cmd/worker/main.go

migrate:
	@for f in migrations/*.sql; do \
		echo "Running $$f..."; \
		psql $(DATABASE_URL) -f $$f; \
	done

test:
	go test ./... -v -count=1

lint:
	golangci-lint run ./...

build-api:
	CGO_ENABLED=0 go build -o bin/api cmd/api/main.go

build-worker:
	CGO_ENABLED=0 go build -o bin/worker cmd/worker/main.go
```

---

*OpenTap · Agent Build Specification · v1.0 · March 2026*
