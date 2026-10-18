package projects

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/opentap/opentap/internal/models"
)

type Repository interface {
	Create(ctx context.Context, project *models.Project) error
	GetByID(ctx context.Context, id uuid.UUID) (*models.Project, error)
	List(ctx context.Context) ([]*models.Project, error)
}

type postgresRepository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) Repository {
	return &postgresRepository{pool: pool}
}

func (r *postgresRepository) Create(ctx context.Context, project *models.Project) error {
	return r.pool.QueryRow(ctx,
		`INSERT INTO projects (name, created_at, updated_at) VALUES ($1, NOW(), NOW()) RETURNING id, created_at, updated_at`,
		project.Name,
	).Scan(&project.ID, &project.CreatedAt, &project.UpdatedAt)
}

func (r *postgresRepository) GetByID(ctx context.Context, id uuid.UUID) (*models.Project, error) {
	p := &models.Project{}
	err := r.pool.QueryRow(ctx,
		`SELECT id, name, created_at, updated_at FROM projects WHERE id = $1`,
		id,
	).Scan(&p.ID, &p.Name, &p.CreatedAt, &p.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return p, err
}

func (r *postgresRepository) List(ctx context.Context) ([]*models.Project, error) {
	rows, _ := r.pool.Query(ctx,
		`SELECT id, name, created_at, updated_at FROM projects ORDER BY created_at DESC`,
	)
	return pgx.CollectRows(rows, pgx.RowToAddrOfStructByPos[models.Project])
}
