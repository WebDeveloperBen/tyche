package server_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/webdeveloperben/tyche/pagination"
	"github.com/webdeveloperben/tyche/server"
)

var generatedPaginationCodecOnce sync.Once

func registerGeneratedPaginationCodec() {
	generatedPaginationCodecOnce.Do(func() {
		server.RegisterGeneratedCodec(server.GeneratedRouteMeta{
			OperationID:       "generated-pagination-wrapper",
			Method:            http.MethodGet,
			Path:              "/generated-pagination-wrapper",
			HasGeneratedCodec: true,
		}, server.GeneratedRouteCodec{
			Parse: func(*http.Request) (any, error) {
				return &benchmarkPaginationInput{}, nil
			},
			Write: func(w http.ResponseWriter, _ *http.Request, value any) error {
				return server.WriteTypedResponse(w, value.(*benchmarkPaginationOutput))
			},
		})
	})
}

func TestGeneratedPaginationUsesConfiguredPolicy(t *testing.T) {
	registerGeneratedPaginationCodec()

	api := server.NewAPI(server.NewServeMuxAdapter())
	var got pagination.Params
	if err := server.RegisterE(api, server.Operation{
		OperationID: "generated-pagination-wrapper",
		Method:      http.MethodGet,
		Path:        "/generated-pagination-wrapper",
	}, func(_ context.Context, input *benchmarkPaginationInput) (*benchmarkPaginationOutput, error) {
		got = input.Params
		return &benchmarkPaginationOutput{}, nil
	}, server.WithPaginationConfig(pagination.Config{DefaultLimit: 5, MaxLimit: 10})); err != nil {
		t.Fatalf("RegisterE: %v", err)
	}

	t.Run("default", func(t *testing.T) {
		got = pagination.Params{}
		req := httptest.NewRequest(http.MethodGet, "/generated-pagination-wrapper", nil)
		rec := httptest.NewRecorder()
		api.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
		}
		if got.Limit != 5 || got.Cursor != "" {
			t.Fatalf("params = %#v, want default limit 5", got)
		}
	})

	t.Run("configured", func(t *testing.T) {
		got = pagination.Params{}
		req := httptest.NewRequest(http.MethodGet, "/generated-pagination-wrapper?limit=7&cursor=next", nil)
		rec := httptest.NewRecorder()
		api.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
		}
		if got.Limit != 7 || got.Cursor != "next" {
			t.Fatalf("params = %#v, want limit 7 and cursor next", got)
		}
	})

	t.Run("limit enforcement", func(t *testing.T) {
		got = pagination.Params{}
		req := httptest.NewRequest(http.MethodGet, "/generated-pagination-wrapper?limit=11", nil)
		rec := httptest.NewRecorder()
		api.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, body = %s; want 400", rec.Code, rec.Body.String())
		}
		if got != (pagination.Params{}) {
			t.Fatalf("handler ran with params = %#v", got)
		}
	})
}
