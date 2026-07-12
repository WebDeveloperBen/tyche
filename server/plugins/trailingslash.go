package plugins

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/webdeveloperben/tyche/server"
)

type TrailingSlashConfig struct {
	Redirect bool
}

func TrailingSlash(cfg ...TrailingSlashConfig) server.Middleware {
	c := TrailingSlashConfig{Redirect: true}
	if len(cfg) > 0 {
		c = cfg[0]
	}

	return func(next server.HandlerFunc) server.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) error {
			path := r.URL.Path

			if len(path) > 1 && path[len(path)-1] == '/' {
				if c.Redirect {
					trimmed := path[:len(path)-1]
					target := trimmed
					if r.URL.RawQuery != "" {
						target = trimmed + "?" + r.URL.RawQuery
					}

					// Validate the redirect target is a local (relative) URL to
					// prevent open redirects to external hosts. Backslashes are
					// normalized to forward slashes since some browsers treat
					// "\" as "/", which could otherwise smuggle a host past parsing.
					normalized := strings.ReplaceAll(target, "\\", "/")
					parsed, err := url.Parse(normalized)
					if err != nil || parsed.Hostname() != "" {
						http.Error(w, "Bad Request", http.StatusBadRequest)
						return nil //nolint:nilerr // request already rejected with 400; nothing to propagate
					}

					// Target validated above as a local/relative URL, so this is not an open redirect.
					http.Redirect(w, r, normalized, http.StatusMovedPermanently) //nolint:gosec // G710: target validated as local URL
					return nil
				}

				trimmed := path[:len(path)-1]
				r.URL.Path = trimmed
			}

			return next(w, r)
		}
	}
}

func TrailingSlashWithDefaults() server.Middleware {
	return TrailingSlash(TrailingSlashConfig{Redirect: true})
}

func TrailingSlashRemove() server.Middleware {
	return TrailingSlash(TrailingSlashConfig{Redirect: false})
}
