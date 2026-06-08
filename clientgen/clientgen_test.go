package clientgen_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/webdeveloperben/tyche/clientgen"
)

const sampleSpec = `{
  "openapi": "3.1.0",
  "info": {"title": "Test API", "version": "1.0.0"},
  "paths": {
    "/users/{id}": {
      "get": {
        "operationId": "get-user",
        "summary": "Fetch a user by id",
        "parameters": [
          {"name": "id", "in": "path", "required": true, "schema": {"type": "string"}},
          {"name": "expand", "in": "query", "required": false, "schema": {"type": "string"}},
          {"name": "X-Trace", "in": "header", "required": false, "schema": {"type": "string"}}
        ],
        "responses": {
          "200": {"description": "ok", "content": {"application/json": {"schema": {"type": "object", "properties": {"data": {"type": "object", "properties": {
            "id": {"type": "string"},
            "name": {"type": "string"},
            "age": {"type": "integer", "format": "int32"},
            "status": {"type": "string", "enum": ["active", "inactive"]},
            "createdAt": {"type": "string", "format": "date-time"},
            "tags": {"type": "array", "items": {"type": "string"}},
            "meta": {"type": "object", "additionalProperties": {"type": "integer", "format": "int64"}}
          }, "required": ["id", "name", "status"]}}}}}}
        }
      }
    },
    "/users": {
      "get": {
        "operationId": "list-users",
        "parameters": [{"name": "tag", "in": "query", "required": false, "schema": {"type": "array", "items": {"type": "string"}}}],
        "responses": {"200": {"description": "ok", "content": {"application/json": {"schema": {"type": "object", "properties": {"data": {"type": "array", "items": {"type": "object", "properties": {"id": {"type": "string"}}, "required": ["id"]}}}}}}}}
      },
      "post": {
        "operationId": "create-user",
        "requestBody": {"required": true, "content": {"application/json": {"schema": {"type": "object", "properties": {"name": {"type": "string"}, "tags": {"type": "array", "items": {"type": "string"}}}, "required": ["name"]}}}},
        "responses": {"201": {"description": "created", "content": {"application/json": {"schema": {"type": "object", "properties": {"data": {"type": "object", "properties": {"id": {"type": "string"}}, "required": ["id"]}}}}}}}
      }
    },
    "/users/{id}/disable": {
      "post": {
        "operationId": "disable-user",
        "parameters": [{"name": "id", "in": "path", "required": true, "schema": {"type": "string"}}],
        "responses": {"204": {"description": "no content"}}
      }
    },
    "/events": {
      "get": {
        "operationId": "stream-events",
        "parameters": [{"name": "topic", "in": "query", "required": true, "schema": {"type": "string"}}],
        "responses": {"200": {"description": "stream", "content": {"text/event-stream": {"schema": {"type": "object", "properties": {"message": {"type": "string"}, "seq": {"type": "integer", "format": "int64"}}, "required": ["message"]}}}}}
      }
    }
  },
  "components": {"schemas": {}}
}`

func generateSample(t *testing.T) *clientgen.Result {
	t.Helper()
	doc, err := clientgen.ParseDocument([]byte(sampleSpec))
	if err != nil {
		t.Fatalf("ParseDocument: %v", err)
	}
	res, err := clientgen.Generate(doc, clientgen.Options{Module: "example.com/test/client"})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	return res
}

func fileByName(res *clientgen.Result, name string) (string, bool) {
	for _, f := range res.Files {
		if f.Name == name {
			return string(f.Content), true
		}
	}
	return "", false
}

