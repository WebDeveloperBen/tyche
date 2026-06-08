package server_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/webdeveloperben/tyche/server"
)

func TestEventStream_Headers(t *testing.T) {
	rec := httptest.NewRecorder()
	_, err := server.NewEventStream(rec, httptest.NewRequest(http.MethodGet, "/stream", nil))
	if err != nil {
		t.Fatalf("NewEventStream: %v", err)
	}
	if got := rec.Header().Get("Content-Type"); got != "text/event-stream" {
		t.Errorf("Content-Type = %q", got)
	}
	if got := rec.Header().Get("Cache-Control"); got != "no-cache" {
		t.Errorf("Cache-Control = %q", got)
	}
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d", rec.Code)
	}
}

func TestEventStream_SendFraming(t *testing.T) {
	rec := httptest.NewRecorder()
	stream, err := server.NewEventStream(rec, httptest.NewRequest(http.MethodGet, "/stream", nil))
	if err != nil {
		t.Fatalf("NewEventStream: %v", err)
	}

	if err := stream.Send(server.SSEEvent{ID: "1", Event: "greeting", Data: "hello"}); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if err := stream.SendData(map[string]int{"n": 7}); err != nil {
		t.Fatalf("SendData: %v", err)
	}
	if err := stream.Comment("ping"); err != nil {
		t.Fatalf("Comment: %v", err)
	}

	body := rec.Body.String()
	wantContains := []string{
		"id: 1\n",
		"event: greeting\n",
		"data: hello\n",
		`data: {"n":7}`,
		": ping\n",
	}
	for _, want := range wantContains {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %q\n--- body ---\n%s", want, body)
		}
	}
	// First event must be terminated by a blank line.
	if !strings.Contains(body, "data: hello\n\n") {
		t.Errorf("event not terminated by blank line:\n%s", body)
	}
}

func TestEventStream_MultilineData(t *testing.T) {
	rec := httptest.NewRecorder()
	stream, _ := server.NewEventStream(rec, httptest.NewRequest(http.MethodGet, "/s", nil))
	if err := stream.Send(server.SSEEvent{Data: "line1\nline2"}); err != nil {
		t.Fatalf("Send: %v", err)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "data: line1\ndata: line2\n") {
		t.Errorf("multiline data not split into data fields:\n%s", body)
	}
}

type streamInput struct {
	Topic string `query:"topic" required:"true"`
}

type tokenEvent struct {
	Token string `json:"token"`
}

func TestRegisterStream_EndToEnd(t *testing.T) {
	router := server.NewRouter()
	api := router.Group("/v1")

	server.RegisterStream(api, server.Operation{
		OperationID: "stream-tokens",
		Method:      http.MethodGet,
		Path:        "/stream",
	}, func(ctx context.Context, in *streamInput, stream *server.Stream[tokenEvent]) error {
		for _, tok := range []string{"a", "b", "c"} {
			if err := stream.Send(tokenEvent{Token: in.Topic + ":" + tok}); err != nil {
				return err
			}
		}
		return nil
	})

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/stream?topic=x", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{`"token":"x:a"`, `"token":"x:b"`, `"token":"x:c"`} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q in stream:\n%s", want, body)
		}
	}
}

func TestRegisterStream_ValidationErrorBeforeStream(t *testing.T) {
	router := server.NewRouter()
	api := router.Group("/v1")

	server.RegisterStream(api, server.Operation{
		OperationID: "stream-required",
		Method:      http.MethodGet,
		Path:        "/stream",
	}, func(ctx context.Context, in *streamInput, stream *server.Stream[tokenEvent]) error {
		t.Error("handler should not run when validation fails")
		return nil
	})

	rec := httptest.NewRecorder()
	// Missing required ?topic=
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/stream", nil))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "problem+json") {
		t.Errorf("expected problem+json, got %q", ct)
	}
}

func TestRegisterStream_RouteMiddlewareRuns(t *testing.T) {
	router := server.NewRouter()
	api := router.Group("/v1")

	var ran bool
	mw := func(next server.HandlerFunc) server.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) error {
			ran = true
			return next(w, r)
		}
	}

	// Proves RouteOption (WithMiddleware) threads through the typed
	// registration path (RegisterStreamE -> HandleE -> opts).
	server.RegisterStream(api, server.Operation{
		OperationID: "stream-guarded",
		Method:      http.MethodGet,
		Path:        "/guarded",
	}, func(ctx context.Context, in *streamInput, stream *server.Stream[tokenEvent]) error {
		return nil
	}, server.WithMiddleware(mw))

	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/v1/guarded?topic=x", nil))

	if !ran {
		t.Error("route-level middleware did not run on a typed stream route")
	}
}

func TestRegisterStream_DocumentedInOpenAPI(t *testing.T) {
	router := server.NewRouter()
	api := router.Group("/v1")

	server.RegisterStream(api, server.Operation{
		OperationID: "stream-tokens",
		Method:      http.MethodGet,
		Path:        "/stream",
	}, func(ctx context.Context, in *streamInput, stream *server.Stream[tokenEvent]) error { return nil })

	doc := router.OpenAPI()
	item := doc.Paths["/v1/stream"]
	if item == nil || item.GET == nil {
		t.Fatal("operation not registered in OpenAPI")
	}
	resp := item.GET.Responses["200"]
	if resp == nil || resp.Content["text/event-stream"] == nil {
		t.Fatalf("expected text/event-stream response, got %+v", item.GET.Responses)
	}
}
