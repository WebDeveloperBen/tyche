package server_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/webdeveloperben/tyche/server"
)

func TestServer_Basic(t *testing.T) {
	router := server.NewAPI(server.NewServeMuxAdapter())
	router.GET("/test", func(w http.ResponseWriter, r *http.Request) error {
		_, _ = w.Write([]byte("OK"))
		return nil
	})

	ts := httptest.NewServer(router)
	defer ts.Close()

	client := &http.Client{Timeout: 1 * time.Second}
	resp, err := client.Get(ts.URL + "/test")
	if err != nil {
		t.Fatalf("failed to make request: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}
}

func TestServer_DefaultConfig(t *testing.T) {
	cfg := server.DefaultConfig(":8080")

	if cfg.Address != ":8080" {
		t.Errorf("expected address :8080, got %s", cfg.Address)
	}
	if cfg.ShutdownTimeout != 5*time.Second {
		t.Errorf("expected shutdown timeout 5s, got %v", cfg.ShutdownTimeout)
	}
}

func TestServer_ListenAndServeTLS(t *testing.T) {
	router := server.NewAPI(server.NewServeMuxAdapter())
	router.GET("/test", func(w http.ResponseWriter, r *http.Request) error {
		_, _ = w.Write([]byte("OK"))
		return nil
	})

	cfg := server.Config{
		Address:         ":0",
		ReadTimeout:     1 * time.Second,
		WriteTimeout:    1 * time.Second,
		IdleTimeout:     1 * time.Second,
		ShutdownTimeout: 1 * time.Second,
	}

	srv := server.New(cfg, router)

	go func() {
		_ = srv.ListenAndServeTLS("testdata/cert.pem", "testdata/key.pem")
	}()
	time.Sleep(100 * time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
}
