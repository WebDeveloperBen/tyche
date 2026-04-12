//go:build comparison
// +build comparison

package server

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"sync"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/gin-gonic/gin"
	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
)

type compInput struct {
	ID    string `path:"id"`
	Name  string `json:"name" validate:"min=2,max=50"`
	Email string `json:"email" validate:"email"`
	Age   int    `json:"age"`
}

type compOutput struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
	Age   int    `json:"age"`
}

type compNestedInput struct {
	Name     string                 `json:"name" validate:"required"`
	Metadata map[string]interface{} `json:"metadata"`
	Tags     []string               `json:"tags"`
}

type compNestedOutput struct {
	Success bool `json:"success"`
}

type humaCompParamInput struct {
	ID string `path:"id"`
}

type humaCompBodyInput struct {
	Body compInput
}

type humaCompNestedInput struct {
	Body compNestedInput
}

type humaCompOutput struct {
	Body compOutput
}

type humaCompNestedOutput struct {
	Body compNestedOutput
}

var compBody = []byte(`{"name":"Ben","email":"ben@example.com","age":30}`)
var compNestedBody = []byte(`{"name":"Test","metadata":{"key":"value"},"tags":["a","b"]}`)

var structValidator = validator.New()
var compBenchmarkSetupOnce sync.Once
var compJSONBufPool = sync.Pool{
	New: func() any {
		buf := make([]byte, 0, 128)
		return &buf
	},
}

var compNestedSuccessJSON = []byte("{\"success\":true}")

func compBenchmarkSetup() {
	compBenchmarkSetupOnce.Do(func() {
		inputType := reflect.TypeOf(compInput{})
		outputType := reflect.TypeOf(compOutput{})
		inputKey := generatedTypeKey(inputType)
		outputKey := generatedTypeKey(outputType)

		RegisterGeneratedCodec(GeneratedRouteMeta{
			PackagePath:       "github.com/webdeveloperben/tyche/server",
			OperationID:       "comp-static",
			Method:            http.MethodGet,
			Path:              "/static",
			InputTypeKey:      inputKey,
			OutputTypeKey:     outputKey,
			HasGeneratedCodec: true,
		}, GeneratedRouteCodec{
			Parse: func(req *http.Request) (any, error) { return &compInput{}, nil },
			Write: func(w http.ResponseWriter, req *http.Request, out any) error {
				t, _ := out.(*compOutput)
				return writeCompOutputResponse(w, t)
			},
		})

		RegisterGeneratedCodec(GeneratedRouteMeta{
			PackagePath:       "github.com/webdeveloperben/tyche/server",
			OperationID:       "comp-param",
			Method:            http.MethodGet,
			Path:              "/param/:id",
			InputTypeKey:      inputKey,
			OutputTypeKey:     outputKey,
			HasGeneratedCodec: true,
		}, GeneratedRouteCodec{
			Parse: func(req *http.Request) (any, error) {
				return &compInput{ID: Param(req, "id")}, nil
			},
			Write: func(w http.ResponseWriter, req *http.Request, out any) error {
				t, _ := out.(*compOutput)
				return writeCompOutputResponse(w, t)
			},
		})

		RegisterGeneratedCodec(GeneratedRouteMeta{
			PackagePath:       "github.com/webdeveloperben/tyche/server",
			OperationID:       "comp-body",
			Method:            http.MethodPost,
			Path:              "/body",
			InputTypeKey:      inputKey,
			OutputTypeKey:     outputKey,
			HasGeneratedCodec: true,
		}, GeneratedRouteCodec{
			Parse: func(req *http.Request) (any, error) {
				var in compInput
				if err := DecodeRequestJSONBodyFast(req, &in); err != nil {
					return nil, err
				}
				return &in, nil
			},
			Write: func(w http.ResponseWriter, req *http.Request, out any) error {
				t, _ := out.(*compOutput)
				return writeCompOutputResponse(w, t)
			},
		})

		// Nested - JSON parse with nested objects
		nestedInputType := reflect.TypeOf(compNestedInput{})
		nestedOutputType := reflect.TypeOf(compNestedOutput{})
		nestedInputKey := generatedTypeKey(nestedInputType)
		nestedOutputKey := generatedTypeKey(nestedOutputType)

		RegisterGeneratedCodec(GeneratedRouteMeta{
			PackagePath:       "github.com/webdeveloperben/tyche/server",
			OperationID:       "comp-nested",
			Method:            http.MethodPost,
			Path:              "/nested",
			InputTypeKey:      nestedInputKey,
			OutputTypeKey:     nestedOutputKey,
			HasGeneratedCodec: true,
		}, GeneratedRouteCodec{
			Parse: func(req *http.Request) (any, error) {
				var in compNestedInput
				if err := DecodeRequestJSONBodyFast(req, &in); err != nil {
					return nil, err
				}
				// Manual validation like Chi/Gin do
				if in.Name == "" {
					return nil, NewHTTPError(400, "name required")
				}
				return &in, nil
			},
			Write: func(w http.ResponseWriter, req *http.Request, out any) error {
				t, _ := out.(*compNestedOutput)
				return writeCompNestedOutputResponse(w, t)
			},
		})
	})
}

