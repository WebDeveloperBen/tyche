package server_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/webdeveloperben/tyche/server"
)

type testCodec struct{ mediaType string }

func (c testCodec) MediaType() string { return c.mediaType }
func (c testCodec) DecodeRequest(*http.Request, any) error {
	return errors.New("test codec decode not implemented")
}

func (c testCodec) EncodeSuccess(http.ResponseWriter, int, any) error {
	return errors.New("test codec encode not implemented")
}

type jsonLikeCodec struct{ mediaType string }

func (c jsonLikeCodec) MediaType() string { return c.mediaType }
func (c jsonLikeCodec) DecodeRequest(req *http.Request, dst any) error {
	return json.NewDecoder(req.Body).Decode(dst)
}

func (c jsonLikeCodec) EncodeSuccess(w http.ResponseWriter, status int, data any) error {
	w.Header().Set("Content-Type", c.mediaType)
	w.WriteHeader(status)
	return json.NewEncoder(w).Encode(server.DataResponse{Data: data})
}

func TestJSONCodecDecodeRequestAndEncodeSuccess(t *testing.T) {
	codec := server.JSONCodec{}
	if got := codec.MediaType(); got != "application/json" {
		t.Fatalf("MediaType = %q", got)
	}

	rawReq := httptest.NewRequest(http.MethodPost, "/users", strings.NewReader(`{"name":"Ada"}`))
	rawReq.Header.Set("Content-Type", "application/json")
	body, err := codec.ReadRequest(rawReq)
	if err != nil {
		t.Fatalf("ReadRequest: %v", err)
	}
	if string(body) != `{"name":"Ada"}` {
		t.Fatalf("ReadRequest body = %q", body)
	}

	var in struct {
		Name string `json:"name"`
	}
	req := httptest.NewRequest(http.MethodPost, "/users", strings.NewReader(`{"name":"Ada"}`))
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	if err := codec.DecodeRequest(req, &in); err != nil {
		t.Fatalf("DecodeRequest: %v", err)
	}
	if in.Name != "Ada" {
		t.Fatalf("decoded name = %q", in.Name)
	}

	var strictIn struct {
		Name string `json:"name"`
	}
	strictReq := httptest.NewRequest(http.MethodPost, "/users", strings.NewReader(`{"name":"Ada"}`))
	strictReq.Header.Set("Content-Type", "application/json")
	if err := codec.DecodeRequestStrict(strictReq, &strictIn, true, nil); err != nil {
		t.Fatalf("DecodeRequestStrict: %v", err)
	}
	if strictIn.Name != "Ada" {
		t.Fatalf("strict decoded name = %q", strictIn.Name)
	}

	rec := httptest.NewRecorder()
	if err := codec.EncodeSuccess(rec, http.StatusCreated, struct {
		ID string `json:"id"`
	}{ID: "u1"}); err != nil {
		t.Fatalf("EncodeSuccess: %v", err)
	}
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d", rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("content type = %q", got)
	}
	var out struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if out.Data.ID != "u1" {
		t.Fatalf("data.id = %q", out.Data.ID)
	}
}

func TestAPIConfigCodecs(t *testing.T) {
	t.Run("default json codec", func(t *testing.T) {
		api := server.NewAPI(server.NewServeMuxAdapter())
		codecs := api.Codecs()
		if len(codecs) != 1 {
			t.Fatalf("len(Codecs) = %d", len(codecs))
		}
		if got := codecs[0].MediaType(); got != "application/json" {
			t.Fatalf("codec[0] media type = %q", got)
		}
	})

	t.Run("adds custom codec and keeps copy safe", func(t *testing.T) {
		api := server.NewAPI(server.NewServeMuxAdapter(), server.APIConfig{
			Codecs: []server.Codec{testCodec{mediaType: "application/vnd.tyche+json"}},
		})
		codecs := api.Codecs()
		if len(codecs) != 2 {
			t.Fatalf("len(Codecs) = %d", len(codecs))
		}
		if got := codecs[1].MediaType(); got != "application/vnd.tyche+json" {
			t.Fatalf("codec[1] media type = %q", got)
		}
		codecs[0] = testCodec{mediaType: "mutated"}
		if got := api.Codecs()[0].MediaType(); got != "application/json" {
			t.Fatalf("Codecs returned mutable backing slice, first media type = %q", got)
		}
	})

	t.Run("ignores nil empty and duplicate codecs", func(t *testing.T) {
		api := server.NewAPI(server.NewServeMuxAdapter(), server.APIConfig{
			Codecs: []server.Codec{
				nil,
				testCodec{},
				testCodec{mediaType: " application/json "},
				testCodec{mediaType: "APPLICATION/JSON"},
				testCodec{mediaType: "application/vendor+json"},
			},
		})
		codecs := api.Codecs()
		if len(codecs) != 2 {
			t.Fatalf("len(Codecs) = %d", len(codecs))
		}
		if got := codecs[0].MediaType(); got != "application/json" {
			t.Fatalf("codec[0] media type = %q", got)
		}
		if got := codecs[1].MediaType(); got != "application/vendor+json" {
			t.Fatalf("codec[1] media type = %q", got)
		}
	})
}

