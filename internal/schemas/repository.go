package schemas

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/opentap/opentap/internal/models"
)

type Repository interface {
	Upsert(ctx context.Context, schema *models.EventSchema) error
	UpsertBatch(ctx context.Context, schemas []*models.EventSchema) error
	ListByProject(ctx context.Context, projectID uuid.UUID) ([]*models.EventSchema, error)
	ListByEvent(ctx context.Context, projectID uuid.UUID, eventName string) ([]*models.EventSchema, error)
}

type postgresRepository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) Repository {
	return &postgresRepository{pool: pool}
}

func (r *postgresRepository) Upsert(ctx context.Context, schema *models.EventSchema) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO event_schemas (project_id, event_name, property_name, property_type, distinct_values_count, first_seen, last_seen)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 ON CONFLICT (project_id, event_name, property_name) DO UPDATE SET
		   property_type = EXCLUDED.property_type,
		   distinct_values_count = GREATEST(event_schemas.distinct_values_count, EXCLUDED.distinct_values_count),
		   last_seen = GREATEST(event_schemas.last_seen, EXCLUDED.last_seen)`,
		schema.ProjectID, schema.EventName, schema.PropertyName, schema.PropertyType, schema.DistinctValuesCount, schema.FirstSeen, schema.LastSeen,
	)
	return err
}

func (r *postgresRepository) UpsertBatch(ctx context.Context, schemas []*models.EventSchema) error {
	if len(schemas) == 0 {
		return nil
	}
	batch := &pgx.Batch{}
	for _, s := range schemas {
		batch.Queue(
			`INSERT INTO event_schemas (project_id, event_name, property_name, property_type, distinct_values_count, first_seen, last_seen)
			 VALUES ($1, $2, $3, $4, $5, $6, $7)
			 ON CONFLICT (project_id, event_name, property_name) DO UPDATE SET
			   property_type = EXCLUDED.property_type,
			   distinct_values_count = GREATEST(event_schemas.distinct_values_count, EXCLUDED.distinct_values_count),
			   last_seen = GREATEST(event_schemas.last_seen, EXCLUDED.last_seen)`,
			s.ProjectID, s.EventName, s.PropertyName, s.PropertyType, s.DistinctValuesCount, s.FirstSeen, s.LastSeen,
		)
	}
	br := r.pool.SendBatch(ctx, batch)
	defer br.Close()
	for range schemas {
		if _, err := br.Exec(); err != nil {
			return err
		}
	}
	return nil
}

func (r *postgresRepository) ListByProject(ctx context.Context, projectID uuid.UUID) ([]*models.EventSchema, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, project_id, event_name, property_name, property_type, distinct_values_count, first_seen, last_seen
		 FROM event_schemas WHERE project_id = $1 ORDER BY event_name, property_name`,
		projectID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var schemas []*models.EventSchema
	for rows.Next() {
		s := &models.EventSchema{}
		if err := rows.Scan(&s.ID, &s.ProjectID, &s.EventName, &s.PropertyName, &s.PropertyType, &s.DistinctValuesCount, &s.FirstSeen, &s.LastSeen); err != nil {
			return nil, err
		}
		schemas = append(schemas, s)
	}
	return schemas, nil
}

func (r *postgresRepository) ListByEvent(ctx context.Context, projectID uuid.UUID, eventName string) ([]*models.EventSchema, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, project_id, event_name, property_name, property_type, distinct_values_count, first_seen, last_seen
		 FROM event_schemas WHERE project_id = $1 AND event_name = $2 ORDER BY property_name`,
		projectID, eventName,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var schemas []*models.EventSchema
	for rows.Next() {
		s := &models.EventSchema{}
		if err := rows.Scan(&s.ID, &s.ProjectID, &s.EventName, &s.PropertyName, &s.PropertyType, &s.DistinctValuesCount, &s.FirstSeen, &s.LastSeen); err != nil {
			return nil, err
		}
		schemas = append(schemas, s)
	}
	return schemas, nil
}
