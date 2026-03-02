package users

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/opentap/opentap/internal/models"
	"github.com/opentap/opentap/pkg/cache"
)

type Service interface {
	GetUser(ctx context.Context, projectID, id uuid.UUID) (*models.User, error)
	GetUserByUserID(ctx context.Context, projectID uuid.UUID, userID string) (*models.User, error)
	ListUsers(ctx context.Context, projectID uuid.UUID, params models.CursorParams) ([]*models.User, string, error)
}

type service struct {
	repo  Repository
	cache *cache.RedisClient
}

func NewService(repo Repository, cache *cache.RedisClient) Service {
	return &service{repo: repo, cache: cache}
}

func (s *service) GetUser(ctx context.Context, projectID, id uuid.UUID) (*models.User, error) {
	return s.repo.GetByID(ctx, projectID, id)
}

func (s *service) GetUserByUserID(ctx context.Context, projectID uuid.UUID, userID string) (*models.User, error) {
	// Try cache first
	if s.cache != nil {
		cacheKey := fmt.Sprintf("user:%s:%s", projectID.String(), userID)
		cached, err := s.cache.Get(ctx, cacheKey)
		if err == nil && cached != "" {
			var user models.User
			if err := json.Unmarshal([]byte(cached), &user); err == nil {
				return &user, nil
			}
		}
	}

	user, err := s.repo.GetByUserID(ctx, projectID, userID)
	if err != nil {
		return nil, err
	}
	return user, nil
}

func (s *service) ListUsers(ctx context.Context, projectID uuid.UUID, params models.CursorParams) ([]*models.User, string, error) {
	return s.repo.List(ctx, projectID, params)
}
