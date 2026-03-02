package events

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

type TimeSeriesPoint struct {
	Timestamp time.Time `json:"timestamp"`
	Count     int64     `json:"count"`
}

type EventCount struct {
	EventName string `json:"event_name"`
	Count     int64  `json:"count"`
}

type Repository interface {
	Insert(ctx context.Context, event *models.Event) error
	InsertBatch(ctx context.Context, events []*models.Event) error
	GetByID(ctx context.Context, projectID, eventID uuid.UUID) (*models.Event, error)
	List(ctx context.Context, projectID uuid.UUID, params models.CursorParams, filters EventFilters) ([]*models.Event, string, error)
	ListByUser(ctx context.Context, projectID uuid.UUID, userID string, params models.CursorParams, filters EventFilters) ([]*models.Event, string, error)
	CountByName(ctx context.Context, projectID uuid.UUID, query models.AnalyticsQuery) ([]TimeSeriesPoint, error)
	TopEvents(ctx context.Context, projectID uuid.UUID, timeWindow string, limit int) ([]EventCount, error)
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

func (r *postgresRepository) Insert(ctx context.Context, event *models.Event) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO events (id, project_id, user_id, event_name, properties, timestamp, received_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		event.ID, event.ProjectID, event.UserID, event.EventName, event.Properties, event.Timestamp, event.ReceivedAt,
	)
	return err
}

