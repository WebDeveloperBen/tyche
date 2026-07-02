package plugins

import (
	"net/http"
	"sync/atomic"
	"time"

	"github.com/webdeveloperben/tyche/server"
)

type RateLimitConfig struct {
	RequestsPerSecond int
	Burst             int
}

func RateLimit(cfg ...RateLimitConfig) *RateLimiter {
	c := RateLimitConfig{RequestsPerSecond: 100, Burst: 200}
	if len(cfg) > 0 {
		c = cfg[0]
	}
	return newRateLimiter(c.RequestsPerSecond, c.Burst)
}

func RateLimitWithDefaults() *RateLimiter {
	return newRateLimiter(100, 200)
}

func newRateLimiter(requestsPerSecond, burst int) *RateLimiter {
	if burst < 0 {
		burst = 0
	}
	m := &RateLimiter{
		requestsPerSecond: requestsPerSecond,
		burst:             burst,
		tokens:            int64(burst),
		maxTokens:         int64(burst),
		closed:            make(chan struct{}),
	}
	if requestsPerSecond > 0 && burst > 0 {
		go m.refill()
	}
	return m
}

// RateLimiter is a token-bucket rate limiter. Apply it as middleware with
// api.Use(rl.Middleware()) and call Stop when done to end its refill goroutine.
type RateLimiter struct {
	requestsPerSecond int
	burst             int
	maxTokens         int64
	tokens            int64
	closed            chan struct{}
	stopped           atomic.Bool
}

func (m *RateLimiter) refill() {
	interval := max(time.Second/time.Duration(m.requestsPerSecond), 10*time.Millisecond)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if m.stopped.Load() {
				return
			}
			tokens := atomic.LoadInt64(&m.tokens)
			max := atomic.LoadInt64(&m.maxTokens)
			if tokens < max {
				atomic.AddInt64(&m.tokens, 1)
			}
		case <-m.closed:
			return
		}
	}
}

func (m *RateLimiter) Stop() {
	if m.stopped.Swap(true) {
		return
	}
	close(m.closed)
}

func (m *RateLimiter) Middleware() server.Middleware {
	return func(next server.HandlerFunc) server.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) error {
			for {
				tokens := atomic.LoadInt64(&m.tokens)
				if tokens <= 0 {
					return server.NewHTTPError(429, "Too Many Requests")
				}
				if atomic.CompareAndSwapInt64(&m.tokens, tokens, tokens-1) {
					return next(w, r)
				}
			}
		}
	}
}
