package embeddedpkg

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/webdeveloperben/tyche/server"
)

func TestEmbeddedPaginationRoute(t *testing.T) {
	api := server.NewAPI(server.NewServeMuxAdapter())
	RegisterRoutes(api.Group(""))

	tests := []struct {
		name string
		path string
		want string
	}{
		{name: "configured", path: "/tenants/acme?limit=7&cursor=next", want: `"name":"acme:7:next"`},
		{name: "default", path: "/tenants/acme", want: `"name":"acme:5:"`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, test.path, nil)
			req.SetPathValue("tenant_id", "acme")
			response := httptest.NewRecorder()
			api.ServeHTTP(response, req)

			if response.Code != http.StatusOK {
				t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
			}
			if !strings.Contains(response.Body.String(), test.want) {
				t.Fatalf("body = %s, want %s", response.Body.String(), test.want)
			}
		})
	}
	req := httptest.NewRequest(http.MethodGet, "/tenants/acme?limit=11", nil)
	req.SetPathValue("tenant_id", "acme")
	response := httptest.NewRecorder()
	api.ServeHTTP(response, req)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("invalid limit status = %d, body = %s; want 400", response.Code, response.Body.String())
	}
}
