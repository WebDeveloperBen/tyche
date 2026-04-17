package plugins_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/webdeveloperben/tyche/server"
	"github.com/webdeveloperben/tyche/server/plugins"
)

func TestTimeout(t *testing.T) {
	t.Run("completes within timeout", func(t *testing.T) {
		r := server.NewRouter()
		r.Use(plugins.Timeout(plugins.TimeoutConfig{Timeout: 100 * time.Millisecond}))

		r.GET("/test", func(w http.ResponseWriter, r *http.Request) error {
			time.Sleep(10 * time.Millisecond)
			w.WriteHeader(http.StatusOK)
			return nil
		})

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", w.Code)
		}
	})

	t.Run("returns 504 on timeout", func(t *testing.T) {
		r := server.NewRouter()
		r.Use(plugins.Timeout(plugins.TimeoutConfig{Timeout: 50 * time.Millisecond}))

		r.GET("/test", func(w http.ResponseWriter, r *http.Request) error {
			time.Sleep(200 * time.Millisecond)
			return nil
		})

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusGatewayTimeout {
			t.Errorf("expected 504, got %d", w.Code)
		}
	})

	t.Run("context is cancelled on timeout", func(t *testing.T) {
		r := server.NewRouter()
		r.Use(plugins.Timeout(plugins.TimeoutConfig{Timeout: 50 * time.Millisecond}))

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		done := make(chan struct{})

		r.GET("/test", func(w http.ResponseWriter, r *http.Request) error {
			select {
			case <-time.After(200 * time.Millisecond):
			case <-r.Context().Done():
				close(done)
				return nil
			}
			return nil
		})

		req := httptest.NewRequest(http.MethodGet, "/test", nil).WithContext(ctx)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		select {
		case <-done:
		case <-time.After(500 * time.Millisecond):
			t.Error("context should have been cancelled within timeout")
		}
	})

	t.Run("respects context deadline from request", func(t *testing.T) {
		r := server.NewRouter()
		r.Use(plugins.Timeout(plugins.TimeoutConfig{Timeout: 5 * time.Second}))

		r.GET("/test", func(w http.ResponseWriter, r *http.Request) error {
			ctx := r.Context()
			deadline, ok := ctx.Deadline()
			if !ok {
				t.Error("context should have a deadline")
				return nil
			}
			if time.Until(deadline) > 4*time.Second {
				t.Error("deadline should be set by middleware")
			}
			return nil
		})

		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		req := httptest.NewRequest(http.MethodGet, "/test", nil).WithContext(ctx)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
	})

	t.Run("uses default timeout", func(t *testing.T) {
		r := server.NewRouter()
		r.Use(plugins.Timeout())

		var ctx context.Context
		r.GET("/test", func(w http.ResponseWriter, r *http.Request) error {
			ctx = r.Context()
			return nil
		})

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		deadline, ok := ctx.Deadline()
		if !ok {
			t.Error("context should have a deadline")
			return
		}

		expectedMin := 25 * time.Second
		expectedMax := 35 * time.Second
		actual := time.Until(deadline)

		if actual < expectedMin || actual > expectedMax {
			t.Errorf("deadline should be around 30s, got %v", actual)
		}
	})

	t.Run("handler can use context for cleanup", func(t *testing.T) {
		r := server.NewRouter()
		r.Use(plugins.Timeout(plugins.TimeoutConfig{Timeout: 50 * time.Millisecond}))

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		handlerDone := make(chan struct{}, 1)

		r.GET("/test", func(w http.ResponseWriter, r *http.Request) error {
			select {
			case <-r.Context().Done():
			case <-time.After(100 * time.Millisecond):
				t.Error("handler should have detected timeout")
			}
			handlerDone <- struct{}{}
			return nil
		})

		req := httptest.NewRequest(http.MethodGet, "/test", nil).WithContext(ctx)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		select {
		case <-handlerDone:
		case <-time.After(500 * time.Millisecond):
			t.Error("handler should have completed")
		}
	})
}

func TestTimeoutResponseRace(t *testing.T) {
	t.Run("handler completing before timeout returns handler status", func(t *testing.T) {
		r := server.NewRouter()
		r.Use(plugins.Timeout(plugins.TimeoutConfig{Timeout: 200 * time.Millisecond}))

		handlerReturned := make(chan struct{})

		r.GET("/test", func(w http.ResponseWriter, r *http.Request) error {
			time.Sleep(50 * time.Millisecond)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("handler completed"))
			close(handlerReturned)
			return nil
		})

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		<-handlerReturned

		if w.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", w.Code)
		}
	})

	t.Run("timeout before handler writes header returns 504", func(t *testing.T) {
		r := server.NewRouter()
		r.Use(plugins.Timeout(plugins.TimeoutConfig{Timeout: 10 * time.Millisecond}))

		handlerReturned := make(chan struct{})
		headerWritten := make(chan struct{})

		r.GET("/test", func(w http.ResponseWriter, r *http.Request) error {
			select {
			case <-time.After(500 * time.Millisecond):
			case <-r.Context().Done():
				close(headerWritten)
				return r.Context().Err()
			}
			close(handlerReturned)
			return nil
		})

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		select {
		case <-headerWritten:
			if w.Code != http.StatusGatewayTimeout {
				t.Errorf("expected 504, got %d", w.Code)
			}
		case <-time.After(1 * time.Second):
			t.Error("test timed out")
		}
	})

	t.Run("request context cancellation returns error", func(t *testing.T) {
		r := server.NewRouter()
		r.Use(plugins.Timeout(plugins.TimeoutConfig{Timeout: 10 * time.Second}))

		ctx, cancel := context.WithCancel(context.Background())

		r.GET("/test", func(w http.ResponseWriter, r *http.Request) error {
			time.Sleep(100 * time.Millisecond)
			return nil
		})

		req := httptest.NewRequest(http.MethodGet, "/test", nil).WithContext(ctx)
		w := httptest.NewRecorder()

		go func() {
			time.Sleep(10 * time.Millisecond)
			cancel()
		}()

		r.ServeHTTP(w, req)

		if w.Code != http.StatusInternalServerError {
			t.Errorf("expected 500, got %d", w.Code)
		}
	})
}
