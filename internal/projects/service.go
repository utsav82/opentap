package projects

import (
	"context"
	"fmt"

	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
	"github.com/opentap/opentap/internal/models"
)

type Service interface {
	Create(ctx context.Context, req models.CreateProjectRequest) (*models.Project, error)
	GetByID(ctx context.Context, id uuid.UUID) (*models.Project, error)
	List(ctx context.Context) ([]*models.Project, error)
}

type service struct {
	repo     Repository
	validate *validator.Validate
}

func NewService(repo Repository) Service {
	return &service{
		repo:     repo,
		validate: validator.New(),
	}
}

func (s *service) Create(ctx context.Context, req models.CreateProjectRequest) (*models.Project, error) {
	if err := s.validate.Struct(req); err != nil {
		return nil, fmt.Errorf("validation: %w", err)
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
