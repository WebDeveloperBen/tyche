package embeddedpkg

import (
	"context"
	"fmt"
	"net/http"

	"github.com/webdeveloperben/tyche/pagination"
	"github.com/webdeveloperben/tyche/server"
)

type Input struct {
	TenantID string `path:"tenant_id"`
	pagination.Params
}

type Output struct {
	Name string `json:"name"`
}

func RegisterRoutes(group *server.APIGroup) {
	server.Register(group, server.Operation{
		OperationID: "embedded-list",
		Method:      http.MethodGet,
		Path:        "/tenants/:tenant_id",
	}, func(_ context.Context, input *Input) (*Output, error) {
		return &Output{Name: fmt.Sprintf("%s:%d:%s", input.TenantID, input.Limit, input.Cursor)}, nil
	}, server.WithPaginationConfig(pagination.Config{DefaultLimit: 5, MaxLimit: 10}))
}
