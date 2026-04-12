package server

import (
	"context"
	"net/http"
	"testing"
)

type setupInput struct {
	ID string `path:"id"`
}

type setupOutput struct {
	Name string `json:"name"`
}

func registerSetupTestCodecs() {
	RegisterGeneratedCodec(GeneratedRouteMeta{
		PackagePath:       "github.com/webdeveloperben/tyche/server",
		OperationID:       "dup",
		Method:            http.MethodGet,
		Path:              "/users/:id",
		HasGeneratedCodec: true,
	}, GeneratedRouteCodec{
		Parse: func(req *http.Request) (any, error) {
			return &setupInput{ID: req.PathValue("id")}, nil
		},
		Write: func(w http.ResponseWriter, req *http.Request, out any) error {
			return WriteJSON(w, http.StatusOK, out)
		},
	})
}

func TestHandleE_ReturnsErrorForInvalidPath(t *testing.T) {
	router := NewRouter()
	if err := router.HandleE(http.MethodGet, "users", func(w http.ResponseWriter, r *http.Request) error { return nil }); err == nil {
		t.Fatal("expected invalid path error")
	}
}

func TestRegisterE_ReturnsErrorForDuplicateOperationID(t *testing.T) {
	registerSetupTestCodecs()
	router := NewRouter()
	api := router.Group("/api")

	err := RegisterE(api, Operation{
		OperationID: "dup",
		Method:      http.MethodGet,
		Path:        "/users/:id",
	}, func(ctx context.Context, in *setupInput) (*setupOutput, error) {
		return &setupOutput{}, nil
	})
	if err != nil {
		t.Fatalf("expected first registration to succeed, got %v", err)
	}

	err = RegisterE(api, Operation{
		OperationID: "dup",
		Method:      http.MethodGet,
		Path:        "/users/:id",
	}, func(ctx context.Context, in *setupInput) (*setupOutput, error) {
		return &setupOutput{}, nil
	})
	if err == nil {
		t.Fatal("expected duplicate operation ID error")
	}
}
