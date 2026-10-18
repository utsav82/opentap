package segments

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/opentap/opentap/internal/models"
)

var comparisons = map[string]string{"eq": "=", "neq": "!=", "gt": ">", "gte": ">=", "lt": "<", "lte": "<="}

// BuildQuery validates conditions and returns SQL selecting the internal IDs of matching users.
func BuildQuery(projectID uuid.UUID, conditions map[string]interface{}) (string, []interface{}, error) {
	operator, _ := conditions["operator"].(string)
	if operator != "AND" && operator != "OR" {
		return "", nil, errors.New("conditions.operator must be AND or OR")
	}
	rules, _ := conditions["rules"].([]interface{})
	if len(rules) == 0 {
		return "", nil, errors.New("conditions.rules must be a non-empty array")
	}

	q := &query{}
	project := q.arg(projectID)
	clauses := make([]string, len(rules))
	for i, r := range rules {
		rule, _ := r.(map[string]interface{})
		var err error
		switch rule["type"] {
		case "event":
			clauses[i], err = q.eventRule(project, rule)
		case "user_property":
			clauses[i], err = q.propertyFilter("u.properties", rule)
		default:
			err = errors.New("type must be event or user_property")
		}
		if err != nil {
			return "", nil, fmt.Errorf("rule %d: %w", i, err)
		}
	}

	sql := fmt.Sprintf("SELECT u.id FROM users u WHERE u.project_id = %s AND (%s)", project, strings.Join(clauses, " "+operator+" "))
	return sql, q.args, nil
}

type query struct {
	args []interface{}
}

func (q *query) arg(v interface{}) string {
	q.args = append(q.args, v)
	return fmt.Sprintf("$%d", len(q.args))
}

func (q *query) eventRule(project string, rule map[string]interface{}) (string, error) {
	name, _ := rule["event_name"].(string)
	if name == "" {
		return "", errors.New("event_name is required")
	}
	where := []string{"e.project_id = " + project, "e.event_name = " + q.arg(name)}

	if window, _ := rule["time_window"].(string); window != "" && window != "all" {
		interval, ok := models.TimeWindows[window]
		if !ok {
			return "", fmt.Errorf("invalid time_window %q", window)
		}
		where = append(where, fmt.Sprintf("e.timestamp >= NOW() - INTERVAL '%s'", interval))
	}

	filters, _ := rule["property_filters"].([]interface{})
	for _, f := range filters {
		filter, _ := f.(map[string]interface{})
		clause, err := q.propertyFilter("e.properties", filter)
		if err != nil {
			return "", err
		}
		where = append(where, clause)
	}

	op, _ := rule["count_operator"].(string)
	if op == "" {
		op = "gte"
	}
	if _, ok := comparisons[op]; !ok {
		return "", fmt.Errorf("invalid count_operator %q", op)
	}
	count := 1.0
	if rule["count_value"] != nil {
		var err error
		if count, err = toNumber(rule["count_value"]); err != nil || count < 0 || count != math.Trunc(count) {
			return "", errors.New("count_value must be a non-negative integer")
		}
	}

	n := int64(count)
	having := fmt.Sprintf("COUNT(*) %s %s", comparisons[op], q.arg(n))
	matched := fmt.Sprintf("SELECT e.user_id FROM events e WHERE %s GROUP BY e.user_id", strings.Join(where, " AND "))

	// Users without any matching event have a count of 0, which the HAVING clause never sees.
	zeroMatches := map[string]bool{"eq": n == 0, "neq": n != 0, "gt": false, "gte": n == 0, "lt": n > 0, "lte": true}[op]
	if zeroMatches {
		return fmt.Sprintf("u.user_id NOT IN (%s HAVING NOT (%s))", matched, having), nil
	}
	return fmt.Sprintf("u.user_id IN (%s HAVING %s)", matched, having), nil
}

func (q *query) propertyFilter(column string, filter map[string]interface{}) (string, error) {
	prop, _ := filter["property"].(string)
	if prop == "" {
		return "", errors.New("property is required")
	}
	op, _ := filter["operator"].(string)
	value := filter["value"]
	if value == nil && op != "is_set" && op != "is_not_set" {
		return "", fmt.Errorf("property %q: value is required", prop)
	}

	key := q.arg(prop)
	text := fmt.Sprintf("%s->>%s", column, key)
	switch op {
	case "is_set":
		return fmt.Sprintf("%s ? %s", column, key), nil
	case "is_not_set":
		return fmt.Sprintf("NOT %s ? %s", column, key), nil
	case "eq":
		return fmt.Sprintf("%s = %s", text, q.arg(fmt.Sprint(value))), nil
	case "neq":
		return fmt.Sprintf("%s IS DISTINCT FROM %s", text, q.arg(fmt.Sprint(value))), nil
	case "contains":
		return fmt.Sprintf("%s ILIKE '%%' || %s || '%%'", text, q.arg(fmt.Sprint(value))), nil
	case "not_contains":
		return fmt.Sprintf("COALESCE(%s NOT ILIKE '%%' || %s || '%%', TRUE)", text, q.arg(fmt.Sprint(value))), nil
	case "gt", "gte", "lt", "lte":
		n, err := toNumber(value)
		if err != nil {
			return "", fmt.Errorf("property %q: %w", prop, err)
		}
		// CASE guarantees non-numeric values are skipped instead of failing the cast.
		return fmt.Sprintf("CASE WHEN jsonb_typeof(%s->%s) = 'number' THEN (%s)::numeric END %s %s",
			column, key, text, comparisons[op], q.arg(n)), nil
	}
	return "", fmt.Errorf("property %q: invalid operator %q", prop, op)
}

func toNumber(v interface{}) (float64, error) {
	switch n := v.(type) {
	case float64:
		return n, nil
	case string:
		if f, err := strconv.ParseFloat(n, 64); err == nil {
			return f, nil
		}
	}
	return 0, fmt.Errorf("%v is not a number", v)
}
