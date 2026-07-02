package server_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/webdeveloperben/tyche/server"
	"github.com/webdeveloperben/tyche/server/apidocs"
	"github.com/webdeveloperben/tyche/server/validation"
)

type getUserInput struct {
	ID string `path:"id" doc:"User ID"`
}

type getUserOutput struct {
	Name  string `json:"name" doc:"User name"`
	Email string `json:"email" doc:"User email"`
}

type createUserInput struct {
	Name  string `json:"name" doc:"User name" validate:"min=1,max=255"`
	Email string `json:"email" doc:"User email"`
}

type createUserOutput struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

type wrappedCreateUserOutput struct {
	Status int    `status:"201"`
	Trace  string `header:"X-Trace-ID" doc:"Trace identifier"`
	Body   struct {
		ID string `json:"id"`
	} `body:"true"`
}

type sharedStatusOutput struct {
	Body struct {
		ID string `json:"id"`
	} `body:"true"`
}

var typedTestCodecsOnce sync.Once

func registerTypedTestCodecs() {
	typedTestCodecsOnce.Do(func() {
		server.RegisterGeneratedCodec(server.GeneratedRouteMeta{
			PackagePath:       "github.com/webdeveloperben/tyche/server_test",
			OperationID:       "get-user",
			Method:            http.MethodGet,
			Path:              "/users/:id",
			HasGeneratedCodec: true,
		}, server.GeneratedRouteCodec{
			Parse: func(req *http.Request) (any, error) {
				return &getUserInput{ID: req.PathValue("id")}, nil
			},
			Write: func(w http.ResponseWriter, req *http.Request, out any) error {
				typed, _ := out.(*getUserOutput)
				if req.Method == http.MethodHead {
					w.WriteHeader(http.StatusOK)
					return nil
				}
				return server.WriteSuccess(w, http.StatusOK, typed)
			},
		})
		server.RegisterGeneratedCodec(server.GeneratedRouteMeta{
			PackagePath:       "github.com/webdeveloperben/tyche/server_test",
			OperationID:       "create-user",
			Method:            http.MethodPost,
			Path:              "/users",
			HasGeneratedCodec: true,
		}, server.GeneratedRouteCodec{
			Parse: func(req *http.Request) (any, error) {
				var in createUserInput
				if err := server.DecodeRequestJSONBodyFast(req, &in); err != nil {
					return nil, err
				}
				return &in, nil
			},
			Write: func(w http.ResponseWriter, req *http.Request, out any) error {
				return server.WriteSuccess(w, http.StatusOK, out)
			},
		})
		server.RegisterGeneratedCodec(server.GeneratedRouteMeta{
			PackagePath:       "github.com/webdeveloperben/tyche/server_test",
			OperationID:       "create-wrapped-user",
			Method:            http.MethodPost,
			Path:              "/wrapped-users",
			HasGeneratedCodec: true,
		}, server.GeneratedRouteCodec{
			Parse: func(req *http.Request) (any, error) {
				var in createUserInput
				if err := server.DecodeRequestJSONBodyFast(req, &in); err != nil {
					return nil, err
				}
				return &in, nil
			},
			Write: func(w http.ResponseWriter, req *http.Request, out any) error {
				typed, _ := out.(*wrappedCreateUserOutput)
				w.Header().Set("X-Trace-ID", typed.Trace)
				if req.Method == http.MethodHead {
					w.WriteHeader(http.StatusCreated)
					return nil
				}
				return server.WriteSuccess(w, http.StatusCreated, typed.Body)
			},
		})
		server.RegisterGeneratedCodec(server.GeneratedRouteMeta{
			PackagePath:       "github.com/webdeveloperben/tyche/server_test",
			OperationID:       "create-a",
			Method:            http.MethodPost,
			Path:              "/a",
			HasGeneratedCodec: true,
		}, server.GeneratedRouteCodec{
			Parse: func(req *http.Request) (any, error) {
				var in createUserInput
				if err := server.DecodeRequestJSONBodyFast(req, &in); err != nil {
					return nil, err
				}
				return &in, nil
			},
			Write: func(w http.ResponseWriter, req *http.Request, out any) error {
				typed, _ := out.(*sharedStatusOutput)
				if req.Method == http.MethodHead {
					w.WriteHeader(http.StatusCreated)
					return nil
				}
				return server.WriteSuccess(w, http.StatusCreated, typed.Body)
			},
		})
		server.RegisterGeneratedCodec(server.GeneratedRouteMeta{
			PackagePath:       "github.com/webdeveloperben/tyche/server_test",
			OperationID:       "create-b",
			Method:            http.MethodPost,
			Path:              "/b",
			HasGeneratedCodec: true,
		}, server.GeneratedRouteCodec{
			Parse: func(req *http.Request) (any, error) {
				var in createUserInput
				if err := server.DecodeRequestJSONBodyFast(req, &in); err != nil {
					return nil, err
				}
				return &in, nil
			},
			Write: func(w http.ResponseWriter, req *http.Request, out any) error {
				typed, _ := out.(*sharedStatusOutput)
				if req.Method == http.MethodHead {
					w.WriteHeader(http.StatusOK)
					return nil
				}
				return server.WriteSuccess(w, http.StatusOK, typed.Body)
			},
		})
		server.RegisterGeneratedCodec(server.GeneratedRouteMeta{
			PackagePath:       "github.com/webdeveloperben/tyche/server_test",
			OperationID:       "error-user",
			Method:            http.MethodGet,
			Path:              "/users/:id",
			HasGeneratedCodec: true,
		}, server.GeneratedRouteCodec{
			Parse: func(req *http.Request) (any, error) {
				return &getUserInput{ID: req.PathValue("id")}, nil
			},
			Write: func(w http.ResponseWriter, req *http.Request, out any) error {
				typed, _ := out.(*getUserOutput)
				return server.WriteSuccess(w, http.StatusOK, typed)
			},
		})
		server.RegisterGeneratedCodec(server.GeneratedRouteMeta{
			PackagePath:       "github.com/webdeveloperben/tyche/server_test",
			OperationID:       "head-user",
			Method:            http.MethodHead,
			Path:              "/users/:id",
			HasGeneratedCodec: true,
		}, server.GeneratedRouteCodec{
			Parse: func(req *http.Request) (any, error) {
				return &getUserInput{ID: req.PathValue("id")}, nil
			},
			Write: func(w http.ResponseWriter, req *http.Request, out any) error {
				w.WriteHeader(http.StatusOK)
				return nil
			},
		})
		server.RegisterGeneratedCodec(server.GeneratedRouteMeta{
			PackagePath:       "github.com/webdeveloperben/tyche/server_test",
			OperationID:       "docs-user",
			Method:            http.MethodGet,
			Path:              "/users/:id",
			HasGeneratedCodec: true,
		}, server.GeneratedRouteCodec{
			Parse: func(req *http.Request) (any, error) {
				return &getUserInput{ID: req.PathValue("id")}, nil
			},
			Write: func(w http.ResponseWriter, req *http.Request, out any) error {
				typed, _ := out.(*getUserOutput)
				return server.WriteSuccess(w, http.StatusOK, typed)
			},
		})
		server.RegisterGeneratedCodec(server.GeneratedRouteMeta{
			PackagePath:       "github.com/webdeveloperben/tyche/server_test",
			OperationID:       "dup",
			Method:            http.MethodGet,
			Path:              "/a",
			HasGeneratedCodec: true,
		}, server.GeneratedRouteCodec{
			Parse: func(req *http.Request) (any, error) {
				return &getUserInput{}, nil
			},
			Write: func(w http.ResponseWriter, req *http.Request, out any) error {
				typed, _ := out.(*getUserOutput)
				return server.WriteSuccess(w, http.StatusOK, typed)
			},
		})
	})
}

