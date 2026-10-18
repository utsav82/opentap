package segments

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/opentap/opentap/internal/models"
	"github.com/opentap/opentap/internal/pagination"
)

const segmentColumns = `id, project_id, name, description, conditions, created_at, updated_at, last_computed_at, webhook_url`

type Repository interface {
	Create(ctx context.Context, segment *models.Segment) error
	GetByID(ctx context.Context, projectID, segmentID uuid.UUID) (*models.Segment, error)
	List(ctx context.Context, projectID uuid.UUID, params models.CursorParams) ([]*models.Segment, string, error)
	Update(ctx context.Context, segment *models.Segment) error
	Delete(ctx context.Context, projectID, segmentID uuid.UUID) error
	ListAll(ctx context.Context) ([]*models.Segment, error)
	Recompute(ctx context.Context, segment *models.Segment) (*Change, error)
	GetUsers(ctx context.Context, segmentID uuid.UUID, params models.CursorParams) ([]*models.User, string, error)
	EachUser(ctx context.Context, projectID, segmentID uuid.UUID, fn func(*models.User) error) error
	CountUsers(ctx context.Context, segmentID uuid.UUID) (int64, error)
}

// Change is the result of a recompute: the new size and which external user IDs entered or left.
type Change struct {
	UserCount int64
	Added     []string
	Removed   []string
}

type postgresRepository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) Repository {
	return &postgresRepository{pool: pool}
}

func (r *postgresRepository) Create(ctx context.Context, segment *models.Segment) error {
	return r.pool.QueryRow(ctx,
		`INSERT INTO segments (project_id, name, description, conditions, webhook_url, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, NOW(), NOW()) RETURNING id, created_at, updated_at`,
		segment.ProjectID, segment.Name, segment.Description, segment.Conditions, segment.WebhookURL,
	).Scan(&segment.ID, &segment.CreatedAt, &segment.UpdatedAt)
}

func (r *postgresRepository) GetByID(ctx context.Context, projectID, segmentID uuid.UUID) (*models.Segment, error) {
	rows, _ := r.pool.Query(ctx, `SELECT `+segmentColumns+` FROM segments WHERE id = $1 AND project_id = $2`, segmentID, projectID)
	seg, err := pgx.CollectOneRow(rows, pgx.RowToAddrOfStructByPos[models.Segment])
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return seg, err
}

func (r *postgresRepository) List(ctx context.Context, projectID uuid.UUID, params models.CursorParams) ([]*models.Segment, string, error) {
	args := []interface{}{projectID}
	cursor := ""
	if ts, id, ok := pagination.Decode(params.Cursor); ok {
		args = append(args, ts, id)
		cursor = "AND (created_at, id) < ($2, $3)"
	}

	rows, _ := r.pool.Query(ctx,
		fmt.Sprintf(`SELECT `+segmentColumns+` FROM segments WHERE project_id = $1 %s ORDER BY created_at DESC, id DESC LIMIT $%d`,
			cursor, len(args)+1),
		append(args, params.Limit+1)...,
	)
	segments, err := pgx.CollectRows(rows, pgx.RowToAddrOfStructByPos[models.Segment])
	if err != nil {
		return nil, "", err
	}
	segments, next := pagination.Page(segments, params.Limit, func(s *models.Segment) (time.Time, uuid.UUID) { return s.CreatedAt, s.ID })
	return segments, next, nil
}

