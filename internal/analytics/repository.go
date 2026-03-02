package analytics

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
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
	EventCountOverTime(ctx context.Context, projectID uuid.UUID, query models.AnalyticsQuery) ([]TimeSeriesPoint, error)
	TopEvents(ctx context.Context, projectID uuid.UUID, timeWindow string, limit int) ([]EventCount, error)
	DAU(ctx context.Context, projectID uuid.UUID, query models.AnalyticsQuery) ([]TimeSeriesPoint, error)
}

type postgresRepository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) Repository {
	return &postgresRepository{pool: pool}
}

func (r *postgresRepository) EventCountOverTime(ctx context.Context, projectID uuid.UUID, query models.AnalyticsQuery) ([]TimeSeriesPoint, error) {
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

func (r *postgresRepository) DAU(ctx context.Context, projectID uuid.UUID, query models.AnalyticsQuery) ([]TimeSeriesPoint, error) {
	interval := mapInterval(query.Interval)
	timeFilter := mapTimeWindow(query.TimeWindow)

	conditions := "project_id = $1"
	if timeFilter != "" {
		conditions += fmt.Sprintf(" AND timestamp >= NOW() - INTERVAL '%s'", timeFilter)
	}

	q := fmt.Sprintf(
		`SELECT time_bucket('%s', timestamp) AS bucket, COUNT(DISTINCT user_id) AS count
		 FROM events WHERE %s
		 GROUP BY bucket ORDER BY bucket ASC`,
		interval, conditions,
	)

	rows, err := r.pool.Query(ctx, q, projectID)
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
