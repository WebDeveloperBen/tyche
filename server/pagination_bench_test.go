package server_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/webdeveloperben/tyche/pagination"
	"github.com/webdeveloperben/tyche/server"
)

type benchmarkPaginationInput struct {
	pagination.Params
}

type benchmarkNoPaginationInput struct {
	Query string `query:"q"`
}

type benchmarkPaginationOutput struct{}

func benchmarkPaginationRouter[I any](path string) *server.API {
	router := server.NewAPI(server.NewServeMuxAdapter())
	server.Register(router, server.Operation{Method: http.MethodGet, Path: path}, func(context.Context, *I) (*benchmarkPaginationOutput, error) {
		return &benchmarkPaginationOutput{}, nil
	})
	return router
}

func BenchmarkTypedRoute_Pagination(b *testing.B) {
	router := benchmarkPaginationRouter[benchmarkPaginationInput]("/benchmark-pagination")
	req := httptest.NewRequest(http.MethodGet, "/benchmark-pagination?limit=25&cursor=opaque", nil)
	w := httptest.NewRecorder()

	b.ReportAllocs()
	for b.Loop() {
		router.ServeHTTP(w, req)
	}
}

func BenchmarkTypedRoute_NoPagination(b *testing.B) {
	router := benchmarkPaginationRouter[benchmarkNoPaginationInput]("/benchmark-no-pagination")
	req := httptest.NewRequest(http.MethodGet, "/benchmark-no-pagination?q=value", nil)
	w := httptest.NewRecorder()

	b.ReportAllocs()
	for b.Loop() {
		router.ServeHTTP(w, req)
	}
}
