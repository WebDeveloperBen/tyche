package server_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/webdeveloperben/tyche/server"
)

// These tests pin the no-codegen reflection fallback (ParseRequest +
// writeReflectionResponse) to the same response contract a generated codec
// produces: the {"data": …} envelope, status from a Status field, response
// headers, no-body responses, and request-body binding + validation. The final
// case asserts the reflection output is byte-identical to a codec that mirrors
// servergen's emission, guarding against the two implementations drifting.

type confOutBody struct {
	Message string `json:"message"`
	Count   int    `json:"count"`
}

type confInput struct {
	ID string `path:"id"`
}

type confOutput struct {
	ETag   string `header:"ETag"`
	Body   confOutBody
	Status int
}

func confHandler(_ context.Context, in *confInput) (*confOutput, error) {
	return &confOutput{
		Body:   confOutBody{Message: "hello:" + in.ID, Count: 3},
		Status: http.StatusCreated,
		ETag:   "v1",
	}, nil
}

type confEmptyInput struct{}

type confEmptyBody struct{}

type confNoBodyOutput struct {
	Body   confEmptyBody
	Status int
}

func confNoBodyHandler(_ context.Context, _ *confEmptyInput) (*confNoBodyOutput, error) {
	return &confNoBodyOutput{Status: http.StatusNoContent}, nil
}

type confCreateInput struct {
	ID   string `path:"id"`
	Body struct {
		Name string `json:"name" validate:"min=2,max=10"`
	}
}

type confCreateOutput struct {
	Body confOutBody
}

func confCreateHandler(_ context.Context, in *confCreateInput) (*confCreateOutput, error) {
	return &confCreateOutput{Body: confOutBody{Message: "created:" + in.ID + ":" + in.Body.Name, Count: len(in.Body.Name)}}, nil
}

func TestReflectionFallback_Conformance(t *testing.T) {
	t.Run("body envelope, status field, and response header", func(t *testing.T) {
		api := server.NewAPI(server.NewServeMuxAdapter())
		server.Register(api.Group("/api"), server.Operation{
			OperationID: "conf-body", Method: http.MethodGet, Path: "/things/:id",
		}, confHandler)

		rec := httptest.NewRecorder()
		api.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/things/7", nil))

		if rec.Code != http.StatusCreated {
			t.Fatalf("status = %d, want 201", rec.Code)
		}
		if got := rec.Header().Get("ETag"); got != "v1" {
			t.Fatalf("ETag = %q, want %q", got, "v1")
		}
		if got := rec.Body.String(); got != "{\"data\":{\"message\":\"hello:7\",\"count\":3}}\n" {
			t.Fatalf("body = %q, want the {\"data\":…} envelope", got)
		}
	})

	t.Run("no-body output writes only a status", func(t *testing.T) {
		api := server.NewAPI(server.NewServeMuxAdapter())
		server.Register(api.Group("/api"), server.Operation{
			OperationID: "conf-nobody", Method: http.MethodGet, Path: "/ping",
		}, confNoBodyHandler)

		rec := httptest.NewRecorder()
		api.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/ping", nil))

		if rec.Code != http.StatusNoContent {
			t.Fatalf("status = %d, want 204", rec.Code)
		}
		if rec.Body.Len() != 0 {
			t.Fatalf("expected empty body, got %q", rec.Body.String())
		}
	})

	t.Run("request body binds and returns the envelope", func(t *testing.T) {
		api := server.NewAPI(server.NewServeMuxAdapter())
		server.Register(api.Group("/api"), server.Operation{
			OperationID: "conf-create", Method: http.MethodPost, Path: "/things/:id",
		}, confCreateHandler)

		req := httptest.NewRequest(http.MethodPost, "/api/things/9", strings.NewReader(`{"name":"ada"}`))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		api.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
		}
		if got := rec.Body.String(); got != "{\"data\":{\"message\":\"created:9:ada\",\"count\":3}}\n" {
			t.Fatalf("body = %q", got)
		}
	})

	t.Run("invalid request body is rejected via validation", func(t *testing.T) {
		api := server.NewAPI(server.NewServeMuxAdapter())
		server.Register(api.Group("/api"), server.Operation{
			OperationID: "conf-create-invalid", Method: http.MethodPost, Path: "/things/:id",
		}, confCreateHandler)

		req := httptest.NewRequest(http.MethodPost, "/api/things/9", strings.NewReader(`{"name":"a"}`)) // too short
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		api.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400 (body %s)", rec.Code, rec.Body.String())
		}
		if ct := rec.Header().Get("Content-Type"); ct != "application/problem+json" {
			t.Fatalf("content-type = %q, want problem+json", ct)
		}
	})

	t.Run("byte-identical to a codec that mirrors servergen emission", func(t *testing.T) {
		// The reflection path (WriteSuccess → json encoder) must produce exactly
		// the bytes servergen emits (hand-appended {"data":{…}} + status +
		// headers). Register the same types once with such a codec and once
		// without (reflection), then diff the full responses.
		server.RegisterGeneratedCodec(server.GeneratedRouteMeta{
			OperationID: "conf-codec", Method: http.MethodGet, Path: "/thing/:id", HasGeneratedCodec: true,
		}, server.GeneratedRouteCodec{
			Parse: func(req *http.Request) (any, error) {
				return &confInput{ID: req.PathValue("id")}, nil
			},
			Write: func(w http.ResponseWriter, _ *http.Request, value any) error {
				out := value.(*confOutput)
				w.Header().Set("ETag", out.ETag)
				b := make([]byte, 0, 64)
				b = append(b, `{"data":{"message":`...)
				b = strconv.AppendQuote(b, out.Body.Message)
				b = append(b, `,"count":`...)
				b = strconv.AppendInt(b, int64(out.Body.Count), 10)
				b = append(b, '}', '}', '\n')
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(out.Status)
				_, err := w.Write(b)
				return err
			},
		})

		codecAPI := server.NewAPI(server.NewServeMuxAdapter())
		server.Register(codecAPI.Group("/api"), server.Operation{
			OperationID: "conf-codec", Method: http.MethodGet, Path: "/thing/:id",
		}, confHandler)

		reflectAPI := server.NewAPI(server.NewServeMuxAdapter())
		server.Register(reflectAPI.Group("/api"), server.Operation{
			OperationID: "conf-reflect", Method: http.MethodGet, Path: "/thing/:id",
		}, confHandler)

		codecRec := httptest.NewRecorder()
		codecAPI.ServeHTTP(codecRec, httptest.NewRequest(http.MethodGet, "/api/thing/42", nil))
		reflectRec := httptest.NewRecorder()
		reflectAPI.ServeHTTP(reflectRec, httptest.NewRequest(http.MethodGet, "/api/thing/42", nil))

		if codecRec.Code != reflectRec.Code {
			t.Fatalf("status: codec=%d reflection=%d", codecRec.Code, reflectRec.Code)
		}
		if codecRec.Body.String() != reflectRec.Body.String() {
			t.Fatalf("body differs:\n codec      = %q\n reflection = %q", codecRec.Body.String(), reflectRec.Body.String())
		}
		if codecRec.Header().Get("ETag") != reflectRec.Header().Get("ETag") {
			t.Fatalf("ETag: codec=%q reflection=%q", codecRec.Header().Get("ETag"), reflectRec.Header().Get("ETag"))
		}
		if codecRec.Header().Get("Content-Type") != reflectRec.Header().Get("Content-Type") {
			t.Fatalf("content-type: codec=%q reflection=%q", codecRec.Header().Get("Content-Type"), reflectRec.Header().Get("Content-Type"))
		}
	})
}
