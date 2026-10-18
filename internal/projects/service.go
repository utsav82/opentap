package projects

import (
	"context"
	"fmt"

	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
	"github.com/opentap/opentap/internal/auth"
	"github.com/opentap/opentap/internal/models"
)

type Service interface {
	Create(ctx context.Context, req models.CreateProjectRequest) (*models.Project, error)
	GetByID(ctx context.Context, id uuid.UUID) (*models.Project, error)
	List(ctx context.Context) ([]*models.Project, error)
	CreateKey(ctx context.Context, projectID uuid.UUID, req models.CreateAPIKeyRequest) (string, *models.APIKey, error)
	ListKeys(ctx context.Context, projectID uuid.UUID) ([]*models.APIKey, error)
	RevokeKey(ctx context.Context, projectID, keyID uuid.UUID) error
}

type service struct {
	repo     Repository
	keys     auth.Repository
	validate *validator.Validate
}

func NewService(repo Repository, keys auth.Repository) Service {
	return &service{
		repo:     repo,
		keys:     keys,
		validate: validator.New(),
	}
}

func (s *service) Create(ctx context.Context, req models.CreateProjectRequest) (*models.Project, error) {
	if err := s.validate.Struct(req); err != nil {
		return nil, fmt.Errorf("%w: %v", models.ErrValidation, err)
	}

	project := &models.Project{
		Name: req.Name,
	}

	if err := s.repo.Create(ctx, project); err != nil {
		return nil, err
	}

	return project, nil
}

func (s *service) GetByID(ctx context.Context, id uuid.UUID) (*models.Project, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *service) List(ctx context.Context) ([]*models.Project, error) {
	return s.repo.List(ctx)
}

func (s *service) CreateKey(ctx context.Context, projectID uuid.UUID, req models.CreateAPIKeyRequest) (string, *models.APIKey, error) {
	if err := s.validate.Struct(req); err != nil {
		return "", nil, fmt.Errorf("%w: %v", models.ErrValidation, err)
	}
	if req.Scope == "" {
		req.Scope = models.ScopeFull
	}
	project, err := s.repo.GetByID(ctx, projectID)
	if err != nil {
		return "", nil, err
	}
	if project == nil {
		return "", nil, fmt.Errorf("project %w", models.ErrNotFound)
	}
	return s.keys.CreateKey(ctx, projectID, req.Label, req.Scope)
}

func (s *service) ListKeys(ctx context.Context, projectID uuid.UUID) ([]*models.APIKey, error) {
	return s.keys.ListKeys(ctx, projectID)
}

func (s *service) RevokeKey(ctx context.Context, projectID, keyID uuid.UUID) error {
	return s.keys.RevokeKey(ctx, projectID, keyID)
}
