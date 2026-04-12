package plugins

import (
	"net/http"

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
					if r.URL.RawQuery != "" {
						http.Redirect(w, r, trimmed+"?"+r.URL.RawQuery, http.StatusMovedPermanently)
					} else {
						http.Redirect(w, r, trimmed, http.StatusMovedPermanently)
					}
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
