package apidocs

import (
	"errors"
	"net/http"

	"github.com/webdeveloperben/tyche/server"
)

type Renderer interface {
	Handler(opts PageOptions) server.HandlerFunc
}

type PageOptions struct {
	Title   string
	SpecURL string
}

type UIMount struct {
	Renderer Renderer
	Path     string
}

type Config struct {
	Title    string
	SpecPath string
	UIs      []UIMount
}

func Mount(router *server.API, cfg Config) error {
	if router == nil {
		return errors.New("apidocs router is required")
	}
	specPath := cfg.SpecPath
	if specPath == "" {
		specPath = "/openapi.json"
	}

	title := cfg.Title
	if title == "" {
		title = router.OpenAPI().Info.Title
	}

	if err := router.MountOpenAPI(specPath); err != nil {
		return err
	}

	page := PageOptions{
		Title:   title,
		SpecURL: specPath,
	}

	for _, ui := range cfg.UIs {
		if ui.Path == "" || ui.Renderer == nil {
			return errors.New("apidocs UI mounts require both path and renderer")
		}
		handler := ui.Renderer.Handler(page)
		if err := router.HandleE(http.MethodGet, ui.Path, handler); err != nil {
			return err
		}
		if err := router.HandleE(http.MethodHead, ui.Path, handler); err != nil {
			return err
		}
	}
	return nil
}
