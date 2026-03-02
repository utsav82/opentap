package segments

import (
	"fmt"
	"strings"

	"github.com/google/uuid"
)

// Evaluator builds SQL queries from segment conditions.
type Evaluator struct{}

func NewEvaluator() *Evaluator {
	return &Evaluator{}
}

// BuildQuery generates a SQL query that returns user internal UUIDs matching the segment conditions.
func (e *Evaluator) BuildQuery(projectID uuid.UUID, conditions map[string]interface{}) (string, []interface{}, error) {
	operator, _ := conditions["operator"].(string)
	if operator == "" {
		operator = "AND"
	}

	rules, ok := conditions["rules"].([]interface{})
	if !ok || len(rules) == 0 {
		return "", nil, fmt.Errorf("conditions must have rules array")
	}

	var clauses []string
	var args []interface{}
	argIdx := 2 // $1 is project_id

	for _, rule := range rules {
		ruleMap, ok := rule.(map[string]interface{})
		if !ok {
			continue
		}

		ruleType, _ := ruleMap["type"].(string)
		switch ruleType {
		case "event":
			clause, newArgs, newIdx, err := e.buildEventRule(ruleMap, argIdx)
			if err != nil {
				return "", nil, err
			}
			clauses = append(clauses, clause)
			args = append(args, newArgs...)
			argIdx = newIdx

		case "user_property":
			clause, newArgs, newIdx, err := e.buildUserPropertyRule(ruleMap, argIdx)
			if err != nil {
				return "", nil, err
			}
			clauses = append(clauses, clause)
			args = append(args, newArgs...)
			argIdx = newIdx

		default:
			return "", nil, fmt.Errorf("unknown rule type: %s", ruleType)
		}
	}

	if len(clauses) == 0 {
		return "", nil, fmt.Errorf("no valid rules found")
	}

	sqlOperator := " AND "
	if operator == "OR" {
		sqlOperator = " OR "
	}

	// Build the final query that returns user UUIDs
	query := fmt.Sprintf(
		`SELECT u.id FROM users u WHERE u.project_id = $1 AND (%s)`,
		strings.Join(clauses, sqlOperator),
	)

	allArgs := []interface{}{projectID}
	allArgs = append(allArgs, args...)

	return query, allArgs, nil
}

func (e *Evaluator) buildEventRule(rule map[string]interface{}, argIdx int) (string, []interface{}, int, error) {
	eventName, _ := rule["event_name"].(string)
	if eventName == "" {
		return "", nil, argIdx, fmt.Errorf("event rule requires event_name")
	}

	timeWindow, _ := rule["time_window"].(string)
	countOperator, _ := rule["count_operator"].(string)
	countValue := getFloat64(rule, "count_value")

	var args []interface{}

	// Build subquery for counting events
	subConditions := []string{fmt.Sprintf("e.event_name = $%d", argIdx)}
	args = append(args, eventName)
	argIdx++

	subConditions = append(subConditions, "e.project_id = u.project_id")
	subConditions = append(subConditions, "e.user_id = u.user_id")

	// Time window filter
	if timeWindow != "" && timeWindow != "all" {
		interval := mapTimeWindowToInterval(timeWindow)
		if interval != "" {
			subConditions = append(subConditions, fmt.Sprintf("e.timestamp >= NOW() - INTERVAL '%s'", interval))
		}
	}

	// Property filters
	if propFilters, ok := rule["property_filters"].([]interface{}); ok {
		for _, pf := range propFilters {
			pfMap, ok := pf.(map[string]interface{})
			if !ok {
				continue
			}
			prop, _ := pfMap["property"].(string)
			op, _ := pfMap["operator"].(string)
			val := pfMap["value"]

			filterClause, filterArgs, newIdx := buildPropertyFilter(prop, op, val, argIdx)
			if filterClause != "" {
				subConditions = append(subConditions, filterClause)
				args = append(args, filterArgs...)
				argIdx = newIdx
			}
		}
	}

	// Build count comparison
	sqlOp := mapComparisonOperator(countOperator)
	subquery := fmt.Sprintf(
		"(SELECT COUNT(*) FROM events e WHERE %s) %s $%d",
		strings.Join(subConditions, " AND "),
		sqlOp,
		argIdx,
	)
	args = append(args, int64(countValue))
	argIdx++

	return subquery, args, argIdx, nil
}

