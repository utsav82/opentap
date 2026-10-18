package analytics

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/opentap/opentap/internal/models"
)

type Service interface {
	EventCountOverTime(ctx context.Context, projectID uuid.UUID, query models.AnalyticsQuery) ([]TimeSeriesPoint, error)
	TopEvents(ctx context.Context, projectID uuid.UUID, timeWindow string, limit int) ([]EventCount, error)
	DAU(ctx context.Context, projectID uuid.UUID, query models.AnalyticsQuery) ([]TimeSeriesPoint, error)
	Funnel(ctx context.Context, projectID uuid.UUID, steps []string, timeWindow string) ([]FunnelStep, error)
	Retention(ctx context.Context, projectID uuid.UUID, query RetentionQuery) ([]*RetentionCohort, error)
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

func (s *service) Funnel(ctx context.Context, projectID uuid.UUID, steps []string, timeWindow string) ([]FunnelStep, error) {
	if len(steps) < 2 || len(steps) > 10 {
		return nil, fmt.Errorf("%w: a funnel needs 2 to 10 steps", models.ErrValidation)
	}
	for _, step := range steps {
		if step == "" {
			return nil, fmt.Errorf("%w: funnel steps cannot be empty", models.ErrValidation)
		}
	}
	return s.repo.Funnel(ctx, projectID, steps, timeWindow)
}

func (s *service) Retention(ctx context.Context, projectID uuid.UUID, query RetentionQuery) ([]*RetentionCohort, error) {
	if query.StartEvent == "" {
		return nil, fmt.Errorf("%w: start_event is required", models.ErrValidation)
	}
	if query.ReturnEvent == "" {
		query.ReturnEvent = query.StartEvent
	}
	if _, ok := retentionIntervals[query.Interval]; !ok {
		return nil, fmt.Errorf("%w: interval must be day or week", models.ErrValidation)
	}
	if query.Periods < 1 || query.Periods > 52 {
		return nil, fmt.Errorf("%w: periods must be between 1 and 52", models.ErrValidation)
	}
	return s.repo.Retention(ctx, projectID, query)
}
