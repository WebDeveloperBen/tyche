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
          {"name": "since", "in": "query", "required": false, "schema": {"type": "string", "format": "date-time"}},
          {"name": "X-Trace", "in": "header", "required": false, "schema": {"type": "string"}}
        ],
        "responses": {
          "200": {"description": "ok", "content": {"application/json": {"schema": {"type": "object", "properties": {"data": {"type": "object", "properties": {
            "id": {"type": "string"},
            "name": {"type": "string"},
            "age": {"type": "integer", "format": "int32"},
            "status": {"type": "string", "enum": ["active", "inactive"]},
            "priority": {"type": "integer", "enum": [1, 2, 3]},
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
    },
    "/blobs": {
      "post": {
        "operationId": "put-blob",
        "requestBody": {"required": true, "content": {"application/json": {"schema": {"type": "object", "additionalProperties": true}}}},
        "responses": {"204": {"description": "ok"}}
      }
    },
    "/report": {
      "get": {
        "operationId": "download-report",
        "responses": {"200": {"description": "pdf", "content": {"application/pdf": {"schema": {"type": "string", "format": "binary"}}}}}
      }
    },
    "/jobs": {
      "post": {
        "operationId": "start-job",
        "responses": {
          "201": {"description": "accepted"},
          "202": {"description": "started", "content": {"application/json": {"schema": {"type": "object", "properties": {"data": {"type": "object", "properties": {"id": {"type": "string"}}, "required": ["id"]}}}}}}
        }
      }
    },
    "/uploads": {
      "post": {
        "operationId": "upload-avatar",
        "requestBody": {"required": true, "content": {"multipart/form-data": {"schema": {"type": "object", "properties": {
          "title": {"type": "string"},
          "count": {"type": "integer"},
          "tags": {"type": "array", "items": {"type": "string"}},
          "avatar": {"type": "string", "format": "binary"},
          "docs": {"type": "array", "items": {"type": "string", "format": "binary"}}
        }, "required": ["title", "count", "avatar"]}}}},
        "responses": {"200": {"description": "ok", "content": {"application/json": {"schema": {"type": "object", "properties": {"data": {"type": "object", "properties": {"ok": {"type": "boolean"}}, "required": ["ok"]}}}}}}}
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
		"func (c *Client) DisableUser(ctx context.Context, in *DisableUserInput, opts ...CallOption) error",                 // 204 -> error only
		"func (c *Client) DownloadReport(ctx context.Context, in *DownloadReportInput, opts ...CallOption) ([]byte, error)", // non-JSON body -> []byte
		"func (c *Client) StartJob(ctx context.Context, in *StartJobInput, opts ...CallOption) (*StartJobOutput, error)",
		"func (c *Client) StreamEvents(ctx context.Context, in *StreamEventsInput, opts ...CallOption) (*Stream[StreamEventsEvent], error)",
		"func (c *Client) UploadAvatar(ctx context.Context, in *UploadAvatarInput, opts ...CallOption) (*UploadAvatarOutput, error)",
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

	// date-time param + free-form body must yield used time / encoding/json
	// imports in operations.go (regression guard for per-file import derivation).
	nops := normalizeWS(ops)
	for _, want := range []string{
		"Since *time.Time",
		"Body *json.RawMessage",
		"Avatar *File `file:\"avatar\"`",
		"Docs []File `files:\"docs\"`",
		"Title string `form:\"title\"`",
		"body.addField(\"title\", fmtParam(in.Title))",
		"body.addFile(\"avatar\", *in.Avatar)",
		"func (c *Client) PutBlob(ctx context.Context, in *PutBlobInput, opts ...CallOption) error",
		`return doBytes(ctx, c, http.MethodGet, path, nil, nil, nil, "application/pdf", opts)`,
		`return doJSON[StartJobOutput](ctx, c, http.MethodPost, path, nil, nil, nil, opts)`,
	} {
		if !strings.Contains(nops, want) {
			t.Errorf("operations.go missing %q", want)
		}
	}

	stream, _ := fileByName(res, "stream.go")
	for _, want := range []string{"type Stream[O any] struct", "func (s *Stream[O]) Next() bool", "func (s *Stream[O]) Retry() int", "func (s *Stream[O]) Close() error"} {
		if !strings.Contains(stream, want) {
			t.Errorf("stream.go missing %q", want)
		}
	}

	// gofmt column-aligns struct fields, so normalize runs of whitespace to a
	// single space before substring matching.
	types := normalizeWS(mustFile(res, "types.go"))
	for _, want := range []string{
		"CreatedAt *time.Time",                             // optional date-time -> *time.Time
		"Age *int32",                                       // optional int32 -> pointer
		"Meta map[string]int64",                            // additionalProperties -> map
		"Tags []string",                                    // array
		"type GetUserOutputStatus string",                  // string enum -> named type
		"type GetUserOutputPriority int",                   // integer enum -> named int type
		"GetUserOutputPriority1 GetUserOutputPriority = 1", // integer const, unquoted
		`GetUserOutputStatusActive GetUserOutputStatus = "active"`,
	} {
		if !strings.Contains(types, want) {
			t.Errorf("types.go missing %q", want)
		}
	}
}

const compositionSpec = `{
  "openapi":"3.1.0","info":{"title":"E","version":"1.0.0"},
  "paths":{
    "/pets":{"get":{"operationId":"get-pet","responses":{"200":{"description":"ok","content":{"application/json":{"schema":{"type":"object","properties":{"data":{"$ref":"#/components/schemas/Dog"}}}}}}}}},
    "/things":{"get":{"operationId":"get-thing","responses":{"200":{"description":"ok","content":{"application/json":{"schema":{"type":"object","properties":{"data":{"type":"object","properties":{"payload":{"oneOf":[{"type":"string"},{"type":"integer"}]}}}}}}}}}}}
  },
  "components":{"schemas":{
    "Animal":{"type":"object","properties":{"id":{"type":"string"},"name":{"type":"string"}},"required":["id"]},
    "Dog":{"allOf":[{"$ref":"#/components/schemas/Animal"},{"type":"object","properties":{"breed":{"type":"string"},"goodBoy":{"type":"boolean"}},"required":["breed"]}]}
  }}
}`

func TestGenerate_AllOfMergesToStruct(t *testing.T) {
	doc, err := clientgen.ParseDocument([]byte(compositionSpec))
	if err != nil {
		t.Fatalf("ParseDocument: %v", err)
	}
	res, err := clientgen.Generate(doc, clientgen.Options{Module: "example.com/comp/client"})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	types := normalizeWS(mustFile(res, "types.go"))

	// allOf merges the base + extension into one named struct, with required
	// fields flat and optional ones pointers.
	for _, want := range []string{
		"type Dog struct", // keeps the component name
		"ID string",       // required, from the base (merged)
		"Breed string",    // required, from the extension
		"Name *string",    // optional, from the base
		"GoodBoy *bool",   // optional, from the extension
	} {
		if !strings.Contains(types, want) {
			t.Errorf("allOf: types.go missing %q:\n%s", want, mustFile(res, "types.go"))
		}
	}
	if strings.Contains(types, "Data *json.RawMessage") || strings.Contains(types, "Data json.RawMessage") {
		t.Errorf("allOf should merge to a struct, not stay opaque:\n%s", mustFile(res, "types.go"))
	}

	// oneOf is a union — Go can't model it, so it stays opaque.
	if !strings.Contains(types, "Payload json.RawMessage") && !strings.Contains(types, "Payload *json.RawMessage") {
		t.Errorf("oneOf should remain json.RawMessage:\n%s", mustFile(res, "types.go"))
	}
}

const namingStrategySpec = `{
  "openapi":"3.1.0","info":{"title":"Names","version":"1.0.0"},
  "paths":{
    "/alpha":{"get":{"operationId":"alpha","responses":{"200":{"description":"ok","content":{"application/json":{"schema":{"type":"object","properties":{"data":{"type":"object","properties":{"id":{"type":"string"}},"required":["id"]}}}}}}}}},
    "/beta":{"get":{"operationId":"beta","responses":{"200":{"description":"ok","content":{"application/json":{"schema":{"type":"object","properties":{"data":{"type":"object","properties":{"id":{"type":"string"}},"required":["id"]}}}}}}}}}
  },
  "components":{"schemas":{}}
}`

func TestGenerate_DefaultTypeNamingDeduplicatesStructurally(t *testing.T) {
	doc, err := clientgen.ParseDocument([]byte(namingStrategySpec))
	if err != nil {
		t.Fatalf("ParseDocument: %v", err)
	}
	res, err := clientgen.Generate(doc, clientgen.Options{Module: "example.com/names/client"})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	ops := normalizeWS(mustFile(res, "operations.go"))
	types := normalizeWS(mustFile(res, "types.go"))

	if !strings.Contains(ops, "func (c *Client) Alpha(ctx context.Context, in *AlphaInput, opts ...CallOption) (*AlphaOutput, error)") {
		t.Errorf("alpha should return AlphaOutput:\n%s", mustFile(res, "operations.go"))
	}
	if !strings.Contains(ops, "func (c *Client) Beta(ctx context.Context, in *BetaInput, opts ...CallOption) (*AlphaOutput, error)") {
		t.Errorf("default structural naming should reuse AlphaOutput for beta:\n%s", mustFile(res, "operations.go"))
	}
	if strings.Count(types, "type AlphaOutput struct") != 1 {
		t.Errorf("expected one shared AlphaOutput type:\n%s", mustFile(res, "types.go"))
	}
	if strings.Contains(types, "type BetaOutput struct") {
		t.Errorf("default structural naming should not emit BetaOutput:\n%s", mustFile(res, "types.go"))
	}
}

func TestGenerate_OperationScopedTypeNamingKeepsOperationTypesDistinct(t *testing.T) {
	doc, err := clientgen.ParseDocument([]byte(namingStrategySpec))
	if err != nil {
		t.Fatalf("ParseDocument: %v", err)
	}
	res, err := clientgen.Generate(doc, clientgen.Options{
		Module:             "example.com/names/client",
		TypeNamingStrategy: clientgen.TypeNamingOperationScoped,
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	ops := normalizeWS(mustFile(res, "operations.go"))
	types := normalizeWS(mustFile(res, "types.go"))

	for _, want := range []string{
		"func (c *Client) Alpha(ctx context.Context, in *AlphaInput, opts ...CallOption) (*AlphaOutput, error)",
		"func (c *Client) Beta(ctx context.Context, in *BetaInput, opts ...CallOption) (*BetaOutput, error)",
	} {
		if !strings.Contains(ops, want) {
			t.Errorf("operations.go missing %q:\n%s", want, mustFile(res, "operations.go"))
		}
	}
	for _, want := range []string{"type AlphaOutput struct", "type BetaOutput struct"} {
		if !strings.Contains(types, want) {
			t.Errorf("types.go missing %q:\n%s", want, mustFile(res, "types.go"))
		}
	}
}

func TestGenerate_RejectsUnknownTypeNamingStrategy(t *testing.T) {
	doc, err := clientgen.ParseDocument([]byte(namingStrategySpec))
	if err != nil {
		t.Fatalf("ParseDocument: %v", err)
	}
	_, err = clientgen.Generate(doc, clientgen.Options{
		Module:             "example.com/names/client",
		TypeNamingStrategy: clientgen.TypeNamingStrategy(99),
	})
	if err == nil || !strings.Contains(err.Error(), "unknown TypeNamingStrategy") {
		t.Fatalf("expected unknown strategy error, got %v", err)
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
  "openapi": "3.1.0",
  "info": {"title": "RT", "version": "1.0.0"},
  "paths": {
    "/ping": {
      "get": {
        "operationId": "ping",
        "responses": {
          "200": {"description": "ok", "content": {"application/json": {"schema": {"type": "object", "properties": {"data": {"type": "object", "properties": {"pong": {"type": "boolean"}, "count": {"type": "integer", "format": "int64"}}, "required": ["pong", "count"]}}}}}}
        }
      }
    },
    "/boom": {
      "get": {
        "operationId": "boom",
        "responses": {
          "200": {"description": "ok", "content": {"application/json": {"schema": {"type": "object", "properties": {"data": {"type": "object", "properties": {"x": {"type": "string"}}}}}}}}
        }
      }
    },
    "/stream": {
      "get": {
        "operationId": "stream-events",
        "responses": {
          "200": {"description": "s", "content": {"text/event-stream": {"schema": {"type": "object", "properties": {"message": {"type": "string"}}, "required": ["message"]}}}}
        }
      }
    },
    "/upload": {
      "post": {
        "operationId": "upload-avatar",
        "requestBody": {"required": true, "content": {"multipart/form-data": {"schema": {"type": "object", "properties": {"title": {"type": "string"}, "count": {"type": "integer"}, "tags": {"type": "array", "items": {"type": "string"}}, "avatar": {"type": "string", "format": "binary"}, "docs": {"type": "array", "items": {"type": "string", "format": "binary"}}}, "required": ["title", "count", "avatar"]}}}},
        "responses": {
          "200": {"description": "ok", "content": {"application/json": {"schema": {"type": "object", "properties": {"data": {"type": "object", "properties": {"ok": {"type": "boolean"}}, "required": ["ok"]}}}}}}
        }
      }
    },
    "/report": {
      "get": {
        "operationId": "download-report",
        "responses": {
          "200": {"description": "pdf", "content": {"application/pdf": {"schema": {"type": "string", "format": "binary"}}}}
        }
      }
    },
    "/custom": {
      "post": {
        "operationId": "custom-codec",
        "requestBody": {"required": true, "content": {"application/json": {"schema": {"type": "object", "properties": {"name": {"type": "string"}}, "required": ["name"]}}}},
        "responses": {
          "200": {"description": "ok", "content": {"application/json": {"schema": {"type": "object", "properties": {"data": {"type": "object", "properties": {"ok": {"type": "boolean"}}, "required": ["ok"]}}}}}}
        }
      }
    }
  },
  "components": {"schemas": {}}
}`

// runtimeHarness is written into the generated module and exercises the runtime
// against a real httptest server: envelope unwrap, per-call header injection,
// response-header access, typed problem+json errors, and SSE streaming.
const runtimeHarness = `package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type vendorCodec struct{}

func (vendorCodec) MediaType() string { return "application/vnd.tyche+json" }
func (vendorCodec) MatchesResponse(contentType string) bool {
	if i := strings.IndexByte(contentType, ';'); i >= 0 {
		contentType = contentType[:i]
	}
	contentType = strings.ToLower(strings.TrimSpace(contentType))
	if contentType == "" {
		return true
	}
	return contentType == "application/vnd.tyche+json" || contentType == "application/json"
}
func (vendorCodec) Marshal(v any) ([]byte, error) {
	return json.Marshal(v)
}
func (vendorCodec) Unmarshal(data []byte, v any) error {
	return json.Unmarshal(data, v)
}

func TestRuntimeBehavior(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/ping", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Custom") != "hi" {
			t.Errorf("per-call header not sent: %q", r.Header.Get("X-Custom"))
		}
		if got := r.Header.Get("Accept"); got != "application/json" {
			t.Errorf("ping accept = %q", got)
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
		if r.Header.Get("X-Bad-Stream") == "1" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(` + "`" + `{"data":{"message":"not a stream"}}` + "`" + `))
			return
		}
		w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
		f := w.(http.Flusher)
		_, _ = w.Write([]byte("event: token\nid: 1\nretry: 2500\ndata: {\"message\":\"a\"}\n\n"))
		f.Flush()
		for _, m := range []string{"b", "c"} {
			_, _ = w.Write([]byte("data: {\"message\":\"" + m + "\"}\n\n"))
			f.Flush()
		}
	})
	mux.HandleFunc("/upload", func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data;") {
			t.Errorf("expected multipart content type, got %q", r.Header.Get("Content-Type"))
		}
		if err := r.ParseMultipartForm(32 << 20); err != nil {
			t.Errorf("ParseMultipartForm: %v", err)
			w.WriteHeader(400)
			return
		}
		avatar := r.MultipartForm.File["avatar"]
		docs := r.MultipartForm.File["docs"]
		ok := r.FormValue("title") == "portrait" &&
			r.FormValue("count") == "7" &&
			len(r.MultipartForm.Value["tags"]) == 2 &&
			len(avatar) == 1 &&
			avatar[0].Filename == "me.txt" &&
			avatar[0].Header.Get("Content-Type") == "text/plain" &&
			len(docs) == 1 &&
			docs[0].Filename == "doc.txt"
		w.Header().Set("Content-Type", "application/json")
		if ok {
			_, _ = w.Write([]byte("{\"data\":{\"ok\":true}}"))
		} else {
			_, _ = w.Write([]byte("{\"data\":{\"ok\":false}}"))
		}
	})
	mux.HandleFunc("/custom", func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Content-Type"); got != "application/vnd.tyche+json" {
			t.Errorf("custom codec content type = %q", got)
		}
		if got := r.Header.Get("Accept"); got != "application/vnd.tyche+json" {
			t.Errorf("custom codec accept = %q", got)
		}
		w.Header().Set("Content-Type", "application/vnd.tyche+json")
		_, _ = w.Write([]byte("{\"data\":{\"ok\":true}}"))
	})
	mux.HandleFunc("/report", func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Accept"); got != "application/pdf" {
			t.Errorf("report accept = %q", got)
		}
		w.Header().Set("Content-Type", "application/pdf")
		_, _ = w.Write([]byte("%PDF"))
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
		if len(got) == 0 {
			if s.EventName() != "token" || s.ID() != "1" || s.Retry() != 2500 {
				t.Fatalf("stream metadata wrong: event=%q id=%q retry=%d", s.EventName(), s.ID(), s.Retry())
			}
		} else if s.EventName() != "" || s.ID() != "" || s.Retry() != 0 {
			t.Fatalf("stream metadata leaked: event=%q id=%q retry=%d", s.EventName(), s.ID(), s.Retry())
		}
		got = append(got, s.Event().Message)
	}
	if err := s.Err(); err != nil {
		t.Fatalf("stream error: %v", err)
	}
	if len(got) != 3 || got[0] != "a" || got[2] != "c" {
		t.Errorf("stream events wrong: %v", got)
	}

	bad, err := c.StreamEvents(context.Background(), &StreamEventsInput{},
		WithRequestHeader("X-Bad-Stream", "1"))
	if err == nil {
		if bad != nil {
			_ = bad.Close()
		}
		t.Fatal("expected content-type error for non-SSE 2xx stream response")
	}
	if !strings.Contains(err.Error(), ` + "`" + `text/event-stream` + "`" + `) || !strings.Contains(err.Error(), ` + "`" + `application/json` + "`" + `) {
		t.Fatalf("unexpected content-type error: %v", err)
	}

	upload, err := c.UploadAvatar(context.Background(), &UploadAvatarInput{
		Avatar: &File{Name: "me.txt", Content: strings.NewReader("avatar"), ContentType: "text/plain"},
		Count: 7,
		Docs: []File{{Name: "doc.txt", Content: bytes.NewBufferString("doc")}},
		Tags: []string{"profile", "public"},
		Title: "portrait",
	})
	if err != nil {
		t.Fatalf("UploadAvatar: %v", err)
	}
	if !upload.Ok {
		t.Fatalf("multipart upload was not encoded as expected")
	}

	report, err := c.DownloadReport(context.Background(), &DownloadReportInput{})
	if err != nil {
		t.Fatalf("DownloadReport: %v", err)
	}
	if string(report) != "%PDF" {
		t.Fatalf("raw response body = %q", report)
	}

	customClient := New(srv.URL, WithCodec(vendorCodec{}))
	custom, err := customClient.CustomCodec(context.Background(), &CustomCodecInput{
		Body: &CustomCodecBody{Name: "Ada"},
	})
	if err != nil {
		t.Fatalf("CustomCodec: %v", err)
	}
	if !custom.Ok {
		t.Fatalf("custom codec response was not decoded")
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