func TestAPIConfigCodecsOpenAPIContentTypes(t *testing.T) {
	type input struct {
		Name string `json:"name"`
	}
	type output struct {
		Body struct {
			ID string `json:"id"`
		} `body:"true"`
	}

	api := server.NewAPI(server.NewServeMuxAdapter(), server.APIConfig{
		Codecs: []server.Codec{testCodec{mediaType: "application/vnd.tyche+json"}},
	})
	server.Register(api, server.Operation{
		OperationID: "codec-user",
		Method:      http.MethodPost,
		Path:        "/users",
	}, func(_ context.Context, in *input) (*output, error) {
		out := &output{}
		out.Body.ID = in.Name
		return out, nil
	})

	op := api.OpenAPI().Paths["/users"].POST
	if op == nil {
		t.Fatal("expected POST /users operation")
	}
	for _, mediaType := range []string{"application/json", "application/vnd.tyche+json"} {
		if op.RequestBody == nil || op.RequestBody.Content[mediaType] == nil {
			t.Fatalf("request body missing media type %q: %#v", mediaType, op.RequestBody)
		}
		if op.Responses["200"] == nil || op.Responses["200"].Content[mediaType] == nil {
			t.Fatalf("response missing media type %q: %#v", mediaType, op.Responses["200"])
		}
	}
}