func TestGenerate_ExpectedFilesAndSymbols(t *testing.T) {
	res := generateSample(t)

	for _, want := range []string{"go.mod", "client.go", "operations.go", "types.go", "stream.go"} {
		if _, ok := fileByName(res, want); !ok {
			t.Errorf("missing generated file %q", want)
		}
	}

	gomod, _ := fileByName(res, "go.mod")
	if !strings.Contains(gomod, "module example.com/test/client") {
		t.Errorf("go.mod missing module line:\n%s", gomod)
	}

	ops, _ := fileByName(res, "operations.go")
	for _, want := range []string{
		"func (c *Client) GetUser(ctx context.Context, in *GetUserInput, opts ...CallOption) (*GetUserOutput, error)",
		"func (c *Client) CreateUser(ctx context.Context, in *CreateUserInput, opts ...CallOption)",
		"func (c *Client) ListUsers(ctx context.Context, in *ListUsersInput, opts ...CallOption)",
		"func (c *Client) DisableUser(ctx context.Context, in *DisableUserInput, opts ...CallOption) error", // 204 -> error only
		"func (c *Client) StreamEvents(ctx context.Context, in *StreamEventsInput, opts ...CallOption) (*Stream[StreamEventsEvent], error)",
		"return doStream[StreamEventsEvent](ctx, c, http.MethodGet,",
		"opts)\n", // opts threaded to the helper call
		`url.PathEscape(fmtParam(in.ID))`,
		`query.Set("expand", fmtParam(*in.Expand))`, // optional query -> pointer deref
		`query.Set("topic", fmtParam(in.Topic))`,    // required stream query param
		`header.Set("X-Trace", fmtParam(*in.XTrace))`,
	} {
		if !strings.Contains(ops, want) {
			t.Errorf("operations.go missing %q", want)
		}
	}

	stream, _ := fileByName(res, "stream.go")
	for _, want := range []string{"type Stream[O any] struct", "func (s *Stream[O]) Next() bool", "func (s *Stream[O]) Close() error"} {
		if !strings.Contains(stream, want) {
			t.Errorf("stream.go missing %q", want)
		}
	}

	// gofmt column-aligns struct fields, so normalize runs of whitespace to a
	// single space before substring matching.
	types := normalizeWS(mustFile(res, "types.go"))
	for _, want := range []string{
		"CreatedAt *time.Time",            // optional date-time -> *time.Time
		"Age *int32",                      // optional int32 -> pointer
		"Meta map[string]int64",           // additionalProperties -> map
		"Tags []string",                   // array
		"type GetUserOutputStatus string", // string enum -> named type
		`GetUserOutputStatusActive GetUserOutputStatus = "active"`,
	} {
		if !strings.Contains(types, want) {
			t.Errorf("types.go missing %q", want)
		}
	}
}

func mustFile(res *clientgen.Result, name string) string {
	s, _ := fileByName(res, name)
	return s
}

func normalizeWS(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// TestGenerate_Compiles writes the generated module to a temp dir and compiles
// it, proving the output is valid, self-contained Go.
func TestGenerate_Compiles(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping compile test in -short mode")
	}
	res := generateSample(t)

	dir := t.TempDir()
	for _, f := range res.Files {
		if err := os.WriteFile(filepath.Join(dir, f.Name), f.Content, 0o644); err != nil {
			t.Fatalf("write %s: %v", f.Name, err)
		}
	}

	cmd := exec.Command("go", "build", "./...")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOFLAGS=-mod=mod", "GOWORK=off")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("generated client failed to compile: %v\n%s", err, out)
	}
}

func TestGenerate_RequiresModule(t *testing.T) {
	doc, _ := clientgen.ParseDocument([]byte(sampleSpec))
	if _, err := clientgen.Generate(doc, clientgen.Options{}); err == nil {
		t.Error("expected error when Module is empty")
	}
}

const runtimeSpec = `{
  "openapi": "3.1.0", "info": {"title": "RT", "version": "1.0.0"},
  "paths": {
    "/ping": {"get": {"operationId": "ping", "responses": {"200": {"description": "ok", "content": {"application/json": {"schema": {"type": "object", "properties": {"data": {"type": "object", "properties": {"pong": {"type": "boolean"}, "count": {"type": "integer", "format": "int64"}}, "required": ["pong", "count"]}}}}}}}}},
    "/boom": {"get": {"operationId": "boom", "responses": {"200": {"description": "ok", "content": {"application/json": {"schema": {"type": "object", "properties": {"data": {"type": "object", "properties": {"x": {"type": "string"}}}}}}}}}}},
    "/stream": {"get": {"operationId": "stream-events", "responses": {"200": {"description": "s", "content": {"text/event-stream": {"schema": {"type": "object", "properties": {"message": {"type": "string"}}, "required": ["message"]}}}}}}}
  },
  "components": {"schemas": {}}
}`

