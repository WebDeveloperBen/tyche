package server_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/webdeveloperben/tyche/server"
)

func TestCookies(t *testing.T) {
	t.Run("SetCookie sets a cookie", func(t *testing.T) {
		r := server.NewAPI(server.NewServeMuxAdapter())
		r.GET("/set", func(w http.ResponseWriter, r *http.Request) error {
			server.SetCookie(w, server.CookieConfig{
				Name:     "session",
				Value:    "abc123",
				Path:     "/",
				MaxAge:   3600,
				HTTPOnly: true,
				Secure:   true,
				SameSite: http.SameSiteStrictMode,
			})
			return nil
		})

		req := httptest.NewRequest(http.MethodGet, "/set", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		cookies := w.Result().Cookies()
		if len(cookies) != 1 {
			t.Fatalf("expected 1 cookie, got %d", len(cookies))
		}

		if cookies[0].Name != "session" {
			t.Errorf("expected cookie name 'session', got %s", cookies[0].Name)
		}
		if cookies[0].Value != "abc123" {
			t.Errorf("expected cookie value 'abc123', got %s", cookies[0].Value)
		}
	})

	t.Run("SetCookieDefault sets a secure cookie", func(t *testing.T) {
		r := server.NewAPI(server.NewServeMuxAdapter())
		r.GET("/set", func(w http.ResponseWriter, r *http.Request) error {
			server.SetCookieDefault(w, "theme", "dark")
			return nil
		})

		req := httptest.NewRequest(http.MethodGet, "/set", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		cookie := w.Result().Cookies()[0]
		if cookie.Name != "theme" {
			t.Errorf("expected 'theme', got %s", cookie.Name)
		}
		if cookie.Value != "dark" {
			t.Errorf("expected 'dark', got %s", cookie.Value)
		}
		if !cookie.HttpOnly {
			t.Error("expected HttpOnly to be true")
		}
		if !cookie.Secure {
			t.Error("expected Secure to be true")
		}
	})

	t.Run("GetCookie reads a cookie", func(t *testing.T) {
		r := server.NewAPI(server.NewServeMuxAdapter())
		var gotValue string
		r.GET("/get", func(w http.ResponseWriter, r *http.Request) error {
			val, err := server.GetCookie(r, "session")
			if err != nil {
				return err
			}
			gotValue = val
			return nil
		})

		req := httptest.NewRequest(http.MethodGet, "/get", nil)
		req.AddCookie(&http.Cookie{Name: "session", Value: "user123"})
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if gotValue != "user123" {
			t.Errorf("expected 'user123', got %s", gotValue)
		}
	})

	t.Run("GetCookie returns error for missing cookie", func(t *testing.T) {
		r := server.NewAPI(server.NewServeMuxAdapter())
		r.GET("/get", func(w http.ResponseWriter, r *http.Request) error {
			_, err := server.GetCookie(r, "nonexistent")
			if err != http.ErrNoCookie {
				t.Errorf("expected http.ErrNoCookie, got %v", err)
			}
			return nil
		})

		req := httptest.NewRequest(http.MethodGet, "/get", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
	})

	t.Run("GetCookieOr returns default for missing cookie", func(t *testing.T) {
		r := server.NewAPI(server.NewServeMuxAdapter())
		var gotValue string
		r.GET("/get", func(w http.ResponseWriter, r *http.Request) error {
			gotValue = server.GetCookieOr(r, "theme", "light")
			return nil
		})

		req := httptest.NewRequest(http.MethodGet, "/get", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if gotValue != "light" {
			t.Errorf("expected 'light', got %s", gotValue)
		}
	})

	t.Run("DeleteCookie removes a cookie", func(t *testing.T) {
		r := server.NewAPI(server.NewServeMuxAdapter())
		r.GET("/delete", func(w http.ResponseWriter, r *http.Request) error {
			server.DeleteCookie(w, "session")
			return nil
		})

		req := httptest.NewRequest(http.MethodGet, "/delete", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		cookie := w.Result().Cookies()[0]
		if cookie.Name != "session" {
			t.Errorf("expected 'session', got %s", cookie.Name)
		}
		if cookie.MaxAge != -1 {
			t.Errorf("expected MaxAge=-1, got %d", cookie.MaxAge)
		}
	})
}
