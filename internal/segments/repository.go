package segments

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
	Create(ctx context.Context, segment *models.Segment) error
	GetByID(ctx context.Context, projectID, segmentID uuid.UUID) (*models.Segment, error)
	List(ctx context.Context, projectID uuid.UUID, params models.CursorParams) ([]*models.Segment, string, error)
	Update(ctx context.Context, segment *models.Segment) error
	Delete(ctx context.Context, projectID, segmentID uuid.UUID) error
	ListAll(ctx context.Context) ([]*models.Segment, error)
	ReplaceUsers(ctx context.Context, segmentID uuid.UUID, userIDs []uuid.UUID) error
	GetUsers(ctx context.Context, segmentID uuid.UUID, params models.CursorParams) ([]*models.User, string, error)
	CountUsers(ctx context.Context, segmentID uuid.UUID) (int64, error)
}

type postgresRepository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) Repository {
	return &postgresRepository{pool: pool}
}

func (r *postgresRepository) Create(ctx context.Context, segment *models.Segment) error {
	return r.pool.QueryRow(ctx,
		`INSERT INTO segments (project_id, name, description, conditions, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, NOW(), NOW()) RETURNING id, created_at, updated_at`,
		segment.ProjectID, segment.Name, segment.Description, segment.Conditions,
	).Scan(&segment.ID, &segment.CreatedAt, &segment.UpdatedAt)
}

func (r *postgresRepository) GetByID(ctx context.Context, projectID, segmentID uuid.UUID) (*models.Segment, error) {
	seg := &models.Segment{}
	err := r.pool.QueryRow(ctx,
		`SELECT id, project_id, name, description, conditions, created_at, updated_at
		 FROM segments WHERE id = $1 AND project_id = $2`,
		segmentID, projectID,
	).Scan(&seg.ID, &seg.ProjectID, &seg.Name, &seg.Description, &seg.Conditions, &seg.CreatedAt, &seg.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return seg, err
}

func (r *postgresRepository) List(ctx context.Context, projectID uuid.UUID, params models.CursorParams) ([]*models.Segment, string, error) {
	args := []interface{}{projectID}
	conditions := []string{"project_id = $1"}
	argIdx := 2

	if params.Cursor != "" {
		ts, id, err := decodeSegmentCursor(params.Cursor)
		if err == nil {
			conditions = append(conditions, fmt.Sprintf("(created_at, id) < ($%d, $%d)", argIdx, argIdx+1))
			args = append(args, ts, id)
			argIdx += 2
		}
	}

	limit := params.Limit
	if limit <= 0 || limit > 100 {
		limit = 50
	}

	query := fmt.Sprintf(
		`SELECT id, project_id, name, description, conditions, created_at, updated_at
		 FROM segments WHERE %s ORDER BY created_at DESC, id DESC LIMIT $%d`,
		strings.Join(conditions, " AND "), argIdx,
	)
	args = append(args, limit+1)

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()

	var segments []*models.Segment
	for rows.Next() {
		s := &models.Segment{}
		if err := rows.Scan(&s.ID, &s.ProjectID, &s.Name, &s.Description, &s.Conditions, &s.CreatedAt, &s.UpdatedAt); err != nil {
			return nil, "", err
		}
		segments = append(segments, s)
	}

	var nextCursor string
	if len(segments) > limit {
		segments = segments[:limit]
		last := segments[limit-1]
		nextCursor = encodeSegmentCursor(last.CreatedAt, last.ID)
	}

	return segments, nextCursor, nil
}

func (r *postgresRepository) Update(ctx context.Context, segment *models.Segment) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE segments SET name = $1, description = $2, conditions = $3, updated_at = NOW()
		 WHERE id = $4 AND project_id = $5`,
		segment.Name, segment.Description, segment.Conditions, segment.ID, segment.ProjectID,
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
		return fmt.Errorf("segment not found")
	}
	return nil
}

func (r *postgresRepository) ListAll(ctx context.Context) ([]*models.Segment, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, project_id, name, description, conditions, created_at, updated_at FROM segments`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var segments []*models.Segment
	for rows.Next() {
		s := &models.Segment{}
		if err := rows.Scan(&s.ID, &s.ProjectID, &s.Name, &s.Description, &s.Conditions, &s.CreatedAt, &s.UpdatedAt); err != nil {
			return nil, err
		}
		segments = append(segments, s)
	}
	return segments, nil
}

func (r *postgresRepository) ReplaceUsers(ctx context.Context, segmentID uuid.UUID, userIDs []uuid.UUID) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	_, err = tx.Exec(ctx, `DELETE FROM segment_users WHERE segment_id = $1`, segmentID)
	if err != nil {
		return err
	}

	if len(userIDs) > 0 {
		batch := &pgx.Batch{}
		for _, uid := range userIDs {
			batch.Queue(
				`INSERT INTO segment_users (segment_id, user_id, entered_at) VALUES ($1, $2, NOW())`,
				segmentID, uid,
			)
		}
		br := tx.SendBatch(ctx, batch)
		for range userIDs {
			if _, err := br.Exec(); err != nil {
				br.Close()
				return err
			}
		}
		br.Close()
	}

	return tx.Commit(ctx)
}

func (r *postgresRepository) GetUsers(ctx context.Context, segmentID uuid.UUID, params models.CursorParams) ([]*models.User, string, error) {
	args := []interface{}{segmentID}
	conditions := []string{"su.segment_id = $1"}
	argIdx := 2

	if params.Cursor != "" {
		ts, id, err := decodeSegmentCursor(params.Cursor)
		if err == nil {
			conditions = append(conditions, fmt.Sprintf("(u.last_seen, u.id) < ($%d, $%d)", argIdx, argIdx+1))
			args = append(args, ts, id)
			argIdx += 2
		}
	}

	limit := params.Limit
	if limit <= 0 || limit > 100 {
		limit = 50
	}

	query := fmt.Sprintf(
		`SELECT u.id, u.project_id, u.user_id, u.properties, u.first_seen, u.last_seen, u.event_count, u.created_at, u.updated_at
		 FROM segment_users su JOIN users u ON su.user_id = u.id
		 WHERE %s ORDER BY u.last_seen DESC, u.id DESC LIMIT $%d`,
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
		nextCursor = encodeSegmentCursor(last.LastSeen, last.ID)
	}

	return users, nextCursor, nil
}

func (r *postgresRepository) CountUsers(ctx context.Context, segmentID uuid.UUID) (int64, error) {
	var count int64
	err := r.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM segment_users WHERE segment_id = $1`,
		segmentID,
	).Scan(&count)
	return count, err
}

// QueryExecutor provides raw query execution for the segment worker.
type QueryExecutor interface {
	ExecuteQuery(ctx context.Context, query string, args []interface{}) ([]uuid.UUID, error)
}

// ExecuteQuery runs a raw SQL query that returns user UUIDs.
func (r *postgresRepository) ExecuteQuery(ctx context.Context, query string, args []interface{}) ([]uuid.UUID, error) {
	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var userIDs []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		userIDs = append(userIDs, id)
	}
	return userIDs, nil
}

func encodeSegmentCursor(ts time.Time, id uuid.UUID) string {
	raw := fmt.Sprintf("%s:%s", ts.Format(time.RFC3339Nano), id.String())
	return base64.StdEncoding.EncodeToString([]byte(raw))
}

func decodeSegmentCursor(cursor string) (time.Time, uuid.UUID, error) {
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