func TestParseRequest_Path(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /users/{id}", func(w http.ResponseWriter, r *http.Request) {
		input, err := server.ParseRequest[getUserInput](r)
		if err != nil {
			t.Fatalf("ParseRequest failed: %v", err)
		}
		if input.ID != "123" {
			t.Fatalf("expected ID '123', got %q", input.ID)
		}
	})

	req := httptest.NewRequest(http.MethodGet, "/users/123", nil)
	mux.ServeHTTP(httptest.NewRecorder(), req)
}

func TestParseRequest_Validation(t *testing.T) {
	t.Run("min length", func(t *testing.T) {
		type input struct {
			Name string `json:"name" validate:"min=2"`
		}
		req := httptest.NewRequest(http.MethodPost, "/users", strings.NewReader(`{"name":"a"}`))
		req.Header.Set("Content-Type", "application/json")

		_, err := server.ParseRequest[input](req)
		if err == nil || !strings.Contains(err.Error(), "at least 2 characters") {
			t.Fatalf("expected min validation error, got %v", err)
		}
	})

	t.Run("unknown field", func(t *testing.T) {
		type input struct {
			Name string `json:"name"`
		}
		req := httptest.NewRequest(http.MethodPost, "/users", strings.NewReader(`{"name":"ben","extra":true}`))
		req.Header.Set("Content-Type", "application/json")

		_, err := server.ParseRequest[input](req)
		if err == nil || !strings.Contains(err.Error(), "unknown field") {
			t.Fatalf("expected unknown field error, got %v", err)
		}
	})

	t.Run("missing required body field", func(t *testing.T) {
		type input struct {
			Name string `json:"name"`
		}
		req := httptest.NewRequest(http.MethodPost, "/users", strings.NewReader(`{}`))
		req.Header.Set("Content-Type", "application/json")

		_, err := server.ParseRequest[input](req)
		var validationErr *validation.Error
		if err == nil || !errors.As(err, &validationErr) || len(validationErr.Problems) != 1 || validationErr.Problems[0].Pointer != "/name" {
			t.Fatalf("expected validation error for /name, got %v", err)
		}
	})

	t.Run("missing required query", func(t *testing.T) {
		type input struct {
			Query string `query:"q" required:"true"`
		}
		req := httptest.NewRequest(http.MethodGet, "/search", nil)

		_, err := server.ParseRequest[input](req)
		var validationErr *validation.Error
		if err == nil || !errors.As(err, &validationErr) || len(validationErr.Problems) != 1 || validationErr.Problems[0].Pointer != "/query/q" {
			t.Fatalf("expected validation error for /query/q, got %v", err)
		}
	})

	t.Run("case insensitive header", func(t *testing.T) {
		type input struct {
			Token string `header:"x-api-key"`
		}
		req := httptest.NewRequest(http.MethodGet, "/secure", nil)
		req.Header.Set("X-Api-Key", "secret")

		parsed, err := server.ParseRequest[input](req)
		if err != nil {
			t.Fatalf("expected header to parse, got %v", err)
		}
		if parsed.Token != "secret" {
			t.Fatalf("expected header value, got %q", parsed.Token)
		}
	})

	t.Run("optional nested object", func(t *testing.T) {
		type nested struct {
			Name string `json:"name"`
		}
		type input struct {
			Parent *nested `json:"parent,omitempty"`
		}

		req := httptest.NewRequest(http.MethodPost, "/nested", strings.NewReader(`{}`))
		req.Header.Set("Content-Type", "application/json")

		parsed, err := server.ParseRequest[input](req)
		if err != nil {
			t.Fatalf("expected optional nested object to be omitted cleanly, got %v", err)
		}
		if parsed.Parent != nil {
			t.Fatalf("expected nil parent, got %#v", parsed.Parent)
		}
	})

	t.Run("validate tag", func(t *testing.T) {
		type input struct {
			Name  string `json:"name" validate:"min=2,max=10"`
			Email string `json:"email" validate:"email"`
			Role  string `json:"role" validate:"oneof=admin member"`
		}

		req := httptest.NewRequest(http.MethodPost, "/users", strings.NewReader(`{"name":"ab","email":"not-an-email","role":"guest"}`))
		req.Header.Set("Content-Type", "application/json")

		_, err := server.ParseRequest[input](req)
		var validationErr *validation.Error
		if err == nil || !errors.As(err, &validationErr) || len(validationErr.Problems) != 2 {
			t.Fatalf("expected two validation problems, got %v", err)
		}
	})

	t.Run("unicode string length uses runes", func(t *testing.T) {
		type input struct {
			Name string `json:"name" validate:"len=2"`
		}

		req := httptest.NewRequest(http.MethodPost, "/users", strings.NewReader(`{"name":"👍👍"}`))
		req.Header.Set("Content-Type", "application/json")

		parsed, err := server.ParseRequest[input](req)
		if err != nil {
			t.Fatalf("expected rune-based length validation to pass, got %v", err)
		}
		if parsed.Name != "👍👍" {
			t.Fatalf("expected parsed unicode name, got %q", parsed.Name)
		}
	})

	t.Run("required fields in nested arrays", func(t *testing.T) {
		type child struct {
			Code string `json:"code"`
		}
		type input struct {
			Children []child `json:"children"`
		}

		req := httptest.NewRequest(http.MethodPost, "/users", strings.NewReader(`{"children":[{}]}`))
		req.Header.Set("Content-Type", "application/json")

		_, err := server.ParseRequest[input](req)
		var validationErr *validation.Error
		if err == nil || !errors.As(err, &validationErr) || len(validationErr.Problems) != 1 || validationErr.Problems[0].Pointer != "/children/code" {
			t.Fatalf("expected validation error for /children/code, got %v", err)
		}
	})
}

