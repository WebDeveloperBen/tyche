package apidocs_test

import (
	"testing"

	"github.com/webdeveloperben/tyche/server"
	"github.com/webdeveloperben/tyche/server/apidocs"
)

func TestMount_ReturnsErrorForInvalidUIConfig(t *testing.T) {
	router := server.NewAPI(server.NewServeMuxAdapter())
	err := apidocs.Mount(router, apidocs.Config{
		UIs: []apidocs.UIMount{
			{Path: "/docs"},
		},
	})
	if err == nil {
		t.Fatal("expected invalid UI config error")
	}
}
