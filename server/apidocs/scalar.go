package apidocs

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/webdeveloperben/tyche/server"
)

type ScalarOptions struct {
	CDNURL string
}

type scalarRenderer struct {
	opts ScalarOptions
}

func Scalar(opts ...ScalarOptions) Renderer {
	var resolved ScalarOptions
	if len(opts) > 0 {
		resolved = opts[0]
	}
	return scalarRenderer{opts: resolved}
}

func (r scalarRenderer) Handler(page PageOptions) server.HandlerFunc {
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
		cdnURL = "https://cdn.jsdelivr.net/npm/@scalar/api-reference"
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
      html, body, #app {
        margin: 0;
        width: 100%%;
        height: 100%%;
      }
      body {
        background: #f6f3ee;
      }
    </style>
  </head>
  <body>
    <div id="app"></div>
    <script src=%s></script>
    <script>
      Scalar.createApiReference('#app', {
        url: %s,
        pageTitle: %s,
      })
    </script>
  </body>
</html>
`, title, cdnURLJSON, specURLJSON, titleJSON)

	return htmlHandler(html)
}

func htmlHandler(html string) server.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) error {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if req.Method == http.MethodHead {
			w.WriteHeader(http.StatusOK)
			return nil
		}
		w.WriteHeader(http.StatusOK)
		_, err := w.Write([]byte(html))
		return err
	}
}
