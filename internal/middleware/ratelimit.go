package middleware

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/opentap/opentap/pkg/cache"
	"github.com/opentap/opentap/pkg/response"
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

func (rl *RateLimiter) Limit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Get project_id from context (set by auth middleware)
		projectID, ok := r.Context().Value("project_id").(uuid.UUID)
		if !ok {
			// No project_id = no rate limiting (e.g., health check)
			next.ServeHTTP(w, r)
			return
		}

		windowKey := time.Now().Truncate(rl.window).Unix()
		key := fmt.Sprintf("ratelimit:%s:%d", projectID.String(), windowKey)

		count, err := rl.redis.Incr(r.Context(), key)
		if err != nil {
			// If Redis is down, allow the request
			next.ServeHTTP(w, r)
			return
		}

		// Set TTL on first request in window
		if count == 1 {
			rl.redis.Expire(r.Context(), key, rl.window)
		}

		// Set rate limit headers
		w.Header().Set("X-RateLimit-Limit", strconv.Itoa(rl.limit))
		w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(max(0, rl.limit-int(count))))

		if int(count) > rl.limit {
			retryAfter := rl.window.Seconds()
			w.Header().Set("Retry-After", strconv.Itoa(int(retryAfter)))
			response.Error(w, http.StatusTooManyRequests, "rate_limited", "Rate limit exceeded")
			return
		}

		next.ServeHTTP(w, r)
	})
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
