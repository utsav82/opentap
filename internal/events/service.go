package events

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
	"github.com/opentap/opentap/internal/models"
	"github.com/opentap/opentap/pkg/queue"
)

type Service interface {
	TrackEvent(ctx context.Context, projectID uuid.UUID, req models.TrackEventRequest) (uuid.UUID, error)
	TrackBatch(ctx context.Context, projectID uuid.UUID, req models.TrackBatchRequest) ([]uuid.UUID, error)
	GetEvent(ctx context.Context, projectID, eventID uuid.UUID) (*models.Event, error)
	ListEvents(ctx context.Context, projectID uuid.UUID, params models.CursorParams, filters EventFilters) ([]*models.Event, string, error)
	ListUserEvents(ctx context.Context, projectID uuid.UUID, userID string, params models.CursorParams, filters EventFilters) ([]*models.Event, string, error)
}

type service struct {
	repo     Repository
	nats     *queue.NATSClient
	validate *validator.Validate
}

func NewService(repo Repository, nats *queue.NATSClient) Service {
	return &service{
		repo:     repo,
		nats:     nats,
		validate: validator.New(),
	}
}

func (s *service) TrackEvent(ctx context.Context, projectID uuid.UUID, req models.TrackEventRequest) (uuid.UUID, error) {
	if err := s.validate.Struct(req); err != nil {
		return uuid.Nil, fmt.Errorf("validation: %w", err)
	}

	now := time.Now().UTC()
	event := &models.Event{
		ID:        uuid.New(),
		ProjectID: projectID,
		UserID:    req.UserID,
		EventName: req.EventName,
		Properties: req.Properties,
		ReceivedAt: now,
	}

	if req.Timestamp != nil {
		event.Timestamp = *req.Timestamp
	} else {
		event.Timestamp = now
	}

	if event.Properties == nil {
		event.Properties = map[string]interface{}{}
	}

	data, err := json.Marshal(event)
	if err != nil {
		return uuid.Nil, fmt.Errorf("marshal event: %w", err)
	}

	subject := fmt.Sprintf("events.ingest.%s", projectID.String())
	if err := s.nats.Publish(subject, data); err != nil {
		return uuid.Nil, fmt.Errorf("publish event: %w", err)
	}

	return event.ID, nil
}

func (s *service) TrackBatch(ctx context.Context, projectID uuid.UUID, req models.TrackBatchRequest) ([]uuid.UUID, error) {
	if err := s.validate.Struct(req); err != nil {
		return nil, fmt.Errorf("validation: %w", err)
	}

	ids := make([]uuid.UUID, 0, len(req.Events))
	for _, eventReq := range req.Events {
		id, err := s.TrackEvent(ctx, projectID, eventReq)
		if err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, nil
}

func (s *service) GetEvent(ctx context.Context, projectID, eventID uuid.UUID) (*models.Event, error) {
	return s.repo.GetByID(ctx, projectID, eventID)
}

func (s *service) ListEvents(ctx context.Context, projectID uuid.UUID, params models.CursorParams, filters EventFilters) ([]*models.Event, string, error) {
	return s.repo.List(ctx, projectID, params, filters)
}

func (s *service) ListUserEvents(ctx context.Context, projectID uuid.UUID, userID string, params models.CursorParams, filters EventFilters) ([]*models.Event, string, error) {
	return s.repo.ListByUser(ctx, projectID, userID, params, filters)
}
