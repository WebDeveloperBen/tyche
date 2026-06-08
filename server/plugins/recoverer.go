package plugins

import (
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"runtime/debug"

	"github.com/webdeveloperben/tyche/server"
)

type RecovererConfig struct {
	Logger interface{ Error(...any) }
}

func Recoverer(cfg ...RecovererConfig) server.Middleware {
	c := RecovererConfig{}
	if len(cfg) > 0 {
		c = cfg[0]
	}
	m := &recovererMiddleware{logger: c.Logger}
	return m.Middleware()
}

type RecovererOption func(*recovererMiddleware)

func RecovererWithDefaults(cfg ...RecovererConfig) server.Middleware {
	m := &recovererMiddleware{}
	if len(cfg) > 0 {
		m.logger = cfg[0].Logger
	}
	if m.logger == nil {
		m.logger = &slogErrorLogger{logger: slog.Default()}
	}
	return m.Middleware()
}

type recovererMiddleware struct {
	logger interface{ Error(...any) }
}

func (m *recovererMiddleware) Register(r *server.Router) {
	r.Use(m.Middleware())
}

type slogErrorLogger struct {
	logger *slog.Logger
}

func (l *slogErrorLogger) Error(args ...any) {
	if len(args) < 2 {
		return
	}
	msg, ok := args[0].(string)
	if !ok {
		return
	}
	l.logger.Error(msg, args[1:]...)
}

func (m *recovererMiddleware) Middleware() server.Middleware {
	return func(next server.HandlerFunc) server.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) (err error) {
			defer func() {
				if rec := recover(); rec != nil {
					var buf []byte
					func() {
						defer func() { _ = recover() }()
						buf = debug.Stack()
					}()
					if m.logger != nil {
						m.logger.Error(
							"panic recovered",
							"error", fmt.Sprintf("%v", rec),
							"stack", string(buf),
							"method", r.Method,
							"path", r.URL.Path,
						)
					} else {
						log.Printf("panic recovered: %v\n%s", rec, buf)
					}
					err = server.NewHTTPError(http.StatusInternalServerError, "Internal Server Error")
				}
			}()
			return next(w, r)
		}
	}
}
