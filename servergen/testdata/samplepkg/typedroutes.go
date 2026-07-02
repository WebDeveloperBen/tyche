package samplepkg

import (
	"context"
	"net/http"

	"github.com/webdeveloperben/tyche/server"
)

const (
	StaticPath        = "/typed-static"
	ParamPattern      = "/typed-param/:id"
	ParamURL          = "/typed-param/12345"
	QueryHeaderPath   = "/typed-query-header"
	QueryHeaderURL    = "/typed-query-header?page=42"
	BodyPath          = "/typed-body"
	ValidatedBodyPath = "/typed-validated-body"
	NestedBodyPath    = "/typed-nested-body"
)

var (
	BodyJSON          = []byte(`{"name":"Ben","email":"ben@example.com","age":42}`)
	ValidatedBodyJSON = []byte(`{"name":"Benny","kind":"a","code":"AB"}`)
	NestedBodyJSON    = []byte(`{"name":"Ben","meta":{"code":"AB"},"children":[{"code":"CD"},{"code":"EF"}]}`)
)

type StaticInput struct{}

type ParamInput struct {
	ID string `path:"id"`
}

type QueryHeaderInput struct {
	Page  int    `query:"page"`
	Token string `header:"X-Api-Key"`
}

type BodyInput struct {
	Name  string `json:"name"`
	Email string `json:"email"`
	Age   int    `json:"age"`
}

type ValidatedBodyInput struct {
	Name string `json:"name" validate:"min=2,max=10"`
	Kind string `json:"kind" validate:"oneof=a b"`
	Code string `json:"code" validate:"pattern=^[A-Z]{2}$"`
}

type NestedBodyInput struct {
	Name string `json:"name"`
	Meta struct {
		Code string `json:"code"`
	} `json:"meta"`
	Children []struct {
		Code string `json:"code"`
	} `json:"children"`
}

type Output struct {
	Body struct {
		OK bool `json:"ok"`
	} `body:"true"`
}

func RegisterTypedRoutes(group *server.APIGroup) {
	server.Register(group, server.Operation{
		OperationID: "benchmark-typed-static",
		Method:      http.MethodGet,
		Path:        StaticPath,
	}, func(ctx context.Context, in *StaticInput) (*Output, error) {
		return ok(), nil
	})

	server.Register(group, server.Operation{
		OperationID: "benchmark-typed-param",
		Method:      http.MethodGet,
		Path:        ParamPattern,
	}, func(ctx context.Context, in *ParamInput) (*Output, error) {
		_ = in.ID
		return ok(), nil
	})

	server.Register(group, server.Operation{
		OperationID: "benchmark-typed-query-header",
		Method:      http.MethodGet,
		Path:        QueryHeaderPath,
	}, func(ctx context.Context, in *QueryHeaderInput) (*Output, error) {
		_, _ = in.Page, in.Token
		return ok(), nil
	})

	server.Register(group, server.Operation{
		OperationID: "benchmark-typed-body",
		Method:      http.MethodPost,
		Path:        BodyPath,
	}, func(ctx context.Context, in *BodyInput) (*Output, error) {
		_, _, _ = in.Name, in.Email, in.Age
		return ok(), nil
	})

	server.Register(group, server.Operation{
		OperationID: "benchmark-typed-validated-body",
		Method:      http.MethodPost,
		Path:        ValidatedBodyPath,
	}, func(ctx context.Context, in *ValidatedBodyInput) (*Output, error) {
		_, _, _ = in.Name, in.Kind, in.Code
		return ok(), nil
	})

	server.Register(group, server.Operation{
		OperationID: "benchmark-typed-nested-body",
		Method:      http.MethodPost,
		Path:        NestedBodyPath,
	}, func(ctx context.Context, in *NestedBodyInput) (*Output, error) {
		_, _, _ = in.Name, in.Meta.Code, len(in.Children)
		return ok(), nil
	})
}

func ok() *Output {
	out := &Output{}
	out.Body.OK = true
	return out
}