func TestAPIConfigCodecsRuntimeSelection(t *testing.T) {
	type input struct {
		Body struct {
			Name string `json:"name"`
		}
	}
	type output struct {
		Body struct {
			Greeting string `json:"greeting"`
		} `body:"true"`
	}

	api := server.NewAPI(server.NewServeMuxAdapter(), server.APIConfig{
		Codecs: []server.Codec{jsonLikeCodec{mediaType: "application/vnd.tyche+json"}},
	})
	server.Register(api, server.Operation{
		OperationID: "codec-runtime-selection",
		Method:      http.MethodPost,
		Path:        "/users",
	}, func(_ context.Context, in *input) (*output, error) {
		out := &output{}
		out.Body.Greeting = "hello " + in.Body.Name
		return out, nil
	})

	req := httptest.NewRequest(http.MethodPost, "/users", strings.NewReader(`{"name":"Ada"}`))
	req.Header.Set("Content-Type", "application/vnd.tyche+json")
	req.Header.Set("Accept", "application/vnd.tyche+json")
	rec := httptest.NewRecorder()
	api.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); got != "application/vnd.tyche+json" {
		t.Fatalf("content type = %q", got)
	}
	var decoded struct {
		Data struct {
			Greeting string `json:"greeting"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &decoded); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if decoded.Data.Greeting != "hello Ada" {
		t.Fatalf("greeting = %q", decoded.Data.Greeting)
	}
}

func TestAPIConfigCodecsBypassJSONOnlyGeneratedCodec(t *testing.T) {
	type input struct {
		Body struct {
			Name string `json:"name"`
		}
	}
	type output struct {
		Body struct {
			Greeting string `json:"greeting"`
		} `body:"true"`
	}

	server.RegisterGeneratedCodec(server.GeneratedRouteMeta{
		OperationID:       "codec-generated-bypass",
		Method:            http.MethodPost,
		Path:              "/generated-bypass",
		HasGeneratedCodec: true,
	}, server.GeneratedRouteCodec{
		Parse: func(*http.Request) (any, error) {
			return nil, errors.New("generated JSON parser should have been bypassed")
		},
		Write: func(http.ResponseWriter, *http.Request, any) error {
			return errors.New("generated JSON writer should have been bypassed")
		},
	})

	api := server.NewAPI(server.NewServeMuxAdapter(), server.APIConfig{
		Codecs: []server.Codec{jsonLikeCodec{mediaType: "application/vnd.tyche+json"}},
	})
	server.Register(api, server.Operation{
		OperationID: "codec-generated-bypass",
		Method:      http.MethodPost,
		Path:        "/generated-bypass",
	}, func(_ context.Context, in *input) (*output, error) {
		out := &output{}
		out.Body.Greeting = "hello " + in.Body.Name
		return out, nil
	})

	req := httptest.NewRequest(http.MethodPost, "/generated-bypass", strings.NewReader(`{"name":"Ada"}`))
	req.Header.Set("Content-Type", "application/vnd.tyche+json")
	req.Header.Set("Accept", "application/vnd.tyche+json")
	rec := httptest.NewRecorder()
	api.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); got != "application/vnd.tyche+json" {
		t.Fatalf("content type = %q", got)
	}
}

func TestRouteContentTypeOptionsRestrictCodecs(t *testing.T) {
	type input struct {
		Body struct {
			Name string `json:"name"`
		}
	}
	type output struct {
		Body struct {
			Greeting string `json:"greeting"`
		} `body:"true"`
	}
	handler := func(_ context.Context, in *input) (*output, error) {
		out := &output{}
		out.Body.Greeting = "hello " + in.Body.Name
		return out, nil
	}

	api := server.NewAPI(server.NewServeMuxAdapter(), server.APIConfig{
		Codecs: []server.Codec{jsonLikeCodec{mediaType: "application/vnd.tyche+json"}},
	})
	server.Register(
		api, server.Operation{
			OperationID: "codec-route-restrict",
			Method:      http.MethodPost,
			Path:        "/restricted",
		}, handler,
		server.WithRequestContentTypes("application/json"),
		server.WithResponseContentTypes("application/vnd.tyche+json"),
	)

	op := api.OpenAPI().Paths["/restricted"].POST
	if op == nil {
		t.Fatal("expected POST /restricted operation")
	}
	if op.RequestBody.Content["application/json"] == nil {
		t.Fatal("request body missing application/json")
	}
	if op.RequestBody.Content["application/vnd.tyche+json"] != nil {
		t.Fatal("request body should not advertise application/vnd.tyche+json")
	}
	if op.Responses["200"].Content["application/vnd.tyche+json"] == nil {
		t.Fatal("response missing application/vnd.tyche+json")
	}
	if op.Responses["200"].Content["application/json"] != nil {
		t.Fatal("response should not advertise application/json")
	}

	req := httptest.NewRequest(http.MethodPost, "/restricted", strings.NewReader(`{"name":"Ada"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/vnd.tyche+json")
	rec := httptest.NewRecorder()
	api.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); got != "application/vnd.tyche+json" {
		t.Fatalf("content type = %q", got)
	}

	req = httptest.NewRequest(http.MethodPost, "/restricted", strings.NewReader(`{"name":"Ada"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	rec = httptest.NewRecorder()
	api.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotAcceptable {
		t.Fatalf("status = %d, want 406; body = %s", rec.Code, rec.Body.String())
	}
}

func TestRouteContentTypeOptionsRejectUnregisteredCodec(t *testing.T) {
	type input struct{}
	type output struct{}

	api := server.NewAPI(server.NewServeMuxAdapter())
	err := server.RegisterE(api, server.Operation{
		OperationID: "codec-route-unregistered",
		Method:      http.MethodGet,
		Path:        "/unregistered",
	}, func(context.Context, *input) (*output, error) {
		return &output{}, nil
	}, server.WithResponseContentTypes("application/vnd.tyche+json"))
	if err == nil {
		t.Fatal("expected RegisterE to reject unregistered response codec")
	}
	if !strings.Contains(err.Error(), "not registered") {
		t.Fatalf("error = %v", err)
	}
}

func TestJSONCodecGeneratedSuccessHelpers(t *testing.T) {
	codec := server.JSONCodec{}
	bufPtr := codec.AcquireGeneratedSuccessBuffer()
	defer codec.ReleaseGeneratedSuccessBuffer(bufPtr)

	b := (*bufPtr)[:0]
	b = append(b, `{"data":{"id":`...)
	b = codec.AppendString(b, "u1")
	b = append(b, `,"ok":`...)
	b = codec.AppendBool(b, true)
	b = append(b, "}}\n"...)

	rec := httptest.NewRecorder()
	if err := codec.WriteGeneratedSuccess(rec, http.StatusCreated, b); err != nil {
		t.Fatalf("WriteGeneratedSuccess: %v", err)
	}
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d", rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("content type = %q", got)
	}
	if got := rec.Body.String(); got != "{\"data\":{\"id\":\"u1\",\"ok\":true}}\n" {
		t.Fatalf("body = %q", got)
	}
}
