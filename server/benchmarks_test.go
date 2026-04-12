package server_test

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/webdeveloperben/tyche/server"
	"github.com/webdeveloperben/tyche/server/validation"
)

type benchmarkValidationNestedItem struct {
	ID    string   `json:"id" validate:"uuid"`
	URL   string   `json:"url" validate:"url"`
	Tags  []string `json:"tags" validate:"minItems=1,items.min=2"`
	Email string   `json:"email" validate:"email"`
}

type benchmarkValidationInput struct {
	Name  string                          `json:"name" validate:"min=2,max=32"`
	Code  string                          `json:"code" validate:"pattern=^[A-Z]{2}$"`
	Items []benchmarkValidationNestedItem `json:"items"`
}

func BenchmarkRouter_StaticRoute(b *testing.B) {
	router := server.NewRouter()
	router.GET("/api/users", func(w http.ResponseWriter, r *http.Request) error {
		return nil
	})

	req := httptest.NewRequest(http.MethodGet, "/api/users", nil)
	w := httptest.NewRecorder()

	b.ReportAllocs()

	for b.Loop() {
		router.ServeHTTP(w, req)
	}
}

func BenchmarkRouter_ParamRoute(b *testing.B) {
	router := server.NewRouter()
	router.GET("/api/users/:id", func(w http.ResponseWriter, r *http.Request) error {
		_ = server.Param(r, "id")
		return nil
	})

	req := httptest.NewRequest(http.MethodGet, "/api/users/12345", nil)
	w := httptest.NewRecorder()

	b.ReportAllocs()

	for b.Loop() {
		router.ServeHTTP(w, req)
	}
}

func BenchmarkRouter_WildcardRoute(b *testing.B) {
	router := server.NewRouter()
	router.GET("/files/*", func(w http.ResponseWriter, r *http.Request) error {
		_ = server.Wildcard(r)
		return nil
	})

	req := httptest.NewRequest(http.MethodGet, "/files/a/b/c/d/e.txt", nil)
	w := httptest.NewRecorder()

	b.ReportAllocs()

	for b.Loop() {
		router.ServeHTTP(w, req)
	}
}

func BenchmarkRouter_DeepNesting(b *testing.B) {
	router := server.NewRouter()
	router.GET("/a/b/c/d/e/f", func(w http.ResponseWriter, r *http.Request) error {
		return nil
	})

	req := httptest.NewRequest(http.MethodGet, "/a/b/c/d/e/f", nil)
	w := httptest.NewRecorder()

	b.ReportAllocs()

	for b.Loop() {
		router.ServeHTTP(w, req)
	}
}

func BenchmarkRouter_NotFound(b *testing.B) {
	router := server.NewRouter()
	router.GET("/api/users", func(w http.ResponseWriter, r *http.Request) error {
		return nil
	})

	req := httptest.NewRequest(http.MethodGet, "/api/users/notfound", nil)
	w := httptest.NewRecorder()

	b.ReportAllocs()

	for b.Loop() {
		router.ServeHTTP(w, req)
	}
}

func BenchmarkRouter_ManyStaticRoutes(b *testing.B) {
	router := server.NewRouter()
	routes := []string{
		"/api/users", "/api/posts", "/api/comments", "/api/tags",
		"/api/users/:id", "/api/posts/:id", "/api/comments/:id",
		"/api/v1/users", "/api/v1/posts", "/api/v2/users", "/api/v2/posts",
		"/admin/users", "/admin/posts", "/admin/settings",
		"/files/*", "/assets/*",
	}
	for _, route := range routes {
		router.GET(route, func(w http.ResponseWriter, r *http.Request) error {
			return nil
		})
	}

	req := httptest.NewRequest(http.MethodGet, "/api/users", nil)
	w := httptest.NewRecorder()

	b.ReportAllocs()

	for b.Loop() {
		router.ServeHTTP(w, req)
	}
}

