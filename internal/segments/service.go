package segments

import (
	"context"
	"fmt"
	"strconv"

	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
	"github.com/opentap/opentap/internal/models"
	"github.com/opentap/opentap/pkg/cache"
)

type SegmentWithCount struct {
	models.Segment
	UserCount int64 `json:"user_count"`
}

type Service interface {
	Create(ctx context.Context, projectID uuid.UUID, req models.CreateSegmentRequest) (*models.Segment, error)
	GetByID(ctx context.Context, projectID, segmentID uuid.UUID) (*SegmentWithCount, error)
	List(ctx context.Context, projectID uuid.UUID, params models.CursorParams) ([]*models.Segment, string, error)
	Update(ctx context.Context, projectID, segmentID uuid.UUID, req models.UpdateSegmentRequest) (*models.Segment, error)
	Delete(ctx context.Context, projectID, segmentID uuid.UUID) error
	GetUsers(ctx context.Context, projectID, segmentID uuid.UUID, params models.CursorParams) ([]*models.User, string, error)
	ExportUsers(ctx context.Context, projectID, segmentID uuid.UUID, fn func(*models.User) error) error
}

type service struct {
	repo     Repository
	cache    *cache.RedisClient
	validate *validator.Validate
}

func NewService(repo Repository, cache *cache.RedisClient) Service {
	return &service{
		repo:     repo,
		cache:    cache,
		validate: validator.New(),
	}
}

func CountCacheKey(segmentID uuid.UUID) string {
	return fmt.Sprintf("segment:%s:count", segmentID)
}

func (s *service) Create(ctx context.Context, projectID uuid.UUID, req models.CreateSegmentRequest) (*models.Segment, error) {
	if err := s.validateRequest(req, req.Conditions); err != nil {
		return nil, err
	}

	segment := &models.Segment{
		ProjectID:   projectID,
		Name:        req.Name,
		Description: req.Description,
		Conditions:  req.Conditions,
		WebhookURL:  req.WebhookURL,
	}

	if err := s.repo.Create(ctx, segment); err != nil {
		return nil, err
	}

	return segment, nil
}

func (s *service) GetByID(ctx context.Context, projectID, segmentID uuid.UUID) (*SegmentWithCount, error) {
	segment, err := s.repo.GetByID(ctx, projectID, segmentID)
	if err != nil || segment == nil {
		return nil, err
	}

	result := &SegmentWithCount{Segment: *segment}
	if cached, err := s.cache.Get(ctx, CountCacheKey(segmentID)); err == nil {
		if result.UserCount, err = strconv.ParseInt(cached, 10, 64); err == nil {
			return result, nil
		}
	}

	result.UserCount, err = s.repo.CountUsers(ctx, segmentID)
	return result, err
}

func (s *service) List(ctx context.Context, projectID uuid.UUID, params models.CursorParams) ([]*models.Segment, string, error) {
	return s.repo.List(ctx, projectID, params)
}

func (s *service) Update(ctx context.Context, projectID, segmentID uuid.UUID, req models.UpdateSegmentRequest) (*models.Segment, error) {
	segment, err := s.repo.GetByID(ctx, projectID, segmentID)
	if err != nil {
		return nil, err
	}
	if segment == nil {
		return nil, fmt.Errorf("segment %w", models.ErrNotFound)
	}

	if req.Name != nil {
		segment.Name = *req.Name
	}
	if req.Description != nil {
		segment.Description = *req.Description
	}
	if req.Conditions != nil {
		segment.Conditions = req.Conditions
	}
	if req.WebhookURL != nil {
		segment.WebhookURL = *req.WebhookURL
	}
	if err := s.validateRequest(req, segment.Conditions); err != nil {
		return nil, err
	}

	if err := s.repo.Update(ctx, segment); err != nil {
		return nil, err
	}
	s.cache.Delete(ctx, CountCacheKey(segmentID))
	return segment, nil
}

func (s *service) Delete(ctx context.Context, projectID, segmentID uuid.UUID) error {
	if err := s.repo.Delete(ctx, projectID, segmentID); err != nil {
		return err
	}
	s.cache.Delete(ctx, CountCacheKey(segmentID))
	return nil
}

func (s *service) GetUsers(ctx context.Context, projectID, segmentID uuid.UUID, params models.CursorParams) ([]*models.User, string, error) {
	segment, err := s.repo.GetByID(ctx, projectID, segmentID)
	if err != nil {
		return nil, "", err
	}
	if segment == nil {
		return nil, "", fmt.Errorf("segment %w", models.ErrNotFound)
	}
	return s.repo.GetUsers(ctx, segmentID, params)
}

func (s *service) ExportUsers(ctx context.Context, projectID, segmentID uuid.UUID, fn func(*models.User) error) error {
	return s.repo.EachUser(ctx, projectID, segmentID, fn)
}

func (s *service) validateRequest(req interface{}, conditions map[string]interface{}) error {
	if err := s.validate.Struct(req); err != nil {
		return fmt.Errorf("%w: %v", models.ErrValidation, err)
	}
	if _, _, err := BuildQuery(uuid.Nil, conditions); err != nil {
		return fmt.Errorf("%w: %v", models.ErrValidation, err)
	}
	return nil
}
