package schemas

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/opentap/opentap/internal/models"
)

type EventSummary struct {
	EventName     string    `json:"event_name"`
	PropertyCount int       `json:"property_count"`
	FirstSeen     time.Time `json:"first_seen"`
	LastSeen      time.Time `json:"last_seen"`
}

type Repository interface {
	UpsertBatch(ctx context.Context, schemas []*models.EventSchema) error
	ListEventTypes(ctx context.Context, projectID uuid.UUID) ([]EventSummary, error)
	ListByEvent(ctx context.Context, projectID uuid.UUID, eventName string) ([]*models.EventSchema, error)
}

type postgresRepository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) Repository {
	return &postgresRepository{pool: pool}
}

func (r *postgresRepository) UpsertBatch(ctx context.Context, schemas []*models.EventSchema) error {
	batch := &pgx.Batch{}
	for _, s := range schemas {
		batch.Queue(
			`INSERT INTO event_schemas (project_id, event_name, property_name, property_type, distinct_values_count, first_seen, last_seen)
			 VALUES ($1, $2, $3, $4, $5, $6, $7)
			 ON CONFLICT (project_id, event_name, property_name) DO UPDATE SET
			   property_type = CASE
			     WHEN EXCLUDED.property_type IN (event_schemas.property_type, 'null') THEN event_schemas.property_type
			     WHEN event_schemas.property_type = 'null' THEN EXCLUDED.property_type
			     ELSE 'mixed'
			   END,
			   distinct_values_count = GREATEST(event_schemas.distinct_values_count, EXCLUDED.distinct_values_count),
			   last_seen = GREATEST(event_schemas.last_seen, EXCLUDED.last_seen)`,
			s.ProjectID, s.EventName, s.PropertyName, s.PropertyType, s.DistinctValuesCount, s.FirstSeen, s.LastSeen,
		)
	}
	return r.pool.SendBatch(ctx, batch).Close()
}

func (r *postgresRepository) ListEventTypes(ctx context.Context, projectID uuid.UUID) ([]EventSummary, error) {
	rows, _ := r.pool.Query(ctx,
		`SELECT event_name, COUNT(*), MIN(first_seen), MAX(last_seen)
		 FROM event_schemas WHERE project_id = $1 GROUP BY event_name ORDER BY event_name`,
		projectID,
	)
	return pgx.CollectRows(rows, pgx.RowToStructByPos[EventSummary])
}

func (r *postgresRepository) ListByEvent(ctx context.Context, projectID uuid.UUID, eventName string) ([]*models.EventSchema, error) {
	rows, _ := r.pool.Query(ctx,
		`SELECT id, project_id, event_name, property_name, property_type, distinct_values_count, first_seen, last_seen
		 FROM event_schemas WHERE project_id = $1 AND event_name = $2 ORDER BY property_name`,
		projectID, eventName,
	)
	return pgx.CollectRows(rows, pgx.RowToAddrOfStructByPos[models.EventSchema])
}
