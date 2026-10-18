package segments

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func conditions(t *testing.T, raw string) map[string]interface{} {
	t.Helper()
	var c map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &c); err != nil {
		t.Fatal(err)
	}
	return c
}

func TestBuildQueryRejectsInvalidConditions(t *testing.T) {
	tests := map[string]string{
		"missing operator":       `{"rules":[{"type":"user_property","property":"plan","operator":"is_set"}]}`,
		"empty rules":            `{"operator":"AND","rules":[]}`,
		"unknown rule type":      `{"operator":"AND","rules":[{"type":"cohort"}]}`,
		"rule not an object":     `{"operator":"AND","rules":["x"]}`,
		"missing event name":     `{"operator":"AND","rules":[{"type":"event"}]}`,
		"bad count operator":     `{"operator":"AND","rules":[{"type":"event","event_name":"x","count_operator":"banana"}]}`,
		"negative count":         `{"operator":"AND","rules":[{"type":"event","event_name":"x","count_value":-1}]}`,
		"fractional count":       `{"operator":"AND","rules":[{"type":"event","event_name":"x","count_value":1.5}]}`,
		"bad time window":        `{"operator":"AND","rules":[{"type":"event","event_name":"x","time_window":"2w"}]}`,
		"bad filter operator":    `{"operator":"AND","rules":[{"type":"event","event_name":"x","property_filters":[{"property":"p","operator":"nope","value":1}]}]}`,
		"non-numeric comparison": `{"operator":"AND","rules":[{"type":"user_property","property":"age","operator":"gt","value":"old"}]}`,
		"missing value":          `{"operator":"AND","rules":[{"type":"user_property","property":"plan","operator":"eq"}]}`,
	}
	for name, raw := range tests {
		t.Run(name, func(t *testing.T) {
			if _, _, err := BuildQuery(uuid.New(), conditions(t, raw)); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

func TestBuildQuery(t *testing.T) {
	tests := []struct {
		name     string
		raw      string
		contains []string
		args     int
	}{
		{
			name:     "count defaults to at least one",
			raw:      `{"operator":"AND","rules":[{"type":"event","event_name":"purchase"}]}`,
			contains: []string{"u.user_id IN (", "HAVING COUNT(*) >= $3"},
			args:     3,
		},
		{
			name:     "zero-count rules include users without events",
			raw:      `{"operator":"AND","rules":[{"type":"event","event_name":"purchase","count_operator":"lt","count_value":2}]}`,
			contains: []string{"u.user_id NOT IN (", "HAVING NOT (COUNT(*) < $3)"},
			args:     3,
		},
		{
			name:     "numeric filters skip non-numeric values",
			raw:      `{"operator":"AND","rules":[{"type":"event","event_name":"purchase","time_window":"30d","property_filters":[{"property":"amount","operator":"gt","value":"100"}]}]}`,
			contains: []string{"INTERVAL '30 days'", "CASE WHEN jsonb_typeof(e.properties->$3) = 'number' THEN (e.properties->>$3)::numeric END > $4"},
			args:     5,
		},
		{
			name:     "rules are joined with the operator",
			raw:      `{"operator":"OR","rules":[{"type":"user_property","property":"plan","operator":"eq","value":"pro"},{"type":"user_property","property":"beta","operator":"is_set"}]}`,
			contains: []string{"u.properties->>$2 = $3 OR u.properties ? $4"},
			args:     4,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sql, args, err := BuildQuery(uuid.New(), conditions(t, tt.raw))
			if err != nil {
				t.Fatal(err)
			}
			for _, want := range tt.contains {
				if !strings.Contains(sql, want) {
					t.Errorf("query %q does not contain %q", sql, want)
				}
			}
			if len(args) != tt.args {
				t.Errorf("got %d args, want %d", len(args), tt.args)
			}
		})
	}
}
