package server_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/webdeveloperben/tyche/server"
)

type setupInput struct {
	ID string `path:"id"`
}

type setupOutput struct {
	Name string `json:"name"`
}

func registerSetupTestCodecs() {
	server.RegisterGeneratedCodec(server.GeneratedRouteMeta{
		PackagePath:       "github.com/webdeveloperben/tyche/server",
		OperationID:       "dup",
		Method:            http.MethodGet,
		Path:              "/users/:id",
		HasGeneratedCodec: true,
	}, server.GeneratedRouteCodec{
		Parse: func(req *http.Request) (any, error) {
			return &setupInput{ID: req.PathValue("id")}, nil
		},
		Write: func(w http.ResponseWriter, req *http.Request, out any) error {
			return server.WriteJSON(w, http.StatusOK, out)
		},
	})
}

func TestHandleE_ReturnsErrorForInvalidPath(t *testing.T) {
	router := server.NewRouter()
	if err := router.HandleE(http.MethodGet, "users", func(w http.ResponseWriter, r *http.Request) error { return nil }); err == nil {
		t.Fatal("expected invalid path error")
	}
}

func TestRegisterE_ReturnsErrorForDuplicateOperationID(t *testing.T) {
	registerSetupTestCodecs()
	router := server.NewRouter()
	api := router.Group("/api")

	err := server.RegisterE(api, server.Operation{
		OperationID: "dup",
		Method:      http.MethodGet,
		Path:        "/users/:id",
	}, func(ctx context.Context, in *setupInput) (*setupOutput, error) {
		return &setupOutput{}, nil
	})
	if err != nil {
		t.Fatalf("expected first registration to succeed, got %v", err)
	}

	err = server.RegisterE(api, server.Operation{
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
