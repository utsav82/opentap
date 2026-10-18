package users

import (
	"context"
	"fmt"

	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
	"github.com/opentap/opentap/internal/models"
)

type Service interface {
	GetUser(ctx context.Context, projectID, id uuid.UUID) (*models.User, error)
	ListUsers(ctx context.Context, projectID uuid.UUID, params models.CursorParams) ([]*models.User, string, error)
	Identify(ctx context.Context, projectID uuid.UUID, req models.IdentifyRequest) (int64, error)
}

type service struct {
	repo     Repository
	validate *validator.Validate
}

func NewService(repo Repository) Service {
	return &service{repo: repo, validate: validator.New()}
}

func (s *service) GetUser(ctx context.Context, projectID, id uuid.UUID) (*models.User, error) {
	return s.repo.GetByID(ctx, projectID, id)
}

func (s *service) ListUsers(ctx context.Context, projectID uuid.UUID, params models.CursorParams) ([]*models.User, string, error) {
	return s.repo.List(ctx, projectID, params)
}

func (s *service) Identify(ctx context.Context, projectID uuid.UUID, req models.IdentifyRequest) (int64, error) {
	if err := s.validate.Struct(req); err != nil {
		return 0, fmt.Errorf("%w: %v", models.ErrValidation, err)
	}
	return s.repo.Identify(ctx, projectID, req.AnonymousID, req.UserID)
}
