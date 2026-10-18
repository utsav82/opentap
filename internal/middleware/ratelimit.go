package middleware

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/opentap/opentap/internal/auth"
	"github.com/opentap/opentap/pkg/cache"
	"github.com/opentap/opentap/pkg/response"
	"github.com/redis/go-redis/v9"
)

type RateLimiter struct {
	redis  *cache.RedisClient
	limit  int
	window time.Duration
}

func NewRateLimiter(redis *cache.RedisClient, limit int, window time.Duration) *RateLimiter {
	return &RateLimiter{
		redis:  redis,
		limit:  limit,
		window: window,
	}
}

// Limit uses a sliding window approximated from the current and previous fixed windows.
func (rl *RateLimiter) Limit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		now := time.Now()
		current := now.Truncate(rl.window)
		key := func(t time.Time) string {
			return fmt.Sprintf("ratelimit:%s:%d", auth.ProjectID(r.Context()), t.Unix())
		}

		pipe := rl.redis.Client.TxPipeline()
		incr := pipe.Incr(r.Context(), key(current))
		pipe.Expire(r.Context(), key(current), 2*rl.window)
		prev := pipe.Get(r.Context(), key(current.Add(-rl.window)))
		if _, err := pipe.Exec(r.Context()); err != nil && !errors.Is(err, redis.Nil) {
			// If Redis is down, allow the request
			next.ServeHTTP(w, r)
			return
		}

		previous, _ := prev.Int64()
		weight := 1 - float64(now.Sub(current))/float64(rl.window)
		count := int(float64(previous)*weight) + int(incr.Val())

		w.Header().Set("X-RateLimit-Limit", strconv.Itoa(rl.limit))
		w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(max(0, rl.limit-count)))

		if count > rl.limit {
			w.Header().Set("Retry-After", strconv.Itoa(int(rl.window.Seconds())))
			response.Error(w, http.StatusTooManyRequests, "rate_limited", "Rate limit exceeded")
			return
		}

		next.ServeHTTP(w, r)
	})
}
