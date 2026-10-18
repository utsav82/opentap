package schemas

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/opentap/opentap/internal/models"
	"github.com/opentap/opentap/pkg/cache"
	"github.com/redis/go-redis/v9"
)

// Tracker buffers property schemas between flushes. Distinct values are
// counted with a Redis HyperLogLog per property.
type Tracker struct {
	repo  Repository
	redis *redis.Client

	mu      sync.Mutex
	pending map[string]*models.EventSchema
}

func NewTracker(repo Repository, cache *cache.RedisClient) *Tracker {
	return &Tracker{repo: repo, redis: cache.Client, pending: map[string]*models.EventSchema{}}
}

func distinctKey(projectID uuid.UUID, eventName, property string) string {
	return fmt.Sprintf("schema:%s:%s:%s", projectID, eventName, property)
}

func (t *Tracker) Track(ctx context.Context, event *models.Event) error {
	if len(event.Properties) == 0 {
		return nil
	}
	now := time.Now().UTC()
	pipe := t.redis.Pipeline()

	t.mu.Lock()
	for name, value := range event.Properties {
		key := distinctKey(event.ProjectID, event.EventName, name)
		if s, ok := t.pending[key]; ok {
			s.PropertyType = mergeType(s.PropertyType, InferType(value))
			s.LastSeen = now
		} else {
			t.pending[key] = &models.EventSchema{
				ProjectID:    event.ProjectID,
				EventName:    event.EventName,
				PropertyName: name,
				PropertyType: InferType(value),
				FirstSeen:    now,
				LastSeen:     now,
			}
		}
		encoded, _ := json.Marshal(value)
		pipe.PFAdd(ctx, key, encoded)
	}
	t.mu.Unlock()

	_, err := pipe.Exec(ctx)
	return err
}

func (t *Tracker) Flush(ctx context.Context) (int, error) {
	t.mu.Lock()
	pending := t.pending
	t.pending = map[string]*models.EventSchema{}
	t.mu.Unlock()
	if len(pending) == 0 {
		return 0, nil
	}

	pipe := t.redis.Pipeline()
	counts := make(map[string]*redis.IntCmd, len(pending))
	for key := range pending {
		counts[key] = pipe.PFCount(ctx, key)
	}
	if _, err := pipe.Exec(ctx); err != nil {
		return 0, err
	}

	batch := make([]*models.EventSchema, 0, len(pending))
	for key, s := range pending {
		s.DistinctValuesCount = int(counts[key].Val())
		batch = append(batch, s)
	}
	return len(batch), t.repo.UpsertBatch(ctx, batch)
}
