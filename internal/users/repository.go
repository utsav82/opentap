package users

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/opentap/opentap/internal/models"
)

type Repository interface {
	Upsert(ctx context.Context, projectID uuid.UUID, userID string, timestamp time.Time) error
	IncrementEventCount(ctx context.Context, projectID uuid.UUID, userID string) error
	GetByUserID(ctx context.Context, projectID uuid.UUID, userID string) (*models.User, error)
	GetByID(ctx context.Context, projectID, id uuid.UUID) (*models.User, error)
	List(ctx context.Context, projectID uuid.UUID, params models.CursorParams) ([]*models.User, string, error)
}

type postgresRepository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) Repository {
	return &postgresRepository{pool: pool}
}

func (r *postgresRepository) Upsert(ctx context.Context, projectID uuid.UUID, userID string, timestamp time.Time) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO users (project_id, user_id, first_seen, last_seen, created_at, updated_at)
		 VALUES ($1, $2, $3, $3, NOW(), NOW())
		 ON CONFLICT (project_id, user_id) DO UPDATE SET
		   last_seen = GREATEST(users.last_seen, EXCLUDED.last_seen),
		   updated_at = NOW()`,
		projectID, userID, timestamp,
	)
	return err
}

func (r *postgresRepository) IncrementEventCount(ctx context.Context, projectID uuid.UUID, userID string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE users SET event_count = event_count + 1, updated_at = NOW()
		 WHERE project_id = $1 AND user_id = $2`,
		projectID, userID,
	)
	return err
}

func (r *postgresRepository) GetByUserID(ctx context.Context, projectID uuid.UUID, userID string) (*models.User, error) {
	user := &models.User{}
	err := r.pool.QueryRow(ctx,
		`SELECT id, project_id, user_id, properties, first_seen, last_seen, event_count, created_at, updated_at
		 FROM users WHERE project_id = $1 AND user_id = $2`,
		projectID, userID,
	).Scan(&user.ID, &user.ProjectID, &user.UserID, &user.Properties, &user.FirstSeen, &user.LastSeen, &user.EventCount, &user.CreatedAt, &user.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return user, err
}

func (r *postgresRepository) GetByID(ctx context.Context, projectID, id uuid.UUID) (*models.User, error) {
	user := &models.User{}
	err := r.pool.QueryRow(ctx,
		`SELECT id, project_id, user_id, properties, first_seen, last_seen, event_count, created_at, updated_at
		 FROM users WHERE project_id = $1 AND id = $2`,
		projectID, id,
	).Scan(&user.ID, &user.ProjectID, &user.UserID, &user.Properties, &user.FirstSeen, &user.LastSeen, &user.EventCount, &user.CreatedAt, &user.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return user, err
}

func (r *postgresRepository) List(ctx context.Context, projectID uuid.UUID, params models.CursorParams) ([]*models.User, string, error) {
	args := []interface{}{projectID}
	conditions := []string{"project_id = $1"}
	argIdx := 2

	if params.Cursor != "" {
		ts, id, err := decodeUserCursor(params.Cursor)
		if err == nil {
			conditions = append(conditions, fmt.Sprintf("(last_seen, id) < ($%d, $%d)", argIdx, argIdx+1))
			args = append(args, ts, id)
			argIdx += 2
		}
	}

	limit := params.Limit
	if limit <= 0 || limit > 100 {
		limit = 50
	}

	query := fmt.Sprintf(
		`SELECT id, project_id, user_id, properties, first_seen, last_seen, event_count, created_at, updated_at
		 FROM users WHERE %s ORDER BY last_seen DESC, id DESC LIMIT $%d`,
		strings.Join(conditions, " AND "), argIdx,
	)
	args = append(args, limit+1)

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()

	var users []*models.User
	for rows.Next() {
		u := &models.User{}
		if err := rows.Scan(&u.ID, &u.ProjectID, &u.UserID, &u.Properties, &u.FirstSeen, &u.LastSeen, &u.EventCount, &u.CreatedAt, &u.UpdatedAt); err != nil {
			return nil, "", err
		}
		users = append(users, u)
	}

	var nextCursor string
	if len(users) > limit {
		users = users[:limit]
		last := users[limit-1]
		nextCursor = encodeUserCursor(last.LastSeen, last.ID)
	}

	return users, nextCursor, nil
}

func encodeUserCursor(ts time.Time, id uuid.UUID) string {
	raw := fmt.Sprintf("%s:%s", ts.Format(time.RFC3339Nano), id.String())
	return base64.StdEncoding.EncodeToString([]byte(raw))
}

func decodeUserCursor(cursor string) (time.Time, uuid.UUID, error) {
	raw, err := base64.StdEncoding.DecodeString(cursor)
	if err != nil {
		return time.Time{}, uuid.Nil, err
	}
	parts := strings.SplitN(string(raw), ":", 2)
	if len(parts) != 2 {
		return time.Time{}, uuid.Nil, fmt.Errorf("invalid cursor format")
	}
	ts, err := time.Parse(time.RFC3339Nano, parts[0])
	if err != nil {
		return time.Time{}, uuid.Nil, err
	}
	id, err := uuid.Parse(parts[1])
	if err != nil {
		return time.Time{}, uuid.Nil, err
	}
	return ts, id, nil
}
