package plugins

import (
	"context"
	"net/http"

	"github.com/google/uuid"
	"github.com/webdeveloperben/tyche/server"
)

type RequestIDConfig struct {
	HeaderName string
}

func RequestID(cfg ...RequestIDConfig) server.Middleware {
	header := "X-Request-ID"
	if len(cfg) > 0 && cfg[0].HeaderName != "" {
		header = cfg[0].HeaderName
	}
	m := &requestIDMiddleware{headerName: header}
	return m.Middleware()
}

func RequestIDWithDefaults() server.Middleware {
	m := &requestIDMiddleware{headerName: "X-Request-ID"}
	return m.Middleware()
}

type requestIDMiddleware struct {
	headerName string
}

func (m *requestIDMiddleware) Register(r *server.API) {
	r.Use(m.Middleware())
}

func (m *requestIDMiddleware) Middleware() server.Middleware {
	header := m.headerName
	return func(next server.HandlerFunc) server.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) error {
			requestID := r.Header.Get(header)
			if requestID == "" {
				requestID = uuid.New().String()
			}

			w.Header().Set(header, requestID)

			ctx := context.WithValue(r.Context(), requestIDKeyGlobal, requestID)
			return next(w, r.WithContext(ctx))
		}
	}
}

type requestIDKey struct{}

var requestIDKeyGlobal = &requestIDKey{}

func RequestIDFromContext(ctx context.Context) string {
	if id, ok := ctx.Value(requestIDKeyGlobal).(string); ok {
		return id
	}
	return ""
}
