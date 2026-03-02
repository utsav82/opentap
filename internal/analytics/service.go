package analytics

import (
	"context"

	"github.com/google/uuid"
	"github.com/opentap/opentap/internal/models"
)

type Service interface {
	EventCountOverTime(ctx context.Context, projectID uuid.UUID, query models.AnalyticsQuery) ([]TimeSeriesPoint, error)
	TopEvents(ctx context.Context, projectID uuid.UUID, timeWindow string, limit int) ([]EventCount, error)
	DAU(ctx context.Context, projectID uuid.UUID, query models.AnalyticsQuery) ([]TimeSeriesPoint, error)
}

type service struct {
	repo Repository
}

func NewService(repo Repository) Service {
	return &service{repo: repo}
}

func (s *service) EventCountOverTime(ctx context.Context, projectID uuid.UUID, query models.AnalyticsQuery) ([]TimeSeriesPoint, error) {
	return s.repo.EventCountOverTime(ctx, projectID, query)
}

func (s *service) TopEvents(ctx context.Context, projectID uuid.UUID, timeWindow string, limit int) ([]EventCount, error) {
	return s.repo.TopEvents(ctx, projectID, timeWindow, limit)
}

func (s *service) DAU(ctx context.Context, projectID uuid.UUID, query models.AnalyticsQuery) ([]TimeSeriesPoint, error) {
	return s.repo.DAU(ctx, projectID, query)
}