func TestRegister_Integration(t *testing.T) {
	registerTypedTestCodecs()
	srvRouter := server.NewAPI(server.NewServeMuxAdapter())
	protected := srvRouter.Group("/api/v1")

	server.Register(protected, server.Operation{
		OperationID: "get-user",
		Method:      http.MethodGet,
		Path:        "/users/:id",
		Summary:     "Get user by ID",
		Tags:        []string{"users"},
	}, func(ctx context.Context, in *getUserInput) (*getUserOutput, error) {
		return &getUserOutput{Name: "Test User", Email: "test@example.com"}, nil
	})

	server.Register(protected, server.Operation{
		OperationID: "create-user",
		Method:      http.MethodPost,
		Path:        "/users",
		Summary:     "Create a new user",
		Tags:        []string{"users"},
	}, func(ctx context.Context, in *createUserInput) (*createUserOutput, error) {
		return &createUserOutput{ID: "123", Name: in.Name, Email: in.Email}, nil
	})

	server.Register(protected, server.Operation{
		OperationID:   "create-wrapped-user",
		Method:        http.MethodPost,
		Path:          "/wrapped-users",
		Summary:       "Create a wrapped user",
		Tags:          []string{"users"},
		DefaultStatus: http.StatusCreated,
	}, func(ctx context.Context, in *createUserInput) (*wrappedCreateUserOutput, error) {
		out := &wrappedCreateUserOutput{Status: http.StatusCreated, Trace: "trace-123"}
		out.Body.ID = "created-123"
		return out, nil
	})

	t.Run("GET user", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/users/123", nil)
		w := httptest.NewRecorder()
		srvRouter.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d", w.Code)
		}

		var resp struct {
			Data getUserOutput `json:"data"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to unmarshal response: %v", err)
		}
		if resp.Data.Name != "Test User" {
			t.Fatalf("expected name 'Test User', got %q", resp.Data.Name)
		}
	})

	t.Run("POST user", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/users", strings.NewReader(`{"name":"John","email":"john@example.com"}`))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		srvRouter.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d", w.Code)
		}

		var resp struct {
			Data createUserOutput `json:"data"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to unmarshal response: %v", err)
		}
		if resp.Data.Name != "John" {
			t.Fatalf("expected name 'John', got %q", resp.Data.Name)
		}
	})

	t.Run("Wrapped response", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/wrapped-users", strings.NewReader(`{"name":"John","email":"john@example.com"}`))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		srvRouter.ServeHTTP(w, req)

		if w.Code != http.StatusCreated {
			t.Fatalf("expected status 201, got %d", w.Code)
		}
		if got := w.Header().Get("X-Trace-ID"); got != "trace-123" {
			t.Fatalf("expected X-Trace-ID header, got %q", got)
		}

		var resp struct {
			Data struct {
				ID string `json:"id"`
			} `json:"data"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to unmarshal wrapped response: %v", err)
		}
		if resp.Data.ID != "created-123" {
			t.Fatalf("expected wrapped body id, got %q", resp.Data.ID)
		}
	})

	t.Run("OpenAPI spec", func(t *testing.T) {
		spec := srvRouter.OpenAPI()
		if len(spec.Paths) != 3 {
			t.Fatalf("expected 3 paths, got %d", len(spec.Paths))
		}
		if spec.Paths["/api/v1/users/{id}"].GET == nil {
			t.Fatal("expected GET /api/v1/users/{id} to be registered")
		}
		if spec.Paths["/api/v1/users"].POST == nil {
			t.Fatal("expected POST /api/v1/users to be registered")
		}
		wrapped := spec.Paths["/api/v1/wrapped-users"].POST
		if wrapped == nil {
			t.Fatal("expected POST /api/v1/wrapped-users to be registered")
		}
		if wrapped.Responses["201"] == nil {
			t.Fatal("expected wrapped route to document 201 response")
		}
		if wrapped.Responses["201"].Headers["X-Trace-ID"] == nil {
			t.Fatal("expected wrapped route to document X-Trace-ID header")
		}
	})
}

func TestRegister_DefaultStatusDoesNotLeakAcrossRoutes(t *testing.T) {
	registerTypedTestCodecs()
	router := server.NewAPI(server.NewServeMuxAdapter())
	api := router.Group("/api")

	server.Register(api, server.Operation{
		OperationID:   "create-a",
		Method:        http.MethodPost,
		Path:          "/a",
		DefaultStatus: http.StatusCreated,
	}, func(ctx context.Context, in *createUserInput) (*sharedStatusOutput, error) {
		out := &sharedStatusOutput{}
		out.Body.ID = "a"
		return out, nil
	})

	server.Register(api, server.Operation{
		OperationID: "create-b",
		Method:      http.MethodPost,
		Path:        "/b",
	}, func(ctx context.Context, in *createUserInput) (*sharedStatusOutput, error) {
		out := &sharedStatusOutput{}
		out.Body.ID = "b"
		return out, nil
	})

	reqA := httptest.NewRequest(http.MethodPost, "/api/a", strings.NewReader(`{"name":"John","email":"john@example.com"}`))
	reqA.Header.Set("Content-Type", "application/json")
	wA := httptest.NewRecorder()
	router.ServeHTTP(wA, reqA)
	if wA.Code != http.StatusCreated {
		t.Fatalf("expected first route status 201, got %d", wA.Code)
	}

	reqB := httptest.NewRequest(http.MethodPost, "/api/b", strings.NewReader(`{"name":"John","email":"john@example.com"}`))
	reqB.Header.Set("Content-Type", "application/json")
	wB := httptest.NewRecorder()
	router.ServeHTTP(wB, reqB)
	if wB.Code != http.StatusOK {
		t.Fatalf("expected second route status 200, got %d", wB.Code)
	}
}

func TestRegister_DuplicateOperationIDPanics(t *testing.T) {
	registerTypedTestCodecs()
	router := server.NewAPI(server.NewServeMuxAdapter())
	api := router.Group("/api")

	server.Register(api, server.Operation{
		OperationID: "dup",
		Method:      http.MethodGet,
		Path:        "/a",
	}, func(ctx context.Context, in *getUserInput) (*getUserOutput, error) {
		return &getUserOutput{}, nil
	})

	defer func() {
		if recover() == nil {
			t.Fatal("expected duplicate operation ID to panic")
		}
	}()

	server.Register(api, server.Operation{
		OperationID: "dup",
		Method:      http.MethodGet,
		Path:        "/b",
	}, func(ctx context.Context, in *getUserInput) (*getUserOutput, error) {
		return &getUserOutput{}, nil
	})
}

func TestRegister_RequireGeneratedCodecPanicsWhenMissing(t *testing.T) {
	// Register always requires a generated codec and fails explicitly when one
	// is missing (no reflection fallback).
	router := server.NewAPI(server.NewServeMuxAdapter())
	api := router.Group("/api")

	defer func() {
		if recover() == nil {
			t.Fatal("expected missing generated codec to panic")
		}
	}()

	server.Register(api, server.Operation{
		OperationID: "missing-generated",
		Method:      http.MethodGet,
		Path:        "/users/:id",
	}, func(ctx context.Context, in *getUserInput) (*getUserOutput, error) {
		return &getUserOutput{}, nil
	})
}

func TestRegister_ErrorResponseMatchesOpenAPIContract(t *testing.T) {
	registerTypedTestCodecs()
	router := server.NewAPI(server.NewServeMuxAdapter())
	api := router.Group("/api")

	server.Register(api, server.Operation{
		OperationID: "error-user",
		Method:      http.MethodGet,
		Path:        "/users/:id",
	}, func(ctx context.Context, in *getUserInput) (*getUserOutput, error) {
		return nil, server.NewHTTPError(http.StatusBadRequest, "bad user")
	})

	req := httptest.NewRequest(http.MethodGet, "/api/users/123", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if got := w.Header().Get("Content-Type"); got != "application/problem+json" {
		t.Fatalf("expected problem+json error response, got %q", got)
	}
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", w.Code)
	}

	var payload struct {
		Detail string `json:"detail"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to decode error response: %v", err)
	}
	if payload.Detail != "bad user" {
		t.Fatalf("expected error detail, got %#v", payload)
	}

	spec := router.OpenAPI()
	defaultResp := spec.Paths["/api/users/{id}"].GET.Responses["default"]
	if defaultResp == nil || defaultResp.Content["application/problem+json"] == nil {
		t.Fatal("expected OpenAPI default error response")
	}
	if defaultResp.Content["application/problem+json"].Schema.Properties["title"] == nil {
		t.Fatal("expected OpenAPI error schema to include title field")
	}
}