func (e *Evaluator) buildUserPropertyRule(rule map[string]interface{}, argIdx int) (string, []interface{}, int, error) {
	prop, _ := rule["property"].(string)
	if prop == "" {
		return "", nil, argIdx, fmt.Errorf("user_property rule requires property")
	}

	op, _ := rule["operator"].(string)
	val := rule["value"]

	var clause string
	var args []interface{}

	switch op {
	case "is_set":
		clause = fmt.Sprintf("u.properties ? $%d", argIdx)
		args = append(args, prop)
		argIdx++
	case "is_not_set":
		clause = fmt.Sprintf("NOT (u.properties ? $%d)", argIdx)
		args = append(args, prop)
		argIdx++
	case "eq":
		clause = fmt.Sprintf("u.properties->>$%d = $%d", argIdx, argIdx+1)
		args = append(args, prop, fmt.Sprintf("%v", val))
		argIdx += 2
	case "neq":
		clause = fmt.Sprintf("(u.properties->>$%d IS NULL OR u.properties->>$%d != $%d)", argIdx, argIdx, argIdx+1)
		args = append(args, prop, fmt.Sprintf("%v", val))
		argIdx += 2
	case "gt":
		clause = fmt.Sprintf("(u.properties->>$%d)::numeric > $%d", argIdx, argIdx+1)
		args = append(args, prop, val)
		argIdx += 2
	case "gte":
		clause = fmt.Sprintf("(u.properties->>$%d)::numeric >= $%d", argIdx, argIdx+1)
		args = append(args, prop, val)
		argIdx += 2
	case "lt":
		clause = fmt.Sprintf("(u.properties->>$%d)::numeric < $%d", argIdx, argIdx+1)
		args = append(args, prop, val)
		argIdx += 2
	case "lte":
		clause = fmt.Sprintf("(u.properties->>$%d)::numeric <= $%d", argIdx, argIdx+1)
		args = append(args, prop, val)
		argIdx += 2
	case "contains":
		clause = fmt.Sprintf("u.properties->>$%d ILIKE '%%' || $%d || '%%'", argIdx, argIdx+1)
		args = append(args, prop, fmt.Sprintf("%v", val))
		argIdx += 2
	case "not_contains":
		clause = fmt.Sprintf("(u.properties->>$%d IS NULL OR u.properties->>$%d NOT ILIKE '%%' || $%d || '%%')", argIdx, argIdx, argIdx+1)
		args = append(args, prop, fmt.Sprintf("%v", val))
		argIdx += 2
	default:
		return "", nil, argIdx, fmt.Errorf("unknown operator: %s", op)
	}

	return clause, args, argIdx, nil
}

func buildPropertyFilter(prop, op string, val interface{}, argIdx int) (string, []interface{}, int) {
	var args []interface{}

	switch op {
	case "eq":
		clause := fmt.Sprintf("e.properties->>$%d = $%d", argIdx, argIdx+1)
		args = append(args, prop, fmt.Sprintf("%v", val))
		return clause, args, argIdx + 2
	case "neq":
		clause := fmt.Sprintf("e.properties->>$%d != $%d", argIdx, argIdx+1)
		args = append(args, prop, fmt.Sprintf("%v", val))
		return clause, args, argIdx + 2
	case "gt":
		clause := fmt.Sprintf("(e.properties->>$%d)::numeric > $%d", argIdx, argIdx+1)
		args = append(args, prop, val)
		return clause, args, argIdx + 2
	case "gte":
		clause := fmt.Sprintf("(e.properties->>$%d)::numeric >= $%d", argIdx, argIdx+1)
		args = append(args, prop, val)
		return clause, args, argIdx + 2
	case "lt":
		clause := fmt.Sprintf("(e.properties->>$%d)::numeric < $%d", argIdx, argIdx+1)
		args = append(args, prop, val)
		return clause, args, argIdx + 2
	case "lte":
		clause := fmt.Sprintf("(e.properties->>$%d)::numeric <= $%d", argIdx, argIdx+1)
		args = append(args, prop, val)
		return clause, args, argIdx + 2
	case "contains":
		clause := fmt.Sprintf("e.properties->>$%d ILIKE '%%' || $%d || '%%'", argIdx, argIdx+1)
		args = append(args, prop, fmt.Sprintf("%v", val))
		return clause, args, argIdx + 2
	case "not_contains":
		clause := fmt.Sprintf("e.properties->>$%d NOT ILIKE '%%' || $%d || '%%'", argIdx, argIdx+1)
		args = append(args, prop, fmt.Sprintf("%v", val))
		return clause, args, argIdx + 2
	case "is_set":
		clause := fmt.Sprintf("e.properties ? $%d", argIdx)
		args = append(args, prop)
		return clause, args, argIdx + 1
	case "is_not_set":
		clause := fmt.Sprintf("NOT (e.properties ? $%d)", argIdx)
		args = append(args, prop)
		return clause, args, argIdx + 1
	}

	return "", nil, argIdx
}

func mapComparisonOperator(op string) string {
	switch op {
	case "eq":
		return "="
	case "neq":
		return "!="
	case "gt":
		return ">"
	case "gte":
		return ">="
	case "lt":
		return "<"
	case "lte":
		return "<="
	default:
		return ">="
	}
}

func mapTimeWindowToInterval(tw string) string {
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
	default:
		return ""
	}
}

func getFloat64(m map[string]interface{}, key string) float64 {
	if v, ok := m[key]; ok {
		switch val := v.(type) {
		case float64:
			return val
		case int:
			return float64(val)
		case int64:
			return float64(val)
		}
	}
	return 0
}
