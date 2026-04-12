package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRouter_NonHTTPErrorReturnsInternalServerError(t *testing.T) {
	router := NewRouter()
	router.GET("/boom", func(w http.ResponseWriter, r *http.Request) error {
		return errors.New("boom")
	})

	req := httptest.NewRequest(http.MethodGet, "/boom", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected status %d, got %d", http.StatusInternalServerError, w.Code)
	}
	if got := w.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("expected JSON content type, got %q", got)
	}
	var payload struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to decode error response: %v", err)
	}
	if payload.Error != http.StatusText(http.StatusInternalServerError) {
		t.Fatalf("expected body %q, got %#v", http.StatusText(http.StatusInternalServerError), payload)
	}
}

func TestRouter_DuplicatePlainRoutePanics(t *testing.T) {
	router := NewRouter()
	router.GET("/users", func(w http.ResponseWriter, r *http.Request) error { return nil })

	defer func() {
		if recover() == nil {
			t.Fatal("expected duplicate route registration to panic")
		}
	}()

	router.GET("/users", func(w http.ResponseWriter, r *http.Request) error { return nil })
}

func TestHandleE_ReturnsErrorForUnsupportedMethod(t *testing.T) {
	router := NewRouter()
	err := router.HandleE("FETCH", "/users", func(w http.ResponseWriter, r *http.Request) error { return nil })
	if err == nil {
		t.Fatal("expected unsupported method error")
	}
}

func TestRouter_DoesNotWriteErrorAfterPartialResponse(t *testing.T) {
	router := NewRouter()
	router.GET("/partial", func(w http.ResponseWriter, r *http.Request) error {
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte("partial"))
		return errors.New("boom")
	})

	req := httptest.NewRequest(http.MethodGet, "/partial", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusAccepted {
		t.Fatalf("expected status %d, got %d", http.StatusAccepted, w.Code)
	}
	if body := w.Body.String(); body != "partial" {
		t.Fatalf("expected body %q, got %q", "partial", body)
	}
}