func BenchmarkRouter_ParamLookup(b *testing.B) {
	router := server.NewRouter()
	router.GET("/api/:version/:resource/:id", func(w http.ResponseWriter, r *http.Request) error {
		_ = server.Param(r, "version")
		_ = server.Param(r, "resource")
		_ = server.Param(r, "id")
		return nil
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/users/123", nil)
	w := httptest.NewRecorder()

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		router.ServeHTTP(w, req)
	}
}

func BenchmarkSplitRoute(b *testing.B) {
	routes := []string{
		"/api/users",
		"/api/v1/users/:id/posts/:postId",
		"/files/a/b/c/d/e.txt",
	}

	b.ReportAllocs()

	for i := 0; b.Loop(); i++ {
		_ = server.SplitRouteFast(routes[i%len(routes)])
	}
}

func BenchmarkStringOps(b *testing.B) {
	path := "/api/v1/users/123/posts/456"

	b.ReportAllocs()

	for b.Loop() {
		p := path[1:]
		p = strings.TrimLeft(p, "/")
		_ = strings.IndexByte(p, '/')
	}
}

func BenchmarkRouter_WithMiddleware(b *testing.B) {
	router := server.NewRouter()

	router.Use(func(next server.HandlerFunc) server.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) error {
			return next(w, r)
		}
	})

	router.GET("/api/users", func(w http.ResponseWriter, r *http.Request) error {
		return nil
	})

	req := httptest.NewRequest(http.MethodGet, "/api/users", nil)
	w := httptest.NewRecorder()

	b.ReportAllocs()

	for b.Loop() {
		router.ServeHTTP(w, req)
	}
}

