package pagination

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/opentap/opentap/internal/models"
)

const (
	defaultLimit = 50
	maxLimit     = 100
)

func Parse(r *http.Request) models.CursorParams {
	limit, err := strconv.Atoi(r.URL.Query().Get("limit"))
	if err != nil || limit <= 0 || limit > maxLimit {
		limit = defaultLimit
	}
	return models.CursorParams{Cursor: r.URL.Query().Get("cursor"), Limit: limit}
}

func Encode(ts time.Time, id uuid.UUID) string {
	raw := fmt.Sprintf("%s:%s", ts.Format(time.RFC3339Nano), id)
	return base64.StdEncoding.EncodeToString([]byte(raw))
}

func Decode(cursor string) (time.Time, uuid.UUID, bool) {
	raw, err := base64.StdEncoding.DecodeString(cursor)
	if err != nil {
		return time.Time{}, uuid.Nil, false
	}
	sep := strings.LastIndex(string(raw), ":")
	if sep < 0 {
		return time.Time{}, uuid.Nil, false
	}
	ts, tsErr := time.Parse(time.RFC3339Nano, string(raw[:sep]))
	id, idErr := uuid.Parse(string(raw[sep+1:]))
	return ts, id, tsErr == nil && idErr == nil
}

// Page trims a result fetched with limit+1 rows and returns the cursor for the next page.
func Page[T any](items []T, limit int, key func(T) (time.Time, uuid.UUID)) ([]T, string) {
	if len(items) <= limit {
		return items, ""
	}
	items = items[:limit]
	return items, Encode(key(items[limit-1]))
}
