package segments

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/opentap/opentap/internal/models"
)

const (
	webhookBatchSize = 1000
	webhookAttempts  = 3
)

type WebhookPayload struct {
	SegmentID   string    `json:"segment_id"`
	SegmentName string    `json:"segment_name"`
	UserCount   int64     `json:"user_count"`
	ComputedAt  time.Time `json:"computed_at"`
	Added       []string  `json:"added"`
	Removed     []string  `json:"removed"`
}

// Notifier posts membership changes to a segment's webhook, signed with
// X-OpenTap-Signature: sha256=<hex HMAC of the body> when a secret is configured.
type Notifier struct {
	client *http.Client
	secret []byte
}

func NewNotifier(secret string) *Notifier {
	return &Notifier{client: &http.Client{Timeout: 10 * time.Second}, secret: []byte(secret)}
}

func (n *Notifier) Notify(ctx context.Context, segment *models.Segment, change *Change) error {
	if segment.WebhookURL == "" {
		return nil
	}
	now := time.Now().UTC()
	for i := 0; i < len(change.Added) || i < len(change.Removed); i += webhookBatchSize {
		payload := WebhookPayload{
			SegmentID:   segment.ID.String(),
			SegmentName: segment.Name,
			UserCount:   change.UserCount,
			ComputedAt:  now,
			Added:       window(change.Added, i),
			Removed:     window(change.Removed, i),
		}
		if err := n.post(ctx, segment.WebhookURL, payload); err != nil {
			return err
		}
	}
	return nil
}

func (n *Notifier) post(ctx context.Context, url string, payload WebhookPayload) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	for attempt := 1; ; attempt++ {
		err = n.send(ctx, url, body)
		if err == nil || attempt == webhookAttempts {
			return err
		}
		time.Sleep(time.Duration(attempt) * time.Second)
	}
}

func (n *Notifier) send(ctx context.Context, url string, body []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if len(n.secret) > 0 {
		mac := hmac.New(sha256.New, n.secret)
		mac.Write(body)
		req.Header.Set("X-OpenTap-Signature", "sha256="+hex.EncodeToString(mac.Sum(nil)))
	}

	res, err := n.client.Do(req)
	if err != nil {
		return err
	}
	res.Body.Close()
	if res.StatusCode >= 300 {
		return fmt.Errorf("webhook returned %s", res.Status)
	}
	return nil
}

func window(ids []string, start int) []string {
	if start >= len(ids) {
		return []string{}
	}
	return ids[start:min(start+webhookBatchSize, len(ids))]
}
