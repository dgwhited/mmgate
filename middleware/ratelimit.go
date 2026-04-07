package middleware

import (
	"log/slog"
	"net/http"
	"sync"

	"github.com/dgwhited/mmgate/auth"
	"golang.org/x/time/rate"
)

// RateLimiter enforces per-client request rate limits using token buckets.
type RateLimiter struct {
	mu       sync.Mutex
	limiters map[string]*rate.Limiter
}

func NewRateLimiter() *RateLimiter {
	return &RateLimiter{
		limiters: make(map[string]*rate.Limiter),
	}
}

func (rl *RateLimiter) getLimiter(clientID string, ratePerMin int) *rate.Limiter {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	if l, ok := rl.limiters[clientID]; ok {
		return l
	}

	// Convert requests/minute to requests/second, burst allows short spikes
	rps := rate.Limit(float64(ratePerMin) / 60.0)
	burst := ratePerMin / 10
	if burst < 1 {
		burst = 1
	}
	l := rate.NewLimiter(rps, burst)
	rl.limiters[clientID] = l
	return l
}

func (rl *RateLimiter) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		client := auth.ClientFromContext(r.Context())
		if client != nil && client.RateLimit > 0 {
			limiter := rl.getLimiter(client.ID, client.RateLimit)
			if !limiter.Allow() {
				slog.Warn("rate limit exceeded", "client", client.ID)
				http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}
