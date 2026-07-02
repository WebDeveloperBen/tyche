// Package main is a fixture: a single-file main package that registers a typed
// route with its input/output types declared in package main. servergen must
// key the codec under "main" (matching what reflect reports at runtime) so it
// works end to end. It is never built or run — only analysed by servergen tests.
package main

import (
	"context"
	"net/http"

	"github.com/webdeveloperben/tyche/server"
)

type Input struct {
	ID string `path:"id"`
}

type Output struct {
	Body struct {
		ID string `json:"id"`
	}
}

func handler(_ context.Context, in *Input) (*Output, error) {
	out := &Output{}
	out.Body.ID = in.ID
	return out, nil
}

func main() {
	api := server.NewAPI(server.NewServeMuxAdapter())
	server.Register(api.Group("/api"), server.Operation{
		OperationID: "main-route",
		Method:      http.MethodGet,
		Path:        "/things/:id",
	}, handler)
}
