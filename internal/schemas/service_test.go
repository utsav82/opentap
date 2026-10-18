package schemas

import "testing"

func TestMergeType(t *testing.T) {
	tests := []struct{ a, b, want string }{
		{"number", "number", "number"},
		{"number", "null", "number"},
		{"null", "string", "string"},
		{"number", "string", "mixed"},
		{"mixed", "number", "mixed"},
	}
	for _, tt := range tests {
		if got := mergeType(tt.a, tt.b); got != tt.want {
			t.Errorf("mergeType(%q, %q) = %q, want %q", tt.a, tt.b, got, tt.want)
		}
	}
}
