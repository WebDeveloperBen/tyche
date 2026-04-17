package plugins

import (
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/webdeveloperben/tyche/server"
)

type RateLimitConfig struct {
	RequestsPerSecond int
	Burst             int
}

type RateLimitMiddleware interface {
	Middleware() server.Middleware
	Register(r *server.Router) func()
	Stop()
}

func RateLimit(cfg ...RateLimitConfig) *rateLimitMiddleware {
	c := RateLimitConfig{RequestsPerSecond: 100, Burst: 200}
	if len(cfg) > 0 {
		c = cfg[0]
	}
	return newRateLimitMiddleware(c.RequestsPerSecond, c.Burst)
}

func RateLimitWithDefaults() *rateLimitMiddleware {
	return newRateLimitMiddleware(100, 200)
}

func newRateLimitMiddleware(requestsPerSecond, burst int) *rateLimitMiddleware {
	if burst < 0 {
		burst = 0
	}
	m := &rateLimitMiddleware{
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

type rateLimitMiddleware struct {
	requestsPerSecond int
	burst             int
	maxTokens         int64
	tokens            int64
	closed            chan struct{}
	stopped           atomic.Bool
	mu                sync.Mutex
}

func (m *rateLimitMiddleware) refill() {
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
				if atomic.LoadInt64(&m.tokens) > max {
					atomic.StoreInt64(&m.tokens, max)
				}
			}
		case <-m.closed:
			return
		}
	}
}

func (m *rateLimitMiddleware) Stop() {
	if m.stopped.Swap(true) {
		return
	}
	close(m.closed)
}

func (m *rateLimitMiddleware) Register(r *server.Router) func() {
	r.Use(m.Middleware())
	return m.Stop
}

func (m *rateLimitMiddleware) Middleware() server.Middleware {
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
