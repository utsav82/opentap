package segments

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
	"github.com/opentap/opentap/internal/models"
	"github.com/opentap/opentap/pkg/cache"
)

type SegmentWithCount struct {
	models.Segment
	UserCount      int64      `json:"user_count"`
	LastComputedAt *time.Time `json:"last_computed_at,omitempty"`
}

type Service interface {
	Create(ctx context.Context, projectID uuid.UUID, req models.CreateSegmentRequest) (*models.Segment, error)
	GetByID(ctx context.Context, projectID, segmentID uuid.UUID) (*SegmentWithCount, error)
	List(ctx context.Context, projectID uuid.UUID, params models.CursorParams) ([]*models.Segment, string, error)
	Update(ctx context.Context, projectID, segmentID uuid.UUID, req models.UpdateSegmentRequest) (*models.Segment, error)
	Delete(ctx context.Context, projectID, segmentID uuid.UUID) error
	GetUsers(ctx context.Context, segmentID uuid.UUID, params models.CursorParams) ([]*models.User, string, error)
}

type service struct {
	repo      Repository
	cache     *cache.RedisClient
	validate  *validator.Validate
}

func NewService(repo Repository, cache *cache.RedisClient) Service {
	return &service{
		repo:     repo,
		cache:    cache,
		validate: validator.New(),
	}
}

func (s *service) Create(ctx context.Context, projectID uuid.UUID, req models.CreateSegmentRequest) (*models.Segment, error) {
	if err := s.validate.Struct(req); err != nil {
		return nil, fmt.Errorf("validation: %w", err)
	}

	// Validate conditions structure
	if err := validateConditions(req.Conditions); err != nil {
		return nil, fmt.Errorf("validation: %w", err)
	}

	segment := &models.Segment{
		ProjectID:   projectID,
		Name:        req.Name,
		Description: req.Description,
		Conditions:  req.Conditions,
	}

	if err := s.repo.Create(ctx, segment); err != nil {
		return nil, err
	}

	return segment, nil
}

func (s *service) GetByID(ctx context.Context, projectID, segmentID uuid.UUID) (*SegmentWithCount, error) {
	segment, err := s.repo.GetByID(ctx, projectID, segmentID)
	if err != nil {
		return nil, err
	}
	if segment == nil {
		return nil, nil
	}

	result := &SegmentWithCount{
		Segment: *segment,
	}

	// Try cache for user count
	if s.cache != nil {
		cacheKey := fmt.Sprintf("segment:%s:count", segmentID.String())
		cached, err := s.cache.Get(ctx, cacheKey)
		if err == nil && cached != "" {
			var count int64
			if err := json.Unmarshal([]byte(cached), &count); err == nil {
				result.UserCount = count
				return result, nil
			}
		}
	}

	// Fall back to DB count
	count, err := s.repo.CountUsers(ctx, segmentID)
	if err != nil {
		return nil, err
	}
	result.UserCount = count

	return result, nil
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
		return nil, fmt.Errorf("not_found")
	}

	if req.Name != nil {
		segment.Name = *req.Name
	}
	if req.Description != nil {
		segment.Description = *req.Description
	}
	if req.Conditions != nil {
		if err := validateConditions(req.Conditions); err != nil {
			return nil, fmt.Errorf("validation: %w", err)
		}
		segment.Conditions = req.Conditions
	}

	if err := s.repo.Update(ctx, segment); err != nil {
		return nil, err
	}

	// Invalidate cache
	if s.cache != nil {
		s.cache.Delete(ctx, fmt.Sprintf("segment:%s:count", segmentID.String()))
		s.cache.Delete(ctx, fmt.Sprintf("segment:%s:users", segmentID.String()))
	}

	return segment, nil
}

func (s *service) Delete(ctx context.Context, projectID, segmentID uuid.UUID) error {
	if err := s.repo.Delete(ctx, projectID, segmentID); err != nil {
		return err
	}

	// Clean cache
	if s.cache != nil {
		s.cache.Delete(ctx, fmt.Sprintf("segment:%s:count", segmentID.String()))
		s.cache.Delete(ctx, fmt.Sprintf("segment:%s:users", segmentID.String()))
	}

	return nil
}

func (s *service) GetUsers(ctx context.Context, segmentID uuid.UUID, params models.CursorParams) ([]*models.User, string, error) {
	return s.repo.GetUsers(ctx, segmentID, params)
}

func validateConditions(conditions map[string]interface{}) error {
	op, ok := conditions["operator"].(string)
	if !ok || (op != "AND" && op != "OR") {
		return fmt.Errorf("conditions.operator must be AND or OR")
	}

	rules, ok := conditions["rules"].([]interface{})
	if !ok || len(rules) == 0 {
		return fmt.Errorf("conditions.rules must be a non-empty array")
	}

	for i, rule := range rules {
		ruleMap, ok := rule.(map[string]interface{})
		if !ok {
			return fmt.Errorf("rule %d must be an object", i)
		}

		ruleType, _ := ruleMap["type"].(string)
		switch ruleType {
		case "event":
			if _, ok := ruleMap["event_name"].(string); !ok {
				return fmt.Errorf("rule %d: event rule requires event_name", i)
			}
		case "user_property":
			if _, ok := ruleMap["property"].(string); !ok {
				return fmt.Errorf("rule %d: user_property rule requires property", i)
			}
			if _, ok := ruleMap["operator"].(string); !ok {
				return fmt.Errorf("rule %d: user_property rule requires operator", i)
			}
		default:
			return fmt.Errorf("rule %d: unknown type %q, must be event or user_property", i, ruleType)
		}
	}

	return nil
}
