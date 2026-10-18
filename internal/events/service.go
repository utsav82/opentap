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
		return uuid.Nil, fmt.Errorf("%w: %v", models.ErrValidation, err)
	}
	ids, err := s.publish(projectID, []models.TrackEventRequest{req})
	if err != nil {
		return uuid.Nil, err
	}
	return ids[0], nil
}

func (s *service) TrackBatch(ctx context.Context, projectID uuid.UUID, req models.TrackBatchRequest) ([]uuid.UUID, error) {
	if err := s.validate.Struct(req); err != nil {
		return nil, fmt.Errorf("%w: %v", models.ErrValidation, err)
	}
	return s.publish(projectID, req.Events)
}

// publish encodes every event before sending any, so an oversized event rejects the whole batch.
func (s *service) publish(projectID uuid.UUID, reqs []models.TrackEventRequest) ([]uuid.UUID, error) {
	now := time.Now().UTC()
	maxSize := s.nats.Conn.MaxPayload()
	ids := make([]uuid.UUID, len(reqs))
	messages := make([][]byte, len(reqs))

	for i, req := range reqs {
		event := models.Event{
			ID:             uuid.New(),
			ProjectID:      projectID,
			UserID:         req.UserID,
			EventName:      req.EventName,
			Properties:     req.Properties,
			UserProperties: req.UserProperties,
			Timestamp:      now,
			ReceivedAt:     now,
		}
		if req.ID != nil {
			event.ID = *req.ID
		}
		if req.Timestamp != nil {
			event.Timestamp = *req.Timestamp
		}
		if event.Properties == nil {
			event.Properties = map[string]interface{}{}
		}

		data, err := json.Marshal(event)
		if err != nil {
			return nil, fmt.Errorf("marshal event: %w", err)
		}
		if int64(len(data)) > maxSize {
			return nil, fmt.Errorf("%w: event %d is larger than %d bytes", models.ErrValidation, i, maxSize)
		}
		ids[i], messages[i] = event.ID, data
	}

	subject := "events.ingest." + projectID.String()
	for _, data := range messages {
		if err := s.nats.Publish(subject, data); err != nil {
			return nil, fmt.Errorf("publish event: %w", err)
		}
	}
	return ids, nil
}

func (s *service) GetEvent(ctx context.Context, projectID, eventID uuid.UUID) (*models.Event, error) {
	return s.repo.GetByID(ctx, projectID, eventID)
}

func (s *service) ListEvents(ctx context.Context, projectID uuid.UUID, params models.CursorParams, filters EventFilters) ([]*models.Event, string, error) {
	return s.repo.List(ctx, projectID, params, filters)
}
