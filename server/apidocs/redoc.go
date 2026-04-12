package apidocs

import (
	"encoding/json"
	"fmt"

	"github.com/webdeveloperben/tyche/server"
)

type RedocOptions struct {
	CDNURL string
}

type redocRenderer struct {
	opts RedocOptions
}

func Redoc(opts ...RedocOptions) Renderer {
	var resolved RedocOptions
	if len(opts) > 0 {
		resolved = opts[0]
	}
	return redocRenderer{opts: resolved}
}

func (r redocRenderer) Handler(page PageOptions) server.HandlerFunc {
	title := page.Title
	if title == "" {
		title = "API Reference"
	}

	specURL := page.SpecURL
	if specURL == "" {
		specURL = "/openapi.json"
	}

	cdnURL := r.opts.CDNURL
	if cdnURL == "" {
		cdnURL = "https://cdn.jsdelivr.net/npm/redoc@next/bundles/redoc.standalone.js"
	}

	titleJSON, _ := json.Marshal(title)
	specURLJSON, _ := json.Marshal(specURL)
	cdnURLJSON, _ := json.Marshal(cdnURL)

	html := fmt.Sprintf(`<!doctype html>
<html lang="en-AU">
  <head>
    <meta charset="utf-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1" />
    <title>%s</title>
    <style>
      body {
        margin: 0;
        background: #fffdf8;
      }
    </style>
  </head>
  <body>
    <redoc spec-url=%s></redoc>
    <script>
      window.__redoc_state = { pageTitle: %s }
      document.title = window.__redoc_state.pageTitle
    </script>
    <script src=%s></script>
  </body>
</html>
`, title, specURLJSON, titleJSON, cdnURLJSON)

	return htmlHandler(html)
}