func BenchmarkValidation_StructSpecCached(b *testing.B) {
	typ := reflect.TypeFor[benchmarkValidationInput]()

	b.ReportAllocs()
	for b.Loop() {
		if _, err := validation.Struct(typ); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkValidation_RuntimeValid(b *testing.B) {
	spec, err := validation.Struct(reflect.TypeFor[benchmarkValidationInput]())
	if err != nil {
		b.Fatal(err)
	}
	value := benchmarkValidationInput{
		Name: "Valid Name",
		Code: "AB",
		Items: []benchmarkValidationNestedItem{
			{
				ID:    "550e8400-e29b-41d4-a716-446655440000",
				URL:   "https://example.com",
				Tags:  []string{"ab", "cd"},
				Email: "person@example.com",
			},
			{
				ID:    "550e8400-e29b-41d4-a716-446655440001",
				URL:   "https://example.org",
				Tags:  []string{"ef"},
				Email: "other@example.com",
			},
		},
	}

	b.ReportAllocs()
	for b.Loop() {
		if err := validation.ValidateStructValue(reflect.ValueOf(value), spec, "request"); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkValidation_RuntimeInvalid(b *testing.B) {
	spec, err := validation.Struct(reflect.TypeFor[benchmarkValidationInput]())
	if err != nil {
		b.Fatal(err)
	}
	value := benchmarkValidationInput{
		Name: "x",
		Code: "bad",
		Items: []benchmarkValidationNestedItem{
			{
				ID:    "not-a-uuid",
				URL:   "bad-url",
				Tags:  []string{"x"},
				Email: "bad",
			},
		},
	}

	b.ReportAllocs()
	for b.Loop() {
		if err := validation.ValidateStructValue(reflect.ValueOf(value), spec, "request"); err == nil {
			b.Fatal("expected validation error")
		}
	}
}

func BenchmarkValidation_RequiredJSONFieldsNestedArray(b *testing.B) {
	type child struct {
		Code string `json:"code"`
	}
	type payload struct {
		Children []child `json:"children"`
	}

	required := server.RequiredJSONFields(reflect.TypeFor[payload](), nil, nil)
	body := []byte(`{"children":[{"code":"AB"},{"code":"CD"},{"code":"EF"}]}`)

	b.ReportAllocs()
	for b.Loop() {
		if err := server.ValidateRequiredJSONFields(body, required); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkValidation_StringLengthASCII(b *testing.B) {
	value := "plain-ascii-text"

	b.ReportAllocs()
	for b.Loop() {
		if validation.StringLength(value) == 0 {
			b.Fatal("unexpected zero length")
		}
	}
}

func BenchmarkValidation_StringLengthUnicode(b *testing.B) {
	value := "👍🏽⚙️validation"

	b.ReportAllocs()
	for b.Loop() {
		if validation.StringLength(value) == 0 {
			b.Fatal("unexpected zero length")
		}
	}
}

func BenchmarkRouter_WithMultipleMiddleware(b *testing.B) {
	router := server.NewRouter()

	for range 3 {
		router.Use(func(next server.HandlerFunc) server.HandlerFunc {
			return func(w http.ResponseWriter, r *http.Request) error {
				return next(w, r)
			}
		})
	}

	router.GET("/api/users", func(w http.ResponseWriter, r *http.Request) error {
		return nil
	})

	req := httptest.NewRequest(http.MethodGet, "/api/users", nil)
	w := httptest.NewRecorder()

	b.ReportAllocs()

	for b.Loop() {
		router.ServeHTTP(w, req)
	}
}

func BenchmarkRouter_405MethodNotAllowed(b *testing.B) {
	router := server.NewRouter()
	router.POST("/api/users", func(w http.ResponseWriter, r *http.Request) error {
		return nil
	})

	req := httptest.NewRequest(http.MethodGet, "/api/users", nil)
	w := httptest.NewRecorder()

	b.ReportAllocs()

	for b.Loop() {
		router.ServeHTTP(w, req)
	}
}

func BenchmarkRouter_RootRoute(b *testing.B) {
	router := server.NewRouter()
	router.GET("/", func(w http.ResponseWriter, r *http.Request) error {
		return nil
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()

	b.ReportAllocs()

	for b.Loop() {
		router.ServeHTTP(w, req)
	}
}

func BenchmarkRouter_GroupRouting(b *testing.B) {
	router := server.NewRouter()
	g := router.Group("/api")
	g.GET("/users", func(w http.ResponseWriter, r *http.Request) error {
		return nil
	})
	g.GET("/posts", func(w http.ResponseWriter, r *http.Request) error {
		return nil
	})

	req := httptest.NewRequest(http.MethodGet, "/api/users", nil)
	w := httptest.NewRecorder()

	b.ReportAllocs()

	for b.Loop() {
		router.ServeHTTP(w, req)
	}
}

func BenchmarkRouter_MixedRoutes(b *testing.B) {
	router := server.NewRouter()

	api := router.Group("/api/v1")
	api.GET("/users", func(w http.ResponseWriter, r *http.Request) error { return nil })
	api.GET("/users/:id", func(w http.ResponseWriter, r *http.Request) error {
		_ = server.Param(r, "id")
		return nil
	})
	api.POST("/users", func(w http.ResponseWriter, r *http.Request) error { return nil })
	api.DELETE("/users/:id", func(w http.ResponseWriter, r *http.Request) error { return nil })

	admin := router.Group("/admin")
	admin.GET("/settings", func(w http.ResponseWriter, r *http.Request) error { return nil })
	admin.POST("/settings", func(w http.ResponseWriter, r *http.Request) error { return nil })

	files := router.Group("/files")
	files.GET("/*", func(w http.ResponseWriter, r *http.Request) error {
		_ = server.Wildcard(r)
		return nil
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/users/42", nil)
	w := httptest.NewRecorder()

	b.ReportAllocs()

	for b.Loop() {
		router.ServeHTTP(w, req)
	}
}

func BenchmarkRouter_LargePath(b *testing.B) {
	router := server.NewRouter()
	router.GET("/api/v1/users/profile/settings/security/two-factor/enable", func(w http.ResponseWriter, r *http.Request) error {
		return nil
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/users/profile/settings/security/two-factor/enable", nil)
	w := httptest.NewRecorder()

	b.ReportAllocs()

	for b.Loop() {
		router.ServeHTTP(w, req)
	}
}

func BenchmarkHasPathTraversal(b *testing.B) {
	paths := []string{
		"/api/users",
		"/api/../etc/passwd",
		"/api/v1/..%2f..%2fetc/passwd",
		"/files/../../../etc/passwd",
		"/api/./users",
		"/api/v1/users/../../admin",
	}

	b.ReportAllocs()

	for i := 0; b.Loop(); i++ {
		_ = server.HasPathTraversal(paths[i%len(paths)])
	}
}

func BenchmarkTreeFind(b *testing.B) {
	router := server.NewRouter()
	router.GET("/api/users", func(w http.ResponseWriter, r *http.Request) error { return nil })
	router.GET("/api/users/:id", func(w http.ResponseWriter, r *http.Request) error { return nil })
	router.GET("/api/posts/:id", func(w http.ResponseWriter, r *http.Request) error { return nil })

	req := httptest.NewRequest(http.MethodGet, "/api/users/123", nil)
	path := req.URL.Path

	b.ReportAllocs()

	for b.Loop() {
		_ = router.Root.Find(http.MethodGet, path)
	}
}

func BenchmarkMethodIndex(b *testing.B) {
	methods := []string{
		http.MethodGet,
		http.MethodPost,
		http.MethodPut,
		http.MethodDelete,
		http.MethodPatch,
		http.MethodOptions,
		http.MethodHead,
	}

	b.ReportAllocs()

	for i := 0; b.Loop(); i++ {
		_ = server.MethodIndex(methods[i%len(methods)])
	}
}

func BenchmarkRouter_RealisticAPI(b *testing.B) {
	router := server.NewRouter()

	router.GET("/", func(w http.ResponseWriter, r *http.Request) error { return nil })
	router.GET("/health", func(w http.ResponseWriter, r *http.Request) error { return nil })

	api := router.Group("/api/v1")

	api.GET("/users", func(w http.ResponseWriter, r *http.Request) error { return nil })
	api.GET("/users/:id", func(w http.ResponseWriter, r *http.Request) error {
		_ = server.Param(r, "id")
		return nil
	})
	api.POST("/users", func(w http.ResponseWriter, r *http.Request) error { return nil })
	api.PUT("/users/:id", func(w http.ResponseWriter, r *http.Request) error {
		_ = server.Param(r, "id")
		return nil
	})
	api.DELETE("/users/:id", func(w http.ResponseWriter, r *http.Request) error {
		_ = server.Param(r, "id")
		return nil
	})

	api.GET("/posts", func(w http.ResponseWriter, r *http.Request) error { return nil })
	api.GET("/posts/:slug", func(w http.ResponseWriter, r *http.Request) error {
		_ = server.Param(r, "slug")
		return nil
	})
	api.POST("/posts", func(w http.ResponseWriter, r *http.Request) error { return nil })

	api.GET("/comments", func(w http.ResponseWriter, r *http.Request) error { return nil })
	api.GET("/comments/:id", func(w http.ResponseWriter, r *http.Request) error {
		_ = server.Param(r, "id")
		return nil
	})

	api.GET("/files/*", func(w http.ResponseWriter, r *http.Request) error {
		_ = server.Wildcard(r)
		return nil
	})

	paths := []string{
		"/",
		"/health",
		"/api/v1/users",
		"/api/v1/users/123",
		"/api/v1/posts",
		"/api/v1/posts/my-slug",
		"/api/v1/comments",
		"/api/v1/files/doc.pdf",
	}

	req := httptest.NewRequest(http.MethodGet, paths[0], nil)
	w := httptest.NewRecorder()

	b.ReportAllocs()

	for i := 0; b.Loop(); i++ {
		req.URL.Path = paths[i%len(paths)]
		router.ServeHTTP(w, req)
	}
}
