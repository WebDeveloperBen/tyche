package server_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/webdeveloperben/tyche/server"
)

func TestAddSecurityScheme_AppearsInOpenAPI(t *testing.T) {
	router := server.NewRouter()
	router.AddSecurityScheme("bearerAuth", server.BearerScheme("JWT"))
	router.AddSecurityScheme("apiKey", server.APIKeyScheme("X-API-Key", "header"))

	doc := router.OpenAPI()
	if doc.Components == nil || doc.Components.SecuritySchemes == nil {
		t.Fatal("security schemes not registered")
	}
	bearer := doc.Components.SecuritySchemes["bearerAuth"]
	if bearer == nil || bearer.Type != "http" || bearer.Scheme != "bearer" || bearer.BearerFormat != "JWT" {
		t.Errorf("unexpected bearer scheme: %+v", bearer)
	}
	key := doc.Components.SecuritySchemes["apiKey"]
	if key == nil || key.Type != "apiKey" || key.In != "header" || key.Name != "X-API-Key" {
		t.Errorf("unexpected apiKey scheme: %+v", key)
	}
}

func TestSecurityScheme_SerializesToJSON(t *testing.T) {
	router := server.NewRouter()
	router.AddSecurityScheme("basicAuth", server.BasicScheme())

	body, err := json.Marshal(router.OpenAPI())
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	out := string(body)
	if !strings.Contains(out, `"securitySchemes"`) || !strings.Contains(out, `"basicAuth"`) {
		t.Errorf("securitySchemes missing from OpenAPI JSON: %s", out)
	}
	if !strings.Contains(out, `"scheme":"basic"`) {
		t.Errorf("basic scheme not serialized: %s", out)
	}
}

func TestRegisterStream_OperationSecurity(t *testing.T) {
	router := server.NewRouter()
	router.AddSecurityScheme("bearerAuth", server.BearerScheme("JWT"))
	api := router.Group("/v1")

	server.RegisterStream(api, server.Operation{
		OperationID: "secure-stream",
		Method:      http.MethodGet,
		Path:        "/secure",
		Security:    []server.SecurityRequirement{{"bearerAuth": {}}},
	}, func(ctx context.Context, in *streamInput, stream *server.Stream[tokenEvent]) error {
		return nil
	})

	op := router.OpenAPI().Paths["/v1/secure"].GET
	if len(op.Security) != 1 {
		t.Fatalf("expected 1 security requirement, got %d", len(op.Security))
	}
	if _, ok := op.Security[0]["bearerAuth"]; !ok {
		t.Errorf("expected bearerAuth requirement, got %+v", op.Security[0])
	}
}

func TestSecurityRequirement_NilScopesSerializeAsArray(t *testing.T) {
	router := server.NewRouter()
	router.AddSecurityScheme("bearerAuth", server.BearerScheme("JWT"))
	api := router.Group("/v1")

	// nil scope slice must serialize as [] (valid OpenAPI), not null.
	server.RegisterStream(api, server.Operation{
		OperationID: "nil-scope-stream",
		Method:      http.MethodGet,
		Path:        "/s",
		Security:    []server.SecurityRequirement{{"bearerAuth": nil}},
	}, func(ctx context.Context, in *streamInput, stream *server.Stream[tokenEvent]) error {
		return nil
	})

	body, err := json.Marshal(router.OpenAPI())
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	out := string(body)
	if strings.Contains(out, `"bearerAuth":null`) {
		t.Errorf("nil scopes serialized as null (invalid OpenAPI):\n%s", out)
	}
	if !strings.Contains(out, `"bearerAuth":[]`) {
		t.Errorf("expected bearerAuth scopes as [], got:\n%s", out)
	}
}

// ensure the security alias is usable directly too.
func TestSecuritySchemeAlias(t *testing.T) {
	s := server.SecurityScheme{Type: "http", Scheme: "bearer"}
	router := server.NewRouter()
	router.AddSecurityScheme("x", &s)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/missing", nil)) // smoke
	if rec.Code != http.StatusNotFound {
		t.Errorf("unexpected status %d", rec.Code)
	}
}