func writeCompOutputResponse(w http.ResponseWriter, out *compOutput) error {
	if out == nil {
		w.WriteHeader(http.StatusNoContent)
		return nil
	}
	bufPtr := compJSONBufPool.Get().(*[]byte)
	b := (*bufPtr)[:0]
	b = append(b, `{"id":`...)
	b = strconv.AppendQuote(b, out.ID)
	b = append(b, `,"name":`...)
	b = strconv.AppendQuote(b, out.Name)
	b = append(b, `,"email":`...)
	b = strconv.AppendQuote(b, out.Email)
	b = append(b, `,"age":`...)
	b = strconv.AppendInt(b, int64(out.Age), 10)
	b = append(b, '}', '\n')
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, err := w.Write(b)
	*bufPtr = b[:0]
	compJSONBufPool.Put(bufPtr)
	return err
}

func writeCompNestedOutputResponse(w http.ResponseWriter, out *compNestedOutput) error {
	if out == nil {
		w.WriteHeader(http.StatusNoContent)
		return nil
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, err := w.Write(compNestedSuccessJSON)
	return err
}

func compBenchmarkRouter() *Router {
	compBenchmarkSetup()
	r := NewRouter()
	g := r.Group("/comp")
	Register(g, Operation{OperationID: "comp-static", Method: http.MethodGet, Path: "/static"}, func(c context.Context, i *compInput) (*compOutput, error) {
		return &compOutput{ID: "1", Name: "Ben", Email: "ben@example.com", Age: 30}, nil
	})
	Register(g, Operation{OperationID: "comp-param", Method: http.MethodGet, Path: "/param/:id"}, func(c context.Context, i *compInput) (*compOutput, error) {
		return &compOutput{ID: i.ID, Name: "Ben", Email: "ben@example.com", Age: 30}, nil
	})
	Register(g, Operation{OperationID: "comp-body", Method: http.MethodPost, Path: "/body"}, func(c context.Context, i *compInput) (*compOutput, error) {
		return &compOutput{ID: "new", Name: i.Name, Email: i.Email, Age: i.Age}, nil
	})
	Register(g, Operation{OperationID: "comp-nested", Method: http.MethodPost, Path: "/nested"}, func(c context.Context, i *compNestedInput) (*compNestedOutput, error) {
		return &compNestedOutput{Success: true}, nil
	})
	return r
}

func BenchmarkRouter_Static(b *testing.B) {
	tyche := compBenchmarkRouter()
	chiR := chi.NewMux()
	chiR.Get("/comp/static", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(compOutput{ID: "1", Name: "Ben", Email: "ben@example.com", Age: 30})
	})
	ginR := gin.New()
	ginR.GET("/comp/static", func(c *gin.Context) {
		c.JSON(200, compOutput{ID: "1", Name: "Ben", Email: "ben@example.com", Age: 30})
	})
	humaChi := chi.NewMux()
	humaR := humachi.New(humaChi, huma.DefaultConfig("Test", "1.0.0"))
	huma.Register(humaR, huma.Operation{Method: http.MethodGet, Path: "/comp/static"}, func(ctx context.Context, i *struct{}) (*humaCompOutput, error) {
		return &humaCompOutput{Body: compOutput{ID: "1", Name: "Ben", Email: "ben@example.com", Age: 30}}, nil
	})
	req := httptest.NewRequest(http.MethodGet, "/comp/static", nil)
	b.Run("Homebase", func(b *testing.B) { runReq(b, tyche, req) })
	b.Run("Chi", func(b *testing.B) { runReq(b, chiR, req) })
	b.Run("Gin", func(b *testing.B) { runReq(b, ginR, req) })
	b.Run("Huma", func(b *testing.B) { runReq(b, humaChi, req) })
}

func BenchmarkRouter_Param(b *testing.B) {
	tyche := compBenchmarkRouter()
	chiR := chi.NewMux()
	chiR.Get("/comp/param/:id", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(compOutput{ID: chi.URLParam(r, "id"), Name: "Ben", Email: "ben@example.com", Age: 30})
	})
	ginR := gin.New()
	ginR.GET("/comp/param/:id", func(c *gin.Context) {
		c.JSON(200, compOutput{ID: c.Param("id"), Name: "Ben", Email: "ben@example.com", Age: 30})
	})
	humaChi := chi.NewMux()
	humaR := humachi.New(humaChi, huma.DefaultConfig("Test", "1.0.0"))
	huma.Register(humaR, huma.Operation{Method: http.MethodGet, Path: "/comp/param/{id}"}, func(ctx context.Context, i *humaCompParamInput) (*humaCompOutput, error) {
		return &humaCompOutput{Body: compOutput{ID: i.ID, Name: "Ben", Email: "ben@example.com", Age: 30}}, nil
	})
	req := httptest.NewRequest(http.MethodGet, "/comp/param/123", nil)
	b.Run("Homebase", func(b *testing.B) { runReq(b, tyche, req) })
	b.Run("Chi", func(b *testing.B) { runReq(b, chiR, req) })
	b.Run("Gin", func(b *testing.B) { runReq(b, ginR, req) })
	b.Run("Huma", func(b *testing.B) { runReq(b, humaChi, req) })
}

