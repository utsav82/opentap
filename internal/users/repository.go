package users

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/opentap/opentap/internal/models"
	"github.com/opentap/opentap/internal/pagination"
)

const userColumns = `id, project_id, user_id, properties, first_seen, last_seen, event_count, created_at, updated_at`

type Repository interface {
	GetByID(ctx context.Context, projectID, id uuid.UUID) (*models.User, error)
	List(ctx context.Context, projectID uuid.UUID, params models.CursorParams) ([]*models.User, string, error)
	Identify(ctx context.Context, projectID uuid.UUID, anonymousID, userID string) (int64, error)
}

type postgresRepository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) Repository {
	return &postgresRepository{pool: pool}
}

func (r *postgresRepository) GetByID(ctx context.Context, projectID, id uuid.UUID) (*models.User, error) {
	rows, _ := r.pool.Query(ctx, `SELECT `+userColumns+` FROM users WHERE project_id = $1 AND id = $2`, projectID, id)
	user, err := pgx.CollectOneRow(rows, pgx.RowToAddrOfStructByPos[models.User])
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return user, err
}

func (r *postgresRepository) List(ctx context.Context, projectID uuid.UUID, params models.CursorParams) ([]*models.User, string, error) {
	args := []interface{}{projectID}
	cursor := ""
	if ts, id, ok := pagination.Decode(params.Cursor); ok {
		args = append(args, ts, id)
		cursor = "AND (last_seen, id) < ($2, $3)"
	}

	rows, _ := r.pool.Query(ctx,
		fmt.Sprintf(`SELECT `+userColumns+` FROM users WHERE project_id = $1 %s ORDER BY last_seen DESC, id DESC LIMIT $%d`,
			cursor, len(args)+1),
		append(args, params.Limit+1)...,
	)
	users, err := pgx.CollectRows(rows, pgx.RowToAddrOfStructByPos[models.User])
	if err != nil {
		return nil, "", err
	}
	users, next := pagination.Page(users, params.Limit, func(u *models.User) (time.Time, uuid.UUID) { return u.LastSeen, u.ID })
	return users, next, nil
}

// Identify merges the anonymous user's history and profile into userID and records the alias.
// It takes the same per-user lock as event ingestion so in-flight events are never orphaned.
func (r *postgresRepository) Identify(ctx context.Context, projectID uuid.UUID, anonymousID, userID string) (int64, error) {
	var merged int64
	batch := &pgx.Batch{}
	batch.Queue(`SELECT pg_advisory_xact_lock(hashtextextended($1::text || ':' || $2, 0))`, projectID, anonymousID)
	batch.Queue(`INSERT INTO user_aliases (project_id, alias, user_id) VALUES ($1, $2, $3)
		ON CONFLICT (project_id, alias) DO UPDATE SET user_id = EXCLUDED.user_id`, projectID, anonymousID, userID)
	batch.Queue(`UPDATE user_aliases SET user_id = $3 WHERE project_id = $1 AND user_id = $2`, projectID, anonymousID, userID)
	batch.Queue(`INSERT INTO users (project_id, user_id, properties, first_seen, last_seen, event_count)
		SELECT project_id, $3, properties, first_seen, last_seen, event_count FROM users WHERE project_id = $1 AND user_id = $2
		ON CONFLICT (project_id, user_id) DO UPDATE SET
		  properties = EXCLUDED.properties || users.properties,
		  first_seen = LEAST(users.first_seen, EXCLUDED.first_seen),
		  last_seen = GREATEST(users.last_seen, EXCLUDED.last_seen),
		  event_count = users.event_count + EXCLUDED.event_count,
		  updated_at = NOW()`, projectID, anonymousID, userID)
	batch.Queue(`DELETE FROM users WHERE project_id = $1 AND user_id = $2`, projectID, anonymousID)
	batch.Queue(`UPDATE events SET user_id = $3 WHERE project_id = $1 AND user_id = $2`, projectID, anonymousID, userID).
		Exec(func(ct pgconn.CommandTag) error {
			merged = ct.RowsAffected()
			return nil
		})

	err := pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		return tx.SendBatch(ctx, batch).Close()
	})
	return merged, err
}
