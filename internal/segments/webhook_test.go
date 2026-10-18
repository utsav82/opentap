package segments

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/opentap/opentap/internal/models"
)

func TestNotifierBatchesAndSigns(t *testing.T) {
	var mu sync.Mutex
	var payloads []WebhookPayload
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mac := hmac.New(sha256.New, []byte("secret"))
		mac.Write(body)
		if r.Header.Get("X-OpenTap-Signature") != "sha256="+hex.EncodeToString(mac.Sum(nil)) {
			t.Error("bad signature")
		}
		var p WebhookPayload
		json.Unmarshal(body, &p)
		mu.Lock()
		payloads = append(payloads, p)
		mu.Unlock()
	}))
	defer server.Close()

	added := make([]string, 2500)
	for i := range added {
		added[i] = fmt.Sprint("u", i)
	}
	segment := &models.Segment{ID: uuid.New(), Name: "Pro", WebhookURL: server.URL}
	change := &Change{UserCount: 2500, Added: added, Removed: []string{"gone"}}

	if err := NewNotifier("secret").Notify(context.Background(), segment, change); err != nil {
		t.Fatal(err)
	}
	if len(payloads) != 3 {
		t.Fatalf("got %d requests, want 3", len(payloads))
	}
	if len(payloads[0].Added) != 1000 || len(payloads[0].Removed) != 1 || len(payloads[2].Added) != 500 || len(payloads[2].Removed) != 0 {
		t.Fatalf("unexpected batching: %d/%d, %d/%d", len(payloads[0].Added), len(payloads[0].Removed), len(payloads[2].Added), len(payloads[2].Removed))
	}
}

func TestNotifierRetriesAndSkips(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			w.WriteHeader(http.StatusBadGateway)
		}
	}))
	defer server.Close()

	n := NewNotifier("")
	change := &Change{Added: []string{"a"}}
	if err := n.Notify(context.Background(), &models.Segment{WebhookURL: server.URL}, change); err != nil || calls != 2 {
		t.Fatalf("err=%v calls=%d, want success on the second attempt", err, calls)
	}
	if err := n.Notify(context.Background(), &models.Segment{WebhookURL: server.URL}, &Change{}); err != nil || calls != 2 {
		t.Fatalf("expected no request for an unchanged segment, calls=%d", calls)
	}
}
