package plugins_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/webdeveloperben/tyche/server"
	"github.com/webdeveloperben/tyche/server/plugins"
)

func TestRateLimit(t *testing.T) {
	t.Run("allows requests within limit", func(t *testing.T) {
		r := server.NewRouter()
		rl := plugins.RateLimit(plugins.RateLimitConfig{
			RequestsPerSecond: 10,
			Burst:             10,
		})
		r.Use(rl.Middleware())

		r.GET("/test", func(w http.ResponseWriter, r *http.Request) error {
			w.WriteHeader(http.StatusOK)
			return nil
		})

		for i := range 10 {
			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Errorf("request %d: expected 200, got %d", i, w.Code)
			}
		}
	})

	t.Run("returns 429 when limit exceeded", func(t *testing.T) {
		r := server.NewRouter()
		rl := plugins.RateLimit(plugins.RateLimitConfig{
			RequestsPerSecond: 1,
			Burst:             1,
		})
		r.Use(rl.Middleware())

		r.GET("/test", func(w http.ResponseWriter, r *http.Request) error {
			w.WriteHeader(http.StatusOK)
			return nil
		})

		req1 := httptest.NewRequest(http.MethodGet, "/test", nil)
		w1 := httptest.NewRecorder()
		r.ServeHTTP(w1, req1)

		if w1.Code != http.StatusOK {
			t.Errorf("first request: expected 200, got %d", w1.Code)
		}

		req2 := httptest.NewRequest(http.MethodGet, "/test", nil)
		w2 := httptest.NewRecorder()
		r.ServeHTTP(w2, req2)

		if w2.Code != http.StatusTooManyRequests {
			t.Errorf("second request: expected 429, got %d", w2.Code)
		}
	})

	t.Run("token bucket refills over time", func(t *testing.T) {
		r := server.NewRouter()
		rl := plugins.RateLimit(plugins.RateLimitConfig{
			RequestsPerSecond: 100,
			Burst:             1,
		})
		r.Use(rl.Middleware())

		r.GET("/test", func(w http.ResponseWriter, r *http.Request) error {
			w.WriteHeader(http.StatusOK)
			return nil
		})

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("first request: expected 200, got %d", w.Code)
		}

		req = httptest.NewRequest(http.MethodGet, "/test", nil)
		w = httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusTooManyRequests {
			t.Errorf("second request (immediate): expected 429, got %d", w.Code)
		}

		time.Sleep(20 * time.Millisecond)

		req = httptest.NewRequest(http.MethodGet, "/test", nil)
		w = httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("third request (after refill): expected 200, got %d", w.Code)
		}
	})

	t.Run("uses default values", func(t *testing.T) {
		r := server.NewRouter()
		rl := plugins.RateLimit()
		r.Use(rl.Middleware())

		r.GET("/test", func(w http.ResponseWriter, r *http.Request) error {
			w.WriteHeader(http.StatusOK)
			return nil
		})

		for i := range 200 {
			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Errorf("request %d: expected 200, got %d", i, w.Code)
			}
		}
	})

	t.Run("high burst allows burst of requests", func(t *testing.T) {
		r := server.NewRouter()
		rl := plugins.RateLimit(plugins.RateLimitConfig{
			RequestsPerSecond: 1,
			Burst:             100,
		})
		r.Use(rl.Middleware())

		r.GET("/test", func(w http.ResponseWriter, r *http.Request) error {
			w.WriteHeader(http.StatusOK)
			return nil
		})

		for i := range 100 {
			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Errorf("request %d: expected 200, got %d", i, w.Code)
			}
		}
	})

	t.Run("error message is 'Too Many Requests'", func(t *testing.T) {
		r := server.NewRouter()
		rl := plugins.RateLimit(plugins.RateLimitConfig{
			RequestsPerSecond: 1,
			Burst:             0,
		})
		r.Use(rl.Middleware())

		r.GET("/test", func(w http.ResponseWriter, r *http.Request) error {
			w.WriteHeader(http.StatusOK)
			return nil
		})

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusTooManyRequests {
			t.Errorf("expected 429, got %d", w.Code)
		}

		var payload struct {
			Title string `json:"title"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
			t.Fatalf("expected problem+json error response, got %v", err)
		}
		if payload.Title != "Too Many Requests" {
			t.Errorf("expected title 'Too Many Requests', got '%s'", payload.Title)
		}
	})
}

func TestRateLimitConcurrency(t *testing.T) {
	t.Run("concurrent requests are accurately counted", func(t *testing.T) {
		r := server.NewRouter()
		rl := plugins.RateLimit(plugins.RateLimitConfig{
			RequestsPerSecond: 1,
			Burst:             50,
		})
		r.Use(rl.Middleware())

		r.GET("/test", func(w http.ResponseWriter, r *http.Request) error {
			w.WriteHeader(http.StatusOK)
			return nil
		})

		var accepted, rejected int64
		var wg sync.WaitGroup
		const numWorkers = 10
		const requestsPerWorker = 20

		for range numWorkers {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for range requestsPerWorker {
					req := httptest.NewRequest(http.MethodGet, "/test", nil)
					w := httptest.NewRecorder()
					r.ServeHTTP(w, req)
					if w.Code == http.StatusOK {
						atomic.AddInt64(&accepted, 1)
					} else if w.Code == http.StatusTooManyRequests {
						atomic.AddInt64(&rejected, 1)
					}
				}
			}()
		}
		wg.Wait()

		total := atomic.LoadInt64(&accepted) + atomic.LoadInt64(&rejected)
		if total != numWorkers*requestsPerWorker {
			t.Errorf("total requests %d != expected %d", total, numWorkers*requestsPerWorker)
		}
		if atomic.LoadInt64(&accepted) > 50 {
			t.Errorf("accepted %d exceeds burst limit of 50", accepted)
		}
	})

	t.Run("concurrent requests no race conditions", func(t *testing.T) {
		r := server.NewRouter()
		rl := plugins.RateLimit(plugins.RateLimitConfig{
			RequestsPerSecond: 100,
			Burst:             1000,
		})
		r.Use(rl.Middleware())

		r.GET("/test", func(w http.ResponseWriter, r *http.Request) error {
			w.WriteHeader(http.StatusOK)
			return nil
		})

		var wg sync.WaitGroup
		for range 50 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for range 100 {
					req := httptest.NewRequest(http.MethodGet, "/test", nil)
					w := httptest.NewRecorder()
					r.ServeHTTP(w, req)
				}
			}()
		}
		wg.Wait()
	})

	t.Run("stop stops the refill goroutine", func(t *testing.T) {
		r := server.NewRouter()
		rl := plugins.RateLimit(plugins.RateLimitConfig{
			RequestsPerSecond: 1,
			Burst:             1,
		})
		stop := rl.Register(r)

		r.GET("/test", func(w http.ResponseWriter, r *http.Request) error {
			w.WriteHeader(http.StatusOK)
			return nil
		})

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", w.Code)
		}

		req = httptest.NewRequest(http.MethodGet, "/test", nil)
		w = httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusTooManyRequests {
			t.Errorf("expected 429, got %d", w.Code)
		}

		stop()

		time.Sleep(50 * time.Millisecond)

		req = httptest.NewRequest(http.MethodGet, "/test", nil)
		w = httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusTooManyRequests {
			t.Errorf("after stop, refill should not work; expected 429, got %d", w.Code)
		}
	})

	t.Run("separate RateLimit calls have independent token buckets", func(t *testing.T) {
		r := server.NewRouter()
		rl1 := plugins.RateLimit(plugins.RateLimitConfig{RequestsPerSecond: 1, Burst: 100})
		rl2 := plugins.RateLimit(plugins.RateLimitConfig{RequestsPerSecond: 1, Burst: 5})

		r.Use(rl1.Middleware())
		g2 := r.Group("/g2")
		g2.Use(rl2.Middleware())

		r.GET("/a", func(w http.ResponseWriter, r *http.Request) error {
			w.WriteHeader(http.StatusOK)
			return nil
		})
		g2.GET("/b", func(w http.ResponseWriter, r *http.Request) error {
			w.WriteHeader(http.StatusOK)
			return nil
		})

		for i := range 2 {
			req := httptest.NewRequest(http.MethodGet, "/a", nil)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			if w.Code != http.StatusOK {
				t.Errorf("/a request %d: expected 200, got %d", i, w.Code)
			}
		}

		for i := range 5 {
			req := httptest.NewRequest(http.MethodGet, "/g2/b", nil)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			if w.Code != http.StatusOK {
				t.Errorf("/g2/b request %d: expected 200, got %d", i, w.Code)
			}
		}
	})
}
