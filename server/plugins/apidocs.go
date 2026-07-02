package plugins

import (
	"github.com/webdeveloperben/tyche/server"
	"github.com/webdeveloperben/tyche/server/apidocs"
)

type apiDocsPlugin struct {
	cfg apidocs.Config
}

func (p apiDocsPlugin) Register(r *server.API) error {
	return apidocs.Mount(r, p.cfg)
}

func APIDocs(cfg apidocs.Config) Plugin {
	return apiDocsPlugin{cfg: cfg}
}
