package events

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/opentap/opentap/internal/models"
	"github.com/opentap/opentap/internal/pagination"
)

const eventColumns = `id, project_id, user_id, event_name, properties, timestamp, received_at`

type Repository interface {
	Ingest(ctx context.Context, event *models.Event) error
	GetByID(ctx context.Context, projectID, eventID uuid.UUID) (*models.Event, error)
	List(ctx context.Context, projectID uuid.UUID, params models.CursorParams, filters EventFilters) ([]*models.Event, string, error)
	SetRetention(ctx context.Context, days int) error
}

type EventFilters struct {
	EventName string
	UserID    string
	Start     *time.Time
	End       *time.Time
}

type postgresRepository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) Repository {
	return &postgresRepository{pool: pool}
}

// Ingest stores the event and updates its user atomically. Redelivered events are ignored,
// and events sent under an identified anonymous ID are attributed to the known user.
func (r *postgresRepository) Ingest(ctx context.Context, event *models.Event) error {
	return pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1::text || ':' || $2, 0))`, event.ProjectID, event.UserID); err != nil {
			return err
		}
		err := tx.QueryRow(ctx, `SELECT user_id FROM user_aliases WHERE project_id = $1 AND alias = $2`, event.ProjectID, event.UserID).Scan(&event.UserID)
		if err != nil && err != pgx.ErrNoRows {
			return err
		}

		ct, err := tx.Exec(ctx,
			`INSERT INTO events (`+eventColumns+`) VALUES ($1, $2, $3, $4, $5, $6, $7) ON CONFLICT DO NOTHING`,
			event.ID, event.ProjectID, event.UserID, event.EventName, event.Properties, event.Timestamp, event.ReceivedAt,
		)
		if err != nil || ct.RowsAffected() == 0 {
			return err
		}
		userProperties := event.UserProperties
		if userProperties == nil {
			userProperties = map[string]interface{}{}
		}
		_, err = tx.Exec(ctx,
			`INSERT INTO users (project_id, user_id, properties, first_seen, last_seen, event_count)
			 VALUES ($1, $2, $4, $3, $3, 1)
			 ON CONFLICT (project_id, user_id) DO UPDATE SET
			   properties = users.properties || EXCLUDED.properties,
			   first_seen = LEAST(users.first_seen, EXCLUDED.first_seen),
			   last_seen = GREATEST(users.last_seen, EXCLUDED.last_seen),
			   event_count = users.event_count + 1,
			   updated_at = NOW()`,
			event.ProjectID, event.UserID, event.Timestamp, userProperties,
		)
		return err
	})
}

func (r *postgresRepository) GetByID(ctx context.Context, projectID, eventID uuid.UUID) (*models.Event, error) {
	rows, _ := r.pool.Query(ctx, `SELECT `+eventColumns+` FROM events WHERE id = $1 AND project_id = $2`, eventID, projectID)
	event, err := pgx.CollectOneRow(rows, pgx.RowToAddrOfStructByPos[models.Event])
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return event, err
}

func (r *postgresRepository) List(ctx context.Context, projectID uuid.UUID, params models.CursorParams, filters EventFilters) ([]*models.Event, string, error) {
	args := []interface{}{projectID}
	conditions := []string{"project_id = $1"}
	where := func(clause string, values ...interface{}) {
		for _, v := range values {
			args = append(args, v)
			clause = strings.Replace(clause, "?", fmt.Sprintf("$%d", len(args)), 1)
		}
		conditions = append(conditions, clause)
	}

	if filters.EventName != "" {
		where("event_name = ?", filters.EventName)
	}
	if filters.UserID != "" {
		where("user_id = ?", filters.UserID)
	}
	if filters.Start != nil {
		where("timestamp >= ?", *filters.Start)
	}
	if filters.End != nil {
		where("timestamp <= ?", *filters.End)
	}
	if ts, id, ok := pagination.Decode(params.Cursor); ok {
		where("(timestamp, id) < (?, ?)", ts, id)
	}

	rows, _ := r.pool.Query(ctx,
		fmt.Sprintf(`SELECT `+eventColumns+` FROM events WHERE %s ORDER BY timestamp DESC, id DESC LIMIT $%d`,
			strings.Join(conditions, " AND "), len(args)+1),
		append(args, params.Limit+1)...,
	)
	events, err := pgx.CollectRows(rows, pgx.RowToAddrOfStructByPos[models.Event])
	if err != nil {
		return nil, "", err
	}
	events, next := pagination.Page(events, params.Limit, func(e *models.Event) (time.Time, uuid.UUID) { return e.Timestamp, e.ID })
	return events, next, nil
}

func (r *postgresRepository) SetRetention(ctx context.Context, days int) error {
	return pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT remove_retention_policy('events', if_exists => TRUE)`); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `SELECT add_retention_policy('events', make_interval(days => $1))`, days)
		return err
	})
}