func BenchmarkRouter_Body(b *testing.B) {
	tyche := compBenchmarkRouter()
	chiR := chi.NewMux()
	chiR.Post("/comp/body", func(w http.ResponseWriter, r *http.Request) {
		var in compInput
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			http.Error(w, `{"error":"invalid json"}`, 400)
			return
		}
		if err := structValidator.Struct(&in); err != nil {
			http.Error(w, `{"error":"validation"}`, 400)
			return
		}
		w.WriteHeader(200)
		json.NewEncoder(w).Encode(compOutput{ID: "new", Name: in.Name, Email: in.Email, Age: in.Age})
	})
	ginR := gin.New()
	ginR.POST("/comp/body", func(c *gin.Context) {
		var in compInput
		if err := c.ShouldBindJSON(&in); err != nil {
			c.JSON(400, map[string]string{"error": "invalid json"})
			return
		}
		if err := structValidator.Struct(&in); err != nil {
			c.JSON(400, map[string]string{"error": "validation"})
			return
		}
		c.JSON(200, compOutput{ID: "new", Name: in.Name, Email: in.Email, Age: in.Age})
	})
	humaChi := chi.NewMux()
	humaR := humachi.New(humaChi, huma.DefaultConfig("Test", "1.0.0"))
	huma.Register(humaR, huma.Operation{Method: http.MethodPost, Path: "/comp/body"}, func(ctx context.Context, i *humaCompBodyInput) (*humaCompOutput, error) {
		return &humaCompOutput{
			Body: compOutput{ID: "new", Name: i.Body.Name, Email: i.Body.Email, Age: i.Body.Age},
		}, nil
	})
	req := httptest.NewRequest(http.MethodPost, "/comp/body", bytes.NewReader(compBody))
	req.Header.Set("Content-Type", "application/json")
	b.Run("Homebase", func(b *testing.B) { runReqBody(b, tyche, req, compBody) })
	b.Run("Chi", func(b *testing.B) { runReqBody(b, chiR, req, compBody) })
	b.Run("Gin", func(b *testing.B) { runReqBody(b, ginR, req, compBody) })
	b.Run("Huma", func(b *testing.B) { runReqBody(b, humaChi, req, compBody) })
}

func BenchmarkRouter_Nested(b *testing.B) {
	tyche := compBenchmarkRouter()
	chiR := chi.NewMux()
	chiR.Post("/comp/nested", func(w http.ResponseWriter, r *http.Request) {
		var in compNestedInput
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			http.Error(w, `{"error":"invalid json"}`, 400)
			return
		}
		if err := structValidator.Struct(&in); err != nil {
			http.Error(w, `{"error":"validation"}`, 400)
			return
		}
		w.WriteHeader(200)
		json.NewEncoder(w).Encode(compNestedOutput{Success: true})
	})
	ginR := gin.New()
	ginR.POST("/comp/nested", func(c *gin.Context) {
		var in compNestedInput
		if err := c.ShouldBindJSON(&in); err != nil {
			c.JSON(400, map[string]string{"error": "invalid json"})
			return
		}
		if err := structValidator.Struct(&in); err != nil {
			c.JSON(400, map[string]string{"error": "validation"})
			return
		}
		c.JSON(200, compNestedOutput{Success: true})
	})
	humaChi := chi.NewMux()
	humaR := humachi.New(humaChi, huma.DefaultConfig("Test", "1.0.0"))
	huma.Register(humaR, huma.Operation{Method: http.MethodPost, Path: "/comp/nested"}, func(ctx context.Context, i *humaCompNestedInput) (*humaCompNestedOutput, error) {
		return &humaCompNestedOutput{Body: compNestedOutput{Success: true}}, nil
	})
	req := httptest.NewRequest(http.MethodPost, "/comp/nested", bytes.NewReader(compNestedBody))
	req.Header.Set("Content-Type", "application/json")
	b.Run("Homebase", func(b *testing.B) { runReqBody(b, tyche, req, compNestedBody) })
	b.Run("Chi", func(b *testing.B) { runReqBody(b, chiR, req, compNestedBody) })
	b.Run("Gin", func(b *testing.B) { runReqBody(b, ginR, req, compNestedBody) })
	b.Run("Huma", func(b *testing.B) { runReqBody(b, humaChi, req, compNestedBody) })
}

func runReq(b *testing.B, r http.Handler, req *http.Request) {
	w := httptest.NewRecorder()
	for b.Loop() {
		r.ServeHTTP(w, req)
	}
}

func runReqBody(b *testing.B, r http.Handler, req *http.Request, body []byte) {
	w := httptest.NewRecorder()
	for b.Loop() {
		req.Body = io.NopCloser(bytes.NewReader(body))
		r.ServeHTTP(w, req)
	}
}