func (r *postgresRepository) InsertBatch(ctx context.Context, events []*models.Event) error {
	batch := &pgx.Batch{}
	for _, event := range events {
		batch.Queue(
			`INSERT INTO events (id, project_id, user_id, event_name, properties, timestamp, received_at)
			 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
			event.ID, event.ProjectID, event.UserID, event.EventName, event.Properties, event.Timestamp, event.ReceivedAt,
		)
	}
	br := r.pool.SendBatch(ctx, batch)
	defer br.Close()
	for range events {
		if _, err := br.Exec(); err != nil {
			return err
		}
	}
	return nil
}

func (r *postgresRepository) GetByID(ctx context.Context, projectID, eventID uuid.UUID) (*models.Event, error) {
	event := &models.Event{}
	err := r.pool.QueryRow(ctx,
		`SELECT id, project_id, user_id, event_name, properties, timestamp, received_at
		 FROM events WHERE id = $1 AND project_id = $2`,
		eventID, projectID,
	).Scan(&event.ID, &event.ProjectID, &event.UserID, &event.EventName, &event.Properties, &event.Timestamp, &event.ReceivedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return event, err
}

func (r *postgresRepository) List(ctx context.Context, projectID uuid.UUID, params models.CursorParams, filters EventFilters) ([]*models.Event, string, error) {
	args := []interface{}{projectID}
	conditions := []string{"project_id = $1"}
	argIdx := 2

	if filters.EventName != "" {
		conditions = append(conditions, fmt.Sprintf("event_name = $%d", argIdx))
		args = append(args, filters.EventName)
		argIdx++
	}
	if filters.UserID != "" {
		conditions = append(conditions, fmt.Sprintf("user_id = $%d", argIdx))
		args = append(args, filters.UserID)
		argIdx++
	}
	if filters.Start != nil {
		conditions = append(conditions, fmt.Sprintf("timestamp >= $%d", argIdx))
		args = append(args, *filters.Start)
		argIdx++
	}
	if filters.End != nil {
		conditions = append(conditions, fmt.Sprintf("timestamp <= $%d", argIdx))
		args = append(args, *filters.End)
		argIdx++
	}

	if params.Cursor != "" {
		ts, id, err := decodeCursor(params.Cursor)
		if err == nil {
			conditions = append(conditions, fmt.Sprintf("(timestamp, id) < ($%d, $%d)", argIdx, argIdx+1))
			args = append(args, ts, id)
			argIdx += 2
		}
	}

	limit := params.Limit
	if limit <= 0 || limit > 100 {
		limit = 50
	}

	query := fmt.Sprintf(
		`SELECT id, project_id, user_id, event_name, properties, timestamp, received_at
		 FROM events WHERE %s ORDER BY timestamp DESC, id DESC LIMIT $%d`,
		strings.Join(conditions, " AND "), argIdx,
	)
	args = append(args, limit+1)

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()

	var events []*models.Event
	for rows.Next() {
		e := &models.Event{}
		if err := rows.Scan(&e.ID, &e.ProjectID, &e.UserID, &e.EventName, &e.Properties, &e.Timestamp, &e.ReceivedAt); err != nil {
			return nil, "", err
		}
		events = append(events, e)
	}

	var nextCursor string
	if len(events) > limit {
		events = events[:limit]
		last := events[limit-1]
		nextCursor = encodeCursor(last.Timestamp, last.ID)
	}

	return events, nextCursor, nil
}

func (r *postgresRepository) ListByUser(ctx context.Context, projectID uuid.UUID, userID string, params models.CursorParams, filters EventFilters) ([]*models.Event, string, error) {
	filters.UserID = userID
	return r.List(ctx, projectID, params, filters)
}

func (r *postgresRepository) CountByName(ctx context.Context, projectID uuid.UUID, query models.AnalyticsQuery) ([]TimeSeriesPoint, error) {
	interval := mapInterval(query.Interval)
	timeFilter := mapTimeWindow(query.TimeWindow)

	args := []interface{}{projectID}
	conditions := "project_id = $1"
	argIdx := 2

	if timeFilter != "" {
		conditions += fmt.Sprintf(" AND timestamp >= NOW() - INTERVAL '%s'", timeFilter)
	}
	if query.EventName != "" {
		conditions += fmt.Sprintf(" AND event_name = $%d", argIdx)
		args = append(args, query.EventName)
	}

	q := fmt.Sprintf(
		`SELECT time_bucket('%s', timestamp) AS bucket, COUNT(*) AS count
		 FROM events WHERE %s
		 GROUP BY bucket ORDER BY bucket ASC`,
		interval, conditions,
	)

	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var points []TimeSeriesPoint
	for rows.Next() {
		var p TimeSeriesPoint
		if err := rows.Scan(&p.Timestamp, &p.Count); err != nil {
			return nil, err
		}
		points = append(points, p)
	}
	return points, nil
}

func (r *postgresRepository) TopEvents(ctx context.Context, projectID uuid.UUID, timeWindow string, limit int) ([]EventCount, error) {
	timeFilter := mapTimeWindow(timeWindow)
	conditions := "project_id = $1"
	if timeFilter != "" {
		conditions += fmt.Sprintf(" AND timestamp >= NOW() - INTERVAL '%s'", timeFilter)
	}

	q := fmt.Sprintf(
		`SELECT event_name, COUNT(*) AS count FROM events
		 WHERE %s GROUP BY event_name ORDER BY count DESC LIMIT $2`,
		conditions,
	)

	rows, err := r.pool.Query(ctx, q, projectID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var counts []EventCount
	for rows.Next() {
		var ec EventCount
		if err := rows.Scan(&ec.EventName, &ec.Count); err != nil {
			return nil, err
		}
		counts = append(counts, ec)
	}
	return counts, nil
}

func encodeCursor(ts time.Time, id uuid.UUID) string {
	raw := fmt.Sprintf("%s:%s", ts.Format(time.RFC3339Nano), id.String())
	return base64.StdEncoding.EncodeToString([]byte(raw))
}

func decodeCursor(cursor string) (time.Time, uuid.UUID, error) {
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

func mapInterval(interval string) string {
	switch interval {
	case "hour":
		return "1 hour"
	case "day":
		return "1 day"
	case "week":
		return "1 week"
	case "month":
		return "1 month"
	default:
		return "1 day"
	}
}

func mapTimeWindow(tw string) string {
	switch tw {
	case "1h":
		return "1 hour"
	case "24h":
		return "24 hours"
	case "7d":
		return "7 days"
	case "30d":
		return "30 days"
	case "90d":
		return "90 days"
	case "all":
		return ""
	default:
		return "7 days"
	}
}
