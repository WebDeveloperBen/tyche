package apidocs_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/webdeveloperben/tyche/server"
	"github.com/webdeveloperben/tyche/server/apidocs"
)

func TestMount_WithScalarAndRedoc(t *testing.T) {
	router := server.NewAPI(server.NewServeMuxAdapter(), server.APIConfig{
		OpenAPI: server.OpenAPIInfo{
			Title: "Homebase API",
		},
	})

	if err := apidocs.Mount(router, apidocs.Config{
		SpecPath: "/openapi.json",
		UIs: []apidocs.UIMount{
			{Path: "/docs", Renderer: apidocs.Scalar()},
			{Path: "/redoc", Renderer: apidocs.Redoc()},
		},
	}); err != nil {
		t.Fatalf("Mount failed: %v", err)
	}

	openAPIReq := httptest.NewRequest(http.MethodGet, "/openapi.json", nil)
	openAPIResp := httptest.NewRecorder()
	router.ServeHTTP(openAPIResp, openAPIReq)
	if openAPIResp.Code != http.StatusOK {
		t.Fatalf("expected OpenAPI status 200, got %d", openAPIResp.Code)
	}

	docsReq := httptest.NewRequest(http.MethodGet, "/docs", nil)
	docsResp := httptest.NewRecorder()
	router.ServeHTTP(docsResp, docsReq)
	if docsResp.Code != http.StatusOK {
		t.Fatalf("expected Scalar status 200, got %d", docsResp.Code)
	}
	if !strings.Contains(docsResp.Body.String(), "Scalar.createApiReference") {
		t.Fatalf("expected Scalar bootstrap, got %s", docsResp.Body.String())
	}
	if !strings.Contains(docsResp.Body.String(), "Homebase API") {
		t.Fatalf("expected docs title to default from router OpenAPI info, got %s", docsResp.Body.String())
	}

	redocReq := httptest.NewRequest(http.MethodGet, "/redoc", nil)
	redocResp := httptest.NewRecorder()
	router.ServeHTTP(redocResp, redocReq)
	if redocResp.Code != http.StatusOK {
		t.Fatalf("expected Redoc status 200, got %d", redocResp.Code)
	}
	if !strings.Contains(redocResp.Body.String(), "<redoc spec-url=") {
		t.Fatalf("expected Redoc bootstrap, got %s", redocResp.Body.String())
	}
}
