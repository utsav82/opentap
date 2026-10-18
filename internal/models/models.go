package models

import (
	"errors"
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
	Scope     string     `json:"scope"`
}

const (
	ScopeFull   = "full"
	ScopeIngest = "ingest"
)

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
	ID             uuid.UUID              `json:"id"`
	ProjectID      uuid.UUID              `json:"project_id"`
	UserID         string                 `json:"user_id"`
	EventName      string                 `json:"event_name"`
	Properties     map[string]interface{} `json:"properties"`
	Timestamp      time.Time              `json:"timestamp"`
	ReceivedAt     time.Time              `json:"received_at"`
	UserProperties map[string]interface{} `json:"user_properties,omitempty" db:"-"`
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
	ID             uuid.UUID              `json:"id"`
	ProjectID      uuid.UUID              `json:"project_id"`
	Name           string                 `json:"name"`
	Description    string                 `json:"description,omitempty"`
	Conditions     map[string]interface{} `json:"conditions"`
	CreatedAt      time.Time              `json:"created_at"`
	UpdatedAt      time.Time              `json:"updated_at"`
	LastComputedAt *time.Time             `json:"last_computed_at,omitempty"`
	WebhookURL     string                 `json:"webhook_url,omitempty"`
}

type SegmentUser struct {
	SegmentID uuid.UUID `json:"segment_id"`
	UserID    uuid.UUID `json:"user_id"`
	EnteredAt time.Time `json:"entered_at"`
}

// --- Request/Response types ---

type TrackEventRequest struct {
	ID             *uuid.UUID             `json:"id"`
	UserID         string                 `json:"user_id" validate:"required,max=255"`
	EventName      string                 `json:"event_name" validate:"required,max=255"`
	Properties     map[string]interface{} `json:"properties"`
	UserProperties map[string]interface{} `json:"user_properties"`
	Timestamp      *time.Time             `json:"timestamp"`
}

type TrackBatchRequest struct {
	Events []TrackEventRequest `json:"events" validate:"required,min=1,max=100,dive"`
}

type CreateProjectRequest struct {
	Name string `json:"name" validate:"required,max=255"`
}

type IdentifyRequest struct {
	AnonymousID string `json:"anonymous_id" validate:"required,max=255,nefield=UserID"`
	UserID      string `json:"user_id" validate:"required,max=255"`
}

type CreateSegmentRequest struct {
	Name        string                 `json:"name" validate:"required,max=255"`
	Description string                 `json:"description"`
	Conditions  map[string]interface{} `json:"conditions" validate:"required"`
	WebhookURL  string                 `json:"webhook_url" validate:"omitempty,http_url"`
}

type UpdateSegmentRequest struct {
	Name        *string                `json:"name" validate:"omitempty,max=255"`
	Description *string                `json:"description"`
	Conditions  map[string]interface{} `json:"conditions"`
	WebhookURL  *string                `json:"webhook_url" validate:"omitempty,http_url"`
}

type CreateAPIKeyRequest struct {
	Label string `json:"label"`
	Scope string `json:"scope" validate:"omitempty,oneof=full ingest"`
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

var (
	ErrValidation = errors.New("validation")
	ErrNotFound   = errors.New("not found")
)

var TimeWindows = map[string]string{
	"1h":  "1 hour",
	"24h": "24 hours",
	"7d":  "7 days",
	"30d": "30 days",
	"90d": "90 days",
}

var Intervals = map[string]string{
	"hour":  "1 hour",
	"day":   "1 day",
	"week":  "1 week",
	"month": "1 month",
}
