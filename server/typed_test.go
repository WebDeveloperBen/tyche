package server_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"mime/multipart"
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
	Trace string `header:"X-Trace-ID" doc:"Trace identifier"`
	Body  struct {
		ID string `json:"id"`
	} `body:"true"`
	Status int `status:"201"`
}

type sharedStatusOutput struct {
	Body struct {
		ID string `json:"id"`
	} `body:"true"`
}

type uploadInput struct {
	Avatar *multipart.FileHeader   `file:"avatar"`
	Title  string                  `form:"title" validate:"min=3"`
	Tags   []string                `form:"tags,omitempty"`
	Docs   []*multipart.FileHeader `files:"docs,omitempty"`
	Count  int                     `form:"count"`
}

type uploadOutput struct {
	Body struct {
		Title      string `json:"title"`
		AvatarName string `json:"avatarName"`
		TagCount   int    `json:"tagCount"`
		Count      int    `json:"count"`
		DocCount   int    `json:"docCount"`
	} `body:"true"`
}

var typedTestCodecsOnce sync.Once

// These guard the two codecs registered inline inside individual tests, so
// `go test -count=2` (which reruns tests in one process) doesn't re-register
// and trip RegisterGeneratedCodec's duplicate-identity panic.
var (
	limitedUserCodecOnce sync.Once
	deleteUserCodecOnce  sync.Once
)

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

