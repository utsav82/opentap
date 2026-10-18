package auth

import (
	"regexp"
	"testing"
)

func TestGenerateAPIKey(t *testing.T) {
	format := regexp.MustCompile(`^ot_[a-zA-Z0-9]{43}$`)
	seen := map[string]bool{}
	for range 100 {
		key, err := generateAPIKey()
		if err != nil {
			t.Fatal(err)
		}
		if !format.MatchString(key) {
			t.Fatalf("key %q does not match %s", key, format)
		}
		if seen[key] {
			t.Fatalf("duplicate key %q", key)
		}
		seen[key] = true
	}
}