// runtimeHarness is written into the generated module and exercises the runtime
// against a real httptest server: envelope unwrap, per-call header injection,
// response-header access, typed problem+json errors, and SSE streaming.
const runtimeHarness = `package client

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRuntimeBehavior(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/ping", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Custom") != "hi" {
			t.Errorf("per-call header not sent: %q", r.Header.Get("X-Custom"))
		}
		w.Header().Set("X-Trace", "abc")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(` + "`" + `{"data":{"pong":true,"count":7}}` + "`" + `))
	})
	mux.HandleFunc("/boom", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/problem+json")
		w.WriteHeader(400)
		_, _ = w.Write([]byte(` + "`" + `{"type":"about:blank","title":"Bad","status":400,"detail":"nope"}` + "`" + `))
	})
	mux.HandleFunc("/stream", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		f := w.(http.Flusher)
		for _, m := range []string{"a", "b", "c"} {
			_, _ = w.Write([]byte("data: {\"message\":\"" + m + "\"}\n\n"))
			f.Flush()
		}
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := New(srv.URL)

	var trace string
	out, err := c.Ping(context.Background(), &PingInput{},
		WithRequestHeader("X-Custom", "hi"),
		WithResponseCallback(func(resp *http.Response) { trace = resp.Header.Get("X-Trace") }))
	if err != nil {
		t.Fatalf("Ping: %v", err)
	}
	if !out.Pong || out.Count != 7 {
		t.Errorf("envelope unwrap wrong: %+v", out)
	}
	if trace != "abc" {
		t.Errorf("response callback did not see header: %q", trace)
	}

	if _, err := c.Boom(context.Background(), &BoomInput{}); err != nil {
		var apiErr *APIError
		if !errors.As(err, &apiErr) {
			t.Fatalf("expected *APIError, got %T", err)
		}
		if apiErr.StatusCode != 400 || apiErr.Detail != "nope" {
			t.Errorf("bad APIError: %+v", apiErr)
		}
	} else {
		t.Error("expected error from /boom")
	}

	s, err := c.StreamEvents(context.Background(), &StreamEventsInput{})
	if err != nil {
		t.Fatalf("StreamEvents: %v", err)
	}
	defer s.Close()
	var got []string
	for s.Next() {
		got = append(got, s.Event().Message)
	}
	if err := s.Err(); err != nil {
		t.Fatalf("stream error: %v", err)
	}
	if len(got) != 3 || got[0] != "a" || got[2] != "c" {
		t.Errorf("stream events wrong: %v", got)
	}
}
`

// TestGenerate_RuntimeBehavior generates a client, drops a hand-written test
// into the generated module, and runs `go test` there — validating the full
// runtime (envelope, errors, call options, response headers, SSE) end-to-end.
func TestGenerate_RuntimeBehavior(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping runtime behavior test in -short mode")
	}
	doc, err := clientgen.ParseDocument([]byte(runtimeSpec))
	if err != nil {
		t.Fatalf("ParseDocument: %v", err)
	}
	res, err := clientgen.Generate(doc, clientgen.Options{Module: "example.com/rt/client"})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	dir := t.TempDir()
	for _, f := range res.Files {
		if err := os.WriteFile(filepath.Join(dir, f.Name), f.Content, 0o644); err != nil {
			t.Fatalf("write %s: %v", f.Name, err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "runtime_test.go"), []byte(runtimeHarness), 0o644); err != nil {
		t.Fatalf("write harness: %v", err)
	}

	cmd := exec.Command("go", "test", "./...")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOFLAGS=-mod=mod", "GOWORK=off")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("generated client runtime test failed: %v\n%s", err, out)
	}
}