func TestParseRequest_MultipartFormAndFiles(t *testing.T) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("title", "avatar upload"); err != nil {
		t.Fatal(err)
	}
	if err := writer.WriteField("tags", "profile"); err != nil {
		t.Fatal(err)
	}
	if err := writer.WriteField("tags", "public"); err != nil {
		t.Fatal(err)
	}
	if err := writer.WriteField("count", "2"); err != nil {
		t.Fatal(err)
	}
	avatar, err := writer.CreateFormFile("avatar", "me.png")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := avatar.Write([]byte("avatar")); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"a.txt", "b.txt"} {
		doc, err := writer.CreateFormFile("docs", name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := doc.Write([]byte(name)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/upload", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	in, err := server.ParseRequest[uploadInput](req)
	if err != nil {
		t.Fatalf("ParseRequest failed: %v", err)
	}
	if in.Title != "avatar upload" || in.Count != 2 {
		t.Fatalf("form fields parsed incorrectly: %#v", in)
	}
	if len(in.Tags) != 2 || in.Tags[0] != "profile" || in.Tags[1] != "public" {
		t.Fatalf("form slice parsed incorrectly: %#v", in.Tags)
	}
	if in.Avatar == nil || in.Avatar.Filename != "me.png" {
		t.Fatalf("avatar parsed incorrectly: %#v", in.Avatar)
	}
	if len(in.Docs) != 2 || in.Docs[0].Filename != "a.txt" || in.Docs[1].Filename != "b.txt" {
		t.Fatalf("docs parsed incorrectly: %#v", in.Docs)
	}
}

func TestParseRequest_MultipartRequiredFile(t *testing.T) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("title", "avatar upload"); err != nil {
		t.Fatal(err)
	}
	if err := writer.WriteField("count", "1"); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/upload", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	_, err := server.ParseRequest[uploadInput](req)
	var validationErr *validation.Error
	if err == nil || !errors.As(err, &validationErr) || len(validationErr.Problems) != 1 || validationErr.Problems[0].Pointer != "/file/avatar" {
		t.Fatalf("expected required /file/avatar validation error, got %v", err)
	}
}

func TestRegister_MultipartFormRouteAndOpenAPI(t *testing.T) {
	router := server.NewAPI(server.NewServeMuxAdapter())
	api := router.Group("/api")

	server.Register(api, server.Operation{
		OperationID: "upload-avatar",
		Method:      http.MethodPost,
		Path:        "/uploads",
	}, func(ctx context.Context, in *uploadInput) (*uploadOutput, error) {
		out := &uploadOutput{}
		out.Body.Title = in.Title
		out.Body.TagCount = len(in.Tags)
		out.Body.Count = in.Count
		out.Body.AvatarName = in.Avatar.Filename
		out.Body.DocCount = len(in.Docs)
		return out, nil
	})

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("title", "avatar upload"); err != nil {
		t.Fatal(err)
	}
	if err := writer.WriteField("tags", "profile"); err != nil {
		t.Fatal(err)
	}
	if err := writer.WriteField("count", "3"); err != nil {
		t.Fatal(err)
	}
	avatar, err := writer.CreateFormFile("avatar", "me.png")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := avatar.Write([]byte("avatar")); err != nil {
		t.Fatal(err)
	}
	doc, err := writer.CreateFormFile("docs", "notes.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := doc.Write([]byte("notes")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/uploads", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Data struct {
			Title      string `json:"title"`
			AvatarName string `json:"avatarName"`
			TagCount   int    `json:"tagCount"`
			Count      int    `json:"count"`
			DocCount   int    `json:"docCount"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Data.Title != "avatar upload" || resp.Data.TagCount != 1 || resp.Data.Count != 3 || resp.Data.AvatarName != "me.png" || resp.Data.DocCount != 1 {
		t.Fatalf("unexpected response: %+v", resp.Data)
	}

	op := router.OpenAPI().Paths["/api/uploads"].POST
	if op == nil || op.RequestBody == nil {
		t.Fatal("expected multipart route request body in OpenAPI")
	}
	mt := op.RequestBody.Content["multipart/form-data"]
	if mt == nil || mt.Schema == nil {
		t.Fatalf("expected multipart/form-data schema, got %#v", op.RequestBody.Content)
	}
	if mt.Schema.Properties["avatar"].Format != "binary" {
		t.Fatalf("expected avatar to be binary, got %#v", mt.Schema.Properties["avatar"])
	}
	docs := mt.Schema.Properties["docs"]
	if docs == nil || docs.Type != "array" || docs.Items == nil || docs.Items.Format != "binary" {
		t.Fatalf("expected docs to be binary array, got %#v", docs)
	}
	required := strings.Join(mt.Schema.Required, ",")
	for _, want := range []string{"title", "count", "avatar"} {
		if !strings.Contains(required, want) {
			t.Fatalf("expected required multipart field %q in %v", want, mt.Schema.Required)
		}
	}
}

func TestRegister_MultipartRejectsInvalidFileTypes(t *testing.T) {
	type badFileInput struct {
		File string `file:"avatar"`
	}
	type emptyOutput struct{}

	router := server.NewAPI(server.NewServeMuxAdapter())
	err := server.RegisterE(router, server.Operation{
		OperationID: "bad-upload",
		Method:      http.MethodPost,
		Path:        "/bad-upload",
	}, func(ctx context.Context, in *badFileInput) (*emptyOutput, error) {
		return &emptyOutput{}, nil
	})
	if err == nil || !strings.Contains(err.Error(), "file fields must use *multipart.FileHeader") {
		t.Fatalf("expected invalid file field error, got %v", err)
	}
}

func TestRegister_MultipartRejectsJSONBodyMix(t *testing.T) {
	type mixedInput struct {
		Title string                `form:"title"`
		Body  struct{ Name string } `body:"true"`
	}
	type emptyOutput struct{}

	router := server.NewAPI(server.NewServeMuxAdapter())
	err := server.RegisterE(router, server.Operation{
		OperationID: "mixed-upload",
		Method:      http.MethodPost,
		Path:        "/mixed-upload",
	}, func(ctx context.Context, in *mixedInput) (*emptyOutput, error) {
		return &emptyOutput{}, nil
	})
	if err == nil || !strings.Contains(err.Error(), "multipart form/file fields cannot be combined with JSON body fields") {
		t.Fatalf("expected mixed body mode error, got %v", err)
	}
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

func TestRegister_ReflectionFallbackWhenNoCodec(t *testing.T) {
	// With no generated codec, Register falls back to the reflection binder so
	// the route still works during development — binding the input and writing
	// the {"data": …} envelope just like a generated codec would.
	router := server.NewAPI(server.NewServeMuxAdapter())
	api := router.Group("/api")

	server.Register(api, server.Operation{
		OperationID: "reflection-user",
		Method:      http.MethodGet,
		Path:        "/reflect/users/:id",
	}, func(_ context.Context, in *getUserInput) (*getUserOutput, error) {
		return &getUserOutput{Name: "user-" + in.ID, Email: "u@example.com"}, nil
	})

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/reflect/users/42", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("reflection fallback: status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Data getUserOutput `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("reflection fallback: decode envelope: %v (body %s)", err, rec.Body.String())
	}
	if resp.Data.Name != "user-42" {
		t.Fatalf("reflection fallback: data.name = %q, want %q", resp.Data.Name, "user-42")
	}
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

func TestRegister_ContentNegotiation(t *testing.T) {
	type input struct {
		Name string `json:"name"`
	}
	type output struct {
		Body struct {
			OK bool `json:"ok"`
		} `body:"true"`
	}

	router := server.NewAPI(server.NewServeMuxAdapter())
	api := router.Group("/api")

	server.Register(api, server.Operation{
		OperationID: "negotiation-user",
		Method:      http.MethodPost,
		Path:        "/negotiation",
	}, func(ctx context.Context, in *input) (*output, error) {
		out := &output{}
		out.Body.OK = in.Name != ""
		return out, nil
	})

	t.Run("unsupported request content type is 415", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/negotiation", strings.NewReader(`{"name":"Ada"}`))
		req.Header.Set("Content-Type", "text/plain")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnsupportedMediaType {
			t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
		}
		if got := rec.Header().Get("Content-Type"); got != "application/problem+json" {
			t.Fatalf("content-type = %q", got)
		}
	})

	t.Run("unacceptable response content type is 406", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/negotiation", strings.NewReader(`{"name":"Ada"}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/xml")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusNotAcceptable {
			t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
		}
		if got := rec.Header().Get("Content-Type"); got != "application/problem+json" {
			t.Fatalf("content-type = %q", got)
		}
	})

	t.Run("specific q=0 overrides wildcard accept", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/negotiation", strings.NewReader(`{"name":"Ada"}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json;q=0, */*;q=1")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusNotAcceptable {
			t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("wildcard accept allows json", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/negotiation", strings.NewReader(`{"name":"Ada"}`))
		req.Header.Set("Content-Type", "application/json; charset=utf-8")
		req.Header.Set("Accept", "application/*")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
		}
	})
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

	limitedUserCodecOnce.Do(func() {
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
		Body   struct{} `body:"true"`
		Status int      `status:"204"`
	}

	router := server.NewAPI(server.NewServeMuxAdapter())
	api := router.Group("/api")

	deleteUserCodecOnce.Do(func() {
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
