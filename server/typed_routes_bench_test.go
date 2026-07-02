package server_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/webdeveloperben/tyche/server"
	typedroutes "github.com/webdeveloperben/tyche/servergen/testdata/samplepkg"
)

func benchmarkTypedRouter() *server.API {
	router := server.NewAPI(server.NewServeMuxAdapter())
	typedroutes.RegisterTypedRoutes(router.Group(""))
	return router
}

func BenchmarkTypedRoute_Static(b *testing.B) {
	router := benchmarkTypedRouter()
	req := httptest.NewRequest(http.MethodGet, typedroutes.StaticPath, nil)
	w := httptest.NewRecorder()

	b.ReportAllocs()
	for b.Loop() {
		router.ServeHTTP(w, req)
	}
}

func BenchmarkTypedRoute_Param(b *testing.B) {
	router := benchmarkTypedRouter()
	req := httptest.NewRequest(http.MethodGet, typedroutes.ParamURL, nil)
	w := httptest.NewRecorder()

	b.ReportAllocs()
	for b.Loop() {
		router.ServeHTTP(w, req)
	}
}

func BenchmarkTypedRoute_QueryHeader(b *testing.B) {
	router := benchmarkTypedRouter()
	req := httptest.NewRequest(http.MethodGet, typedroutes.QueryHeaderURL, nil)
	req.Header.Set("X-Api-Key", "secret")
	w := httptest.NewRecorder()

	b.ReportAllocs()
	for b.Loop() {
		router.ServeHTTP(w, req)
	}
}

func BenchmarkTypedRoute_Body(b *testing.B) {
	router := benchmarkTypedRouter()
	req := httptest.NewRequest(http.MethodPost, typedroutes.BodyPath, nil)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	b.ReportAllocs()
	for b.Loop() {
		req.Body = ioNopCloserBytes(typedroutes.BodyJSON)
		router.ServeHTTP(w, req)
	}
}

func BenchmarkTypedRoute_ValidatedBody(b *testing.B) {
	router := benchmarkTypedRouter()
	req := httptest.NewRequest(http.MethodPost, typedroutes.ValidatedBodyPath, nil)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	b.ReportAllocs()
	for b.Loop() {
		req.Body = ioNopCloserBytes(typedroutes.ValidatedBodyJSON)
		router.ServeHTTP(w, req)
	}
}

func BenchmarkTypedRoute_NestedBody(b *testing.B) {
	router := benchmarkTypedRouter()
	req := httptest.NewRequest(http.MethodPost, typedroutes.NestedBodyPath, nil)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	b.ReportAllocs()
	for b.Loop() {
		req.Body = ioNopCloserBytes(typedroutes.NestedBodyJSON)
		router.ServeHTTP(w, req)
	}
}

func ioNopCloserBytes(body []byte) *readCloser {
	return &readCloser{Reader: bytes.NewReader(body)}
}

type readCloser struct {
	*bytes.Reader
}

func (r *readCloser) Close() error {
	return nil
}
