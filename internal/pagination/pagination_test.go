package pagination

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestCursorRoundTrip(t *testing.T) {
	ts := time.Date(2026, 1, 15, 10, 30, 0, 123456789, time.UTC)
	id := uuid.New()

	gotTS, gotID, ok := Decode(Encode(ts, id))
	if !ok || !gotTS.Equal(ts) || gotID != id {
		t.Fatalf("got %v %v %v, want %v %v", gotTS, gotID, ok, ts, id)
	}
	if _, _, ok := Decode("garbage"); ok {
		t.Fatal("expected invalid cursor")
	}
}

func TestPage(t *testing.T) {
	now := time.Now()
	key := func(i int) (time.Time, uuid.UUID) { return now.Add(time.Duration(i)), uuid.Nil }

	items, next := Page([]int{1, 2, 3}, 2, key)
	if len(items) != 2 || next == "" {
		t.Fatalf("got %v %q, want two items and a cursor", items, next)
	}
	if ts, _, _ := Decode(next); !ts.Equal(now.Add(2)) {
		t.Fatalf("cursor points at %v, want last returned item", ts)
	}
	if _, next := Page([]int{1, 2}, 2, key); next != "" {
		t.Fatal("expected no cursor on the last page")
	}
}

func TestParseClampsLimit(t *testing.T) {
	for query, want := range map[string]int{"": 50, "limit=10": 10, "limit=0": 50, "limit=500": 50, "limit=x": 50} {
		if got := Parse(httptest.NewRequest("GET", "/?"+query, nil)).Limit; got != want {
			t.Errorf("%q: got %d, want %d", query, got, want)
		}
	}
}