func TestParseRequest_EnforcesRouterBodyLimit(t *testing.T) {
	router := server.NewAPI(server.NewServeMuxAdapter(), server.APIConfig{MaxRequestBodyBytes: 8})
	api := router.Group("/api")

	type input struct {
		Name string `json:"name"`
	}
	type output struct {
		Body struct {
			OK bool `json:"ok"`
		} `body:"true"`
	}

	server.RegisterGeneratedCodec(server.GeneratedRouteMeta{
		PackagePath:       "github.com/webdeveloperben/tyche/server_test",
		OperationID:       "limited-user",
		Method:            http.MethodPost,
		Path:              "/users",
		HasGeneratedCodec: true,
	}, server.GeneratedRouteCodec{
		Parse: func(req *http.Request) (any, error) {
			var in input
			if err := server.DecodeRequestJSONBodyFast(req, &in); err != nil {
				return nil, err
			}
			return &in, nil
		},
		Write: func(w http.ResponseWriter, req *http.Request, out any) error {
			typed, _ := out.(*output)
			return server.WriteSuccess(w, http.StatusOK, typed.Body)
		},
	})

	server.Register(api, server.Operation{
		OperationID: "limited-user",
		Method:      http.MethodPost,
		Path:        "/users",
	}, func(ctx context.Context, in *input) (*output, error) {
		out := &output{}
		out.Body.OK = true
		return out, nil
	})

	req := httptest.NewRequest(http.MethodPost, "/api/users", strings.NewReader(`{"name":"toolong"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", w.Code)
	}

	var payload struct {
		Detail string `json:"detail"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to decode body limit error: %v", err)
	}
	if !strings.Contains(payload.Detail, "request body too large") {
		t.Fatalf("expected body limit error, got %#v", payload)
	}
}

func TestRegister_HeadResponseHasNoBody(t *testing.T) {
	registerTypedTestCodecs()
	router := server.NewAPI(server.NewServeMuxAdapter())
	api := router.Group("/api")

	server.Register(api, server.Operation{
		OperationID: "head-user",
		Method:      http.MethodHead,
		Path:        "/users/:id",
	}, func(ctx context.Context, in *getUserInput) (*getUserOutput, error) {
		return &getUserOutput{Name: "Test User", Email: "test@example.com"}, nil
	})

	req := httptest.NewRequest(http.MethodHead, "/api/users/123", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}
	if w.Body.Len() != 0 {
		t.Fatalf("expected no response body for HEAD, got %q", w.Body.String())
	}
}

func TestRegister_NoBodyStatusDocsAndRuntime(t *testing.T) {
	type noBodyOutput struct {
		Status int      `status:"204"`
		Body   struct{} `body:"true"`
	}

	router := server.NewAPI(server.NewServeMuxAdapter())
	api := router.Group("/api")

	server.RegisterGeneratedCodec(server.GeneratedRouteMeta{
		PackagePath:       "github.com/webdeveloperben/tyche/server_test",
		OperationID:       "delete-user",
		Method:            http.MethodDelete,
		Path:              "/users/:id",
		HasGeneratedCodec: true,
	}, server.GeneratedRouteCodec{
		Parse: func(req *http.Request) (any, error) {
			return &getUserInput{ID: req.PathValue("id")}, nil
		},
		Write: func(w http.ResponseWriter, req *http.Request, out any) error {
			typed, _ := out.(*noBodyOutput)
			w.WriteHeader(typed.Status)
			return nil
		},
	})

	server.Register(api, server.Operation{
		OperationID: "delete-user",
		Method:      http.MethodDelete,
		Path:        "/users/:id",
	}, func(ctx context.Context, in *getUserInput) (*noBodyOutput, error) {
		return &noBodyOutput{Status: http.StatusNoContent}, nil
	})

	req := httptest.NewRequest(http.MethodDelete, "/api/users/123", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected status 204, got %d", w.Code)
	}
	if w.Body.Len() != 0 {
		t.Fatalf("expected empty body for 204, got %q", w.Body.String())
	}

	resp := router.OpenAPI().Paths["/api/users/{id}"].DELETE.Responses["204"]
	if resp == nil {
		t.Fatal("expected 204 response to be documented")
	}
	if resp.Content != nil {
		t.Fatal("expected 204 response content to be omitted")
	}
}

func TestRouter_MountOpenAPI(t *testing.T) {
	registerTypedTestCodecs()
	router := server.NewAPI(server.NewServeMuxAdapter())
	api := router.Group("/api")

	server.Register(api, server.Operation{
		OperationID: "docs-user",
		Method:      http.MethodGet,
		Path:        "/users/:id",
	}, func(ctx context.Context, in *getUserInput) (*getUserOutput, error) {
		return &getUserOutput{Name: "Test User"}, nil
	})

	if err := router.MountOpenAPI("/openapi.json"); err != nil {
		t.Fatalf("MountOpenAPI failed: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/openapi.json", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}
	if got := w.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("expected application/json content type, got %q", got)
	}
	if !strings.Contains(w.Body.String(), `"/api/users/{id}"`) {
		t.Fatalf("expected OpenAPI body to include registered path, got %s", w.Body.String())
	}

	headReq := httptest.NewRequest(http.MethodHead, "/openapi.json", nil)
	headW := httptest.NewRecorder()
	router.ServeHTTP(headW, headReq)
	if headW.Code != http.StatusOK {
		t.Fatalf("expected HEAD status 200, got %d", headW.Code)
	}
	if headW.Body.Len() != 0 {
		t.Fatalf("expected HEAD body to be empty, got %q", headW.Body.String())
	}
}

func TestAPIDocsMount_Scalar(t *testing.T) {
	router := server.NewAPI(server.NewServeMuxAdapter())
	if err := apidocs.Mount(router, apidocs.Config{
		Title:    "Homebase API",
		SpecPath: "/openapi.json",
		UIs: []apidocs.UIMount{
			{Path: "/docs", Renderer: apidocs.Scalar()},
		},
	}); err != nil {
		t.Fatalf("Mount failed: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/docs", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}
	if got := w.Header().Get("Content-Type"); got != "text/html; charset=utf-8" {
		t.Fatalf("expected html content type, got %q", got)
	}
	body := w.Body.String()
	if !strings.Contains(body, "Scalar.createApiReference") {
		t.Fatalf("expected Scalar bootstrap script, got %s", body)
	}
	if !strings.Contains(body, "/openapi.json") {
		t.Fatalf("expected spec url in docs page, got %s", body)
	}

	headReq := httptest.NewRequest(http.MethodHead, "/docs", nil)
	headW := httptest.NewRecorder()
	router.ServeHTTP(headW, headReq)
	if headW.Code != http.StatusOK {
		t.Fatalf("expected HEAD status 200, got %d", headW.Code)
	}
	if headW.Body.Len() != 0 {
		t.Fatalf("expected empty HEAD body, got %q", headW.Body.String())
	}
}

func TestNewRouter_UsesOpenAPIConfig(t *testing.T) {
	router := server.NewAPI(server.NewServeMuxAdapter(), server.APIConfig{
		OpenAPI: server.OpenAPIInfo{
			Title:       "Homebase API",
			Description: "Internal API",
			Version:     "2026.04",
		},
	})

	doc := router.OpenAPI()
	if doc.Info.Title != "Homebase API" {
		t.Fatalf("expected configured title, got %q", doc.Info.Title)
	}
	if doc.Info.Description != "Internal API" {
		t.Fatalf("expected configured description, got %q", doc.Info.Description)
	}
	if doc.Info.Version != "2026.04" {
		t.Fatalf("expected configured version, got %q", doc.Info.Version)
	}
}