func (r *postgresRepository) Update(ctx context.Context, segment *models.Segment) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE segments SET name = $1, description = $2, conditions = $3, webhook_url = $4, updated_at = NOW()
		 WHERE id = $5 AND project_id = $6`,
		segment.Name, segment.Description, segment.Conditions, segment.WebhookURL, segment.ID, segment.ProjectID,
	)
	return err
}

func (r *postgresRepository) Delete(ctx context.Context, projectID, segmentID uuid.UUID) error {
	ct, err := r.pool.Exec(ctx,
		`DELETE FROM segments WHERE id = $1 AND project_id = $2`,
		segmentID, projectID,
	)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return fmt.Errorf("segment %w", models.ErrNotFound)
	}
	return nil
}

func (r *postgresRepository) ListAll(ctx context.Context) ([]*models.Segment, error) {
	rows, _ := r.pool.Query(ctx, `SELECT `+segmentColumns+` FROM segments`)
	return pgx.CollectRows(rows, pgx.RowToAddrOfStructByPos[models.Segment])
}

// Recompute syncs segment_users with the segment's conditions in one statement,
// keeping entered_at for users who still match.
func (r *postgresRepository) Recompute(ctx context.Context, segment *models.Segment) (*Change, error) {
	query, args, err := BuildQuery(segment.ProjectID, segment.Conditions)
	if err != nil {
		return nil, err
	}
	seg := fmt.Sprintf("$%d", len(args)+1)

	change := &Change{}
	err = r.pool.QueryRow(ctx, fmt.Sprintf(`
		WITH matched AS (%[1]s),
		removed AS (
			DELETE FROM segment_users WHERE segment_id = %[2]s AND user_id NOT IN (SELECT id FROM matched)
			RETURNING user_id
		),
		added AS (
			INSERT INTO segment_users (segment_id, user_id) SELECT %[2]s, id FROM matched ON CONFLICT DO NOTHING
			RETURNING user_id
		),
		stamped AS (
			UPDATE segments SET last_computed_at = NOW() WHERE id = %[2]s
		)
		SELECT (SELECT COUNT(*) FROM matched),
		       ARRAY(SELECT u.user_id FROM added a JOIN users u ON u.id = a.user_id),
		       ARRAY(SELECT u.user_id FROM removed r JOIN users u ON u.id = r.user_id)`, query, seg),
		append(args, segment.ID)...,
	).Scan(&change.UserCount, &change.Added, &change.Removed)
	return change, err
}

func (r *postgresRepository) GetUsers(ctx context.Context, segmentID uuid.UUID, params models.CursorParams) ([]*models.User, string, error) {
	args := []interface{}{segmentID}
	cursor := ""
	if ts, id, ok := pagination.Decode(params.Cursor); ok {
		args = append(args, ts, id)
		cursor = "AND (u.last_seen, u.id) < ($2, $3)"
	}

	rows, _ := r.pool.Query(ctx,
		fmt.Sprintf(`SELECT u.id, u.project_id, u.user_id, u.properties, u.first_seen, u.last_seen, u.event_count, u.created_at, u.updated_at
		 FROM segment_users su JOIN users u ON su.user_id = u.id
		 WHERE su.segment_id = $1 %s ORDER BY u.last_seen DESC, u.id DESC LIMIT $%d`,
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

func (r *postgresRepository) EachUser(ctx context.Context, projectID, segmentID uuid.UUID, fn func(*models.User) error) error {
	rows, _ := r.pool.Query(ctx,
		`SELECT u.id, u.project_id, u.user_id, u.properties, u.first_seen, u.last_seen, u.event_count, u.created_at, u.updated_at
		 FROM segment_users su
		 JOIN segments s ON s.id = su.segment_id AND s.project_id = $2
		 JOIN users u ON u.id = su.user_id
		 WHERE su.segment_id = $1 ORDER BY u.user_id`,
		segmentID, projectID,
	)
	defer rows.Close()
	for rows.Next() {
		user, err := pgx.RowToAddrOfStructByPos[models.User](rows)
		if err != nil {
			return err
		}
		if err := fn(user); err != nil {
			return err
		}
	}
	return rows.Err()
}

func (r *postgresRepository) CountUsers(ctx context.Context, segmentID uuid.UUID) (int64, error) {
	var count int64
	err := r.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM segment_users WHERE segment_id = $1`,
		segmentID,
	).Scan(&count)
	return count, err
}
