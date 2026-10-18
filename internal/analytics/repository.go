package analytics

import (
	"context"
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

type FunnelStep struct {
	EventName string `json:"event_name"`
	Users     int64  `json:"users"`
}

type RetentionCohort struct {
	Cohort   time.Time `json:"cohort"`
	Users    int64     `json:"users"`
	Retained []int64   `json:"retained"`
}

type Repository interface {
	EventCountOverTime(ctx context.Context, projectID uuid.UUID, query models.AnalyticsQuery) ([]TimeSeriesPoint, error)
	TopEvents(ctx context.Context, projectID uuid.UUID, timeWindow string, limit int) ([]EventCount, error)
	DAU(ctx context.Context, projectID uuid.UUID, query models.AnalyticsQuery) ([]TimeSeriesPoint, error)
	Funnel(ctx context.Context, projectID uuid.UUID, steps []string, timeWindow string) ([]FunnelStep, error)
	Retention(ctx context.Context, projectID uuid.UUID, query RetentionQuery) ([]*RetentionCohort, error)
}

type postgresRepository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) Repository {
	return &postgresRepository{pool: pool}
}

func (r *postgresRepository) EventCountOverTime(ctx context.Context, projectID uuid.UUID, query models.AnalyticsQuery) ([]TimeSeriesPoint, error) {
	return r.timeSeries(ctx, "COUNT(*)", projectID, query)
}

func (r *postgresRepository) DAU(ctx context.Context, projectID uuid.UUID, query models.AnalyticsQuery) ([]TimeSeriesPoint, error) {
	return r.timeSeries(ctx, "COUNT(DISTINCT user_id)", projectID, query)
}

func (r *postgresRepository) TopEvents(ctx context.Context, projectID uuid.UUID, timeWindow string, limit int) ([]EventCount, error) {
	rows, _ := r.pool.Query(ctx,
		fmt.Sprintf(`SELECT event_name, COUNT(*) AS count FROM events
		 WHERE project_id = $1%s GROUP BY event_name ORDER BY count DESC LIMIT $2`, timeFilter(timeWindow)),
		projectID, limit,
	)
	return pgx.CollectRows(rows, pgx.RowToStructByPos[EventCount])
}

// Funnel counts users who did each step in order, where every step happens at or after the previous one.
func (r *postgresRepository) Funnel(ctx context.Context, projectID uuid.UUID, steps []string, timeWindow string) ([]FunnelStep, error) {
	args := []interface{}{projectID}
	ctes := make([]string, len(steps))
	counts := make([]string, len(steps))
	for i, step := range steps {
		args = append(args, step)
		join := ""
		if i > 0 {
			join = fmt.Sprintf(" JOIN s%d prev ON prev.user_id = e.user_id AND e.timestamp >= prev.t", i-1)
		}
		ctes[i] = fmt.Sprintf(`s%d AS (SELECT e.user_id, MIN(e.timestamp) AS t FROM events e%s
			WHERE e.project_id = $1 AND e.event_name = $%d%s GROUP BY e.user_id)`, i, join, len(args), timeFilter(timeWindow))
		counts[i] = fmt.Sprintf("(SELECT COUNT(*) FROM s%d)", i)
	}

	result := make([]FunnelStep, len(steps))
	dest := make([]interface{}, len(steps))
	for i := range steps {
		result[i].EventName = steps[i]
		dest[i] = &result[i].Users
	}
	err := r.pool.QueryRow(ctx, "WITH "+strings.Join(ctes, ", ")+" SELECT "+strings.Join(counts, ", "), args...).Scan(dest...)
	return result, err
}

type RetentionQuery struct {
	StartEvent  string
	ReturnEvent string
	Interval    string
	Periods     int
}

var retentionIntervals = map[string]int{"day": 86400, "week": 604800}

// Retention groups users by the bucket of their first start event and counts, per later bucket,
// how many of them did the return event.
func (r *postgresRepository) Retention(ctx context.Context, projectID uuid.UUID, query RetentionQuery) ([]*RetentionCohort, error) {
	interval := models.Intervals[query.Interval]
	since := fmt.Sprintf("time_bucket('%s', NOW()) - $4 * INTERVAL '%s'", interval, interval)
	rows, _ := r.pool.Query(ctx, fmt.Sprintf(`
		WITH cohort AS (
			SELECT user_id, time_bucket('%[1]s', MIN(timestamp)) AS cohort FROM events
			WHERE project_id = $1 AND event_name = $2 AND timestamp >= %[2]s
			GROUP BY user_id
		),
		activity AS (
			SELECT DISTINCT user_id, time_bucket('%[1]s', timestamp) AS bucket FROM events
			WHERE project_id = $1 AND event_name = $3 AND timestamp >= %[2]s
		)
		SELECT cohort, -1, COUNT(*) FROM cohort GROUP BY cohort
		UNION ALL
		SELECT c.cohort, (EXTRACT(EPOCH FROM a.bucket - c.cohort) / %[3]d)::int, COUNT(*)
		FROM cohort c JOIN activity a ON a.user_id = c.user_id AND a.bucket >= c.cohort
		GROUP BY 1, 2
		ORDER BY 1, 2`, interval, since, retentionIntervals[query.Interval]),
		projectID, query.StartEvent, query.ReturnEvent, query.Periods,
	)

	cohorts := []*RetentionCohort{}
	var start time.Time
	var period int
	var users int64
	_, err := pgx.ForEachRow(rows, []interface{}{&start, &period, &users}, func() error {
		if len(cohorts) == 0 || !cohorts[len(cohorts)-1].Cohort.Equal(start) {
			cohorts = append(cohorts, &RetentionCohort{Cohort: start, Retained: make([]int64, query.Periods)})
		}
		cohort := cohorts[len(cohorts)-1]
		switch {
		case period < 0:
			cohort.Users = users
		case period < query.Periods:
			cohort.Retained[period] = users
		}
		return nil
	})
	return cohorts, err
}

func (r *postgresRepository) timeSeries(ctx context.Context, aggregate string, projectID uuid.UUID, query models.AnalyticsQuery) ([]TimeSeriesPoint, error) {
	interval, ok := models.Intervals[query.Interval]
	if !ok {
		interval = models.Intervals["day"]
	}

	args := []interface{}{projectID}
	conditions := "project_id = $1" + timeFilter(query.TimeWindow)
	if query.EventName != "" {
		args = append(args, query.EventName)
		conditions += " AND event_name = $2"
	}

	rows, _ := r.pool.Query(ctx,
		fmt.Sprintf(`SELECT time_bucket('%s', timestamp) AS bucket, %s AS count
		 FROM events WHERE %s
		 GROUP BY bucket ORDER BY bucket ASC`, interval, aggregate, conditions),
		args...,
	)
	return pgx.CollectRows(rows, pgx.RowToStructByPos[TimeSeriesPoint])
}

func timeFilter(window string) string {
	if window == "all" {
		return ""
	}
	interval, ok := models.TimeWindows[window]
	if !ok {
		interval = models.TimeWindows["7d"]
	}
	return fmt.Sprintf(" AND timestamp >= NOW() - INTERVAL '%s'", interval)
}
