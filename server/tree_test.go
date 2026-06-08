package server_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/webdeveloperben/tyche/server"
)

func TestRouter_Params(t *testing.T) {
	router := server.NewRouter()

	router.GET("/users/:id", func(w http.ResponseWriter, r *http.Request) error {
		_, _ = w.Write([]byte(server.Param(r, "id")))
		return nil
	})

	tests := []struct {
		path     string
		expected string
	}{
		{"/users/123", "123"},
		{"/users/abc", "abc"},
		{"/users/42", "42"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Errorf("expected status 200, got %d", w.Code)
			}
			if w.Body.String() != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, w.Body.String())
			}
		})
	}
}

func TestRouter_ParamHelper(t *testing.T) {
	router := server.NewRouter()

	router.GET("/users/:id", func(w http.ResponseWriter, r *http.Request) error {
		_, _ = w.Write([]byte(server.Param(r, "id")))
		return nil
	})

	req := httptest.NewRequest(http.MethodGet, "/users/123", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
	if w.Body.String() != "123" {
		t.Errorf("expected '123', got %s", w.Body.String())
	}
}

func TestRouter_MultipleParams(t *testing.T) {
	router := server.NewRouter()

	router.GET("/users/:userId/posts/:postId", func(w http.ResponseWriter, r *http.Request) error {
		_, _ = w.Write([]byte(server.Param(r, "userId") + "-" + server.Param(r, "postId")))
		return nil
	})

	req := httptest.NewRequest(http.MethodGet, "/users/10/posts/20", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
	if w.Body.String() != "10-20" {
		t.Errorf("expected '10-20', got %s", w.Body.String())
	}
}

func TestRouter_ParamAtStart(t *testing.T) {
	router := server.NewRouter()

	router.GET("/:resource", func(w http.ResponseWriter, r *http.Request) error {
		_, _ = w.Write([]byte(server.Param(r, "resource")))
		return nil
	})

	req := httptest.NewRequest(http.MethodGet, "/items", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
	if w.Body.String() != "items" {
		t.Errorf("expected 'items', got %s", w.Body.String())
	}
}

func TestRouter_ParamWithHyphen(t *testing.T) {
	router := server.NewRouter()

	router.GET("/users/:user-id", func(w http.ResponseWriter, r *http.Request) error {
		_, _ = w.Write([]byte(server.Param(r, "user-id")))
		return nil
	})

	req := httptest.NewRequest(http.MethodGet, "/users/user-123", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
	if w.Body.String() != "user-123" {
		t.Errorf("expected 'user-123', got %s", w.Body.String())
	}
}

func TestRouter_Wildcard(t *testing.T) {
	router := server.NewRouter()

	router.GET("/files/*", func(w http.ResponseWriter, r *http.Request) error {
		_, _ = w.Write([]byte(server.Wildcard(r)))
		return nil
	})

	tests := []struct {
		path     string
		expected string
	}{
		{"/files/doc.txt", "doc.txt"},
		{"/files/a/b/c.txt", "a/b/c.txt"},
		{"/files/", ""},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Errorf("expected status 200, got %d", w.Code)
			}
			if w.Body.String() != tt.expected {
				t.Errorf("expected '%s', got '%s'", tt.expected, w.Body.String())
			}
		})
	}
}

func TestRouter_WildcardHelper(t *testing.T) {
	router := server.NewRouter()

	router.GET("/files/*", func(w http.ResponseWriter, r *http.Request) error {
		_, _ = w.Write([]byte(server.Wildcard(r)))
		return nil
	})

	req := httptest.NewRequest(http.MethodGet, "/files/a/b/c.txt", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
	if w.Body.String() != "a/b/c.txt" {
		t.Errorf("expected 'a/b/c.txt', got '%s'", w.Body.String())
	}
}

func TestRouter_NamedWildcardHelper(t *testing.T) {
	router := server.NewRouter()

	router.GET("/files/*path", func(w http.ResponseWriter, r *http.Request) error {
		_, _ = w.Write([]byte(server.Param(r, "path") + ":" + server.Wildcard(r)))
		return nil
	})

	req := httptest.NewRequest(http.MethodGet, "/files/a/b/c.txt", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
	if w.Body.String() != "a/b/c.txt:a/b/c.txt" {
		t.Errorf("expected named and wildcard values, got '%s'", w.Body.String())
	}
}

func TestRouter_RejectsNonTerminalWildcard(t *testing.T) {
	router := server.NewRouter()

	err := router.HandleE(http.MethodGet, "/files/*path/more", func(w http.ResponseWriter, r *http.Request) error {
		return nil
	})
	if err == nil {
		t.Fatal("expected invalid wildcard pattern to be rejected")
	}
}

func TestRouter_MixedParamsAndWildcard(t *testing.T) {
	router := server.NewRouter()

	router.GET("/api/v1/:version/*path", func(w http.ResponseWriter, r *http.Request) error {
		_, _ = w.Write([]byte(server.Param(r, "version") + ":" + server.Wildcard(r)))
		return nil
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/v2/users/list", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
	if w.Body.String() != "v2:users/list" {
		t.Errorf("expected 'v2:users/list', got %s", w.Body.String())
	}
}

func TestRouter_PriorityStaticOverParam(t *testing.T) {
	router := server.NewRouter()

	router.GET("/users", func(w http.ResponseWriter, r *http.Request) error {
		_, _ = w.Write([]byte("users list"))
		return nil
	})
	router.GET("/users/:id", func(w http.ResponseWriter, r *http.Request) error {
		_, _ = w.Write([]byte("user " + server.Param(r, "id")))
		return nil
	})

	req := httptest.NewRequest(http.MethodGet, "/users", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
	if w.Body.String() != "users list" {
		t.Errorf("expected 'users list', got %s", w.Body.String())
	}

	req2 := httptest.NewRequest(http.MethodGet, "/users/123", nil)
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)

	if w2.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w2.Code)
	}
	if w2.Body.String() != "user 123" {
		t.Errorf("expected 'user 123', got %s", w2.Body.String())
	}
}

func TestRouter_DeepNesting(t *testing.T) {
	router := server.NewRouter()

	router.GET("/a/b/c/d/e", func(w http.ResponseWriter, r *http.Request) error {
		_, _ = w.Write([]byte("deep"))
		return nil
	})

	req := httptest.NewRequest(http.MethodGet, "/a/b/c/d/e", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
	if w.Body.String() != "deep" {
		t.Errorf("expected 'deep', got %s", w.Body.String())
	}
}

func TestRouter_PartialMatch(t *testing.T) {
	router := server.NewRouter()

	var called bool
	router.GET("/users", func(w http.ResponseWriter, r *http.Request) error {
		called = true
		return nil
	})

	req := httptest.NewRequest(http.MethodGet, "/users/extra", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if called {
		t.Error("should not match partial path")
	}
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 for unmatched path, got %d", w.Code)
	}
}

func TestRouter_ConflictingRoutes(t *testing.T) {
	router := server.NewRouter()

	var staticCalled, paramCalled bool
	router.GET("/items", func(w http.ResponseWriter, r *http.Request) error {
		staticCalled = true
		return nil
	})
	router.GET("/items/new", func(w http.ResponseWriter, r *http.Request) error {
		paramCalled = true
		return nil
	})

	req := httptest.NewRequest(http.MethodGet, "/items", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if !staticCalled {
		t.Error("/items should match static route")
	}

	staticCalled = false
	req = httptest.NewRequest(http.MethodGet, "/items/new", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if !paramCalled {
		t.Error("/items/new should match static route")
	}
}

func TestRouter_AdjacentParamSegments(t *testing.T) {
	router := server.NewRouter()

	router.GET("/:a:b", func(w http.ResponseWriter, r *http.Request) error {
		_, _ = w.Write([]byte(server.Param(r, "a") + "-" + server.Param(r, "b")))
		return nil
	})

	req := httptest.NewRequest(http.MethodGet, "/xy", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

func TestRouter_EmptySegments(t *testing.T) {
	router := server.NewRouter()

	router.GET("/users//posts", func(w http.ResponseWriter, r *http.Request) error {
		_, _ = w.Write([]byte("success"))
		return nil
	})

	req := httptest.NewRequest(http.MethodGet, "/users/posts", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 for collapsed double slash, got %d", w.Code)
	}
}

func TestRouter_RootHandler(t *testing.T) {
	router := server.NewRouter()

	router.GET("/", func(w http.ResponseWriter, r *http.Request) error {
		_, _ = w.Write([]byte("home"))
		return nil
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
	if w.Body.String() != "home" {
		t.Errorf("expected 'home', got %s", w.Body.String())
	}
}

func TestRouter_OPTIONSMethod(t *testing.T) {
	router := server.NewRouter()

	var optionsCalled bool
	router.OPTIONS("/api", func(w http.ResponseWriter, r *http.Request) error {
		optionsCalled = true
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		return nil
	})

	req := httptest.NewRequest(http.MethodOptions, "/api", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if !optionsCalled {
		t.Error("OPTIONS handler was not called")
	}
	if w.Header().Get("Access-Control-Allow-Methods") != "GET, POST, OPTIONS" {
		t.Errorf("unexpected headers: %v", w.Header())
	}
}

func TestRouter_HeadMethod(t *testing.T) {
	router := server.NewRouter()

	var headCalled bool
	router.HEAD("/resource", func(w http.ResponseWriter, r *http.Request) error {
		headCalled = true
		return nil
	})

	req := httptest.NewRequest(http.MethodHead, "/resource", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if !headCalled {
		t.Error("HEAD handler was not called")
	}
}

func TestRouter_MultipleParamValues(t *testing.T) {
	router := server.NewRouter()

	router.GET("/users/:id/posts/:postId/comments/:commentId", func(w http.ResponseWriter, r *http.Request) error {
		_, _ = w.Write([]byte(server.Param(r, "id") + "/" + server.Param(r, "postId") + "/" + server.Param(r, "commentId")))
		return nil
	})

	req := httptest.NewRequest(http.MethodGet, "/users/1/posts/2/comments/3", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
	if w.Body.String() != "1/2/3" {
		t.Errorf("expected '1/2/3', got %s", w.Body.String())
	}
}

func TestRouter_WildcardWithSlashOnly(t *testing.T) {
	router := server.NewRouter()

	router.GET("/static/*", func(w http.ResponseWriter, r *http.Request) error {
		_, _ = w.Write([]byte(server.Wildcard(r)))
		return nil
	})

	req := httptest.NewRequest(http.MethodGet, "/static/", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
	if w.Body.String() != "" {
		t.Errorf("expected empty string, got '%s'", w.Body.String())
	}
}
