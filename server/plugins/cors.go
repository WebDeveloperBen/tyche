package plugins

import (
	"errors"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/webdeveloperben/tyche/server"
)

var ErrCORSWildcardWithCredentials = errors.New("CORS: wildcard origin with AllowCredentials is invalid; credentials require specific origin")

type CORSConfig struct {
	AllowedOrigins     []string
	AllowedMethods     []string
	AllowedHeaders     []string
	ExposedHeaders     []string
	AllowCredentials   bool
	MaxAge             int
	AllowOriginFunc    func(r *http.Request, origin string) bool
	OptionsPassthrough bool
}

type corsMiddleware struct {
	allowedOrigins     map[string]struct{}
	allowedWOrigins    []wildcard
	allowedHeaders     map[string]struct{}
	allowedMethods     map[string]struct{}
	exposedHeaders     string
	maxAge             int
	allowCredentials   bool
	allowOriginsAll    bool
	allowHeadersAll    bool
	allowOriginFunc    func(r *http.Request, origin string) bool
	optionsPassthrough bool
}

type wildcard struct {
	prefix, suffix string
}

func (w wildcard) match(s string) bool {
	return len(s) >= len(w.prefix)+len(w.suffix) && strings.HasPrefix(s, w.prefix) && strings.HasSuffix(s, w.suffix)
}

func CORS(cfg ...CORSConfig) (server.ServeHTTPMiddleware, error) {
	c := CORSConfig{}
	if len(cfg) > 0 {
		c = cfg[0]
	}
	return newCORS(c)
}

func newCORS(c CORSConfig) (server.ServeHTTPMiddleware, error) {
	if c.AllowCredentials {
		hasWildcard := slices.Contains(c.AllowedOrigins, "*")
		if hasWildcard {
			return nil, ErrCORSWildcardWithCredentials
		}
	}

	m := &corsMiddleware{
		allowCredentials:   c.AllowCredentials,
		maxAge:             c.MaxAge,
		allowOriginFunc:    c.AllowOriginFunc,
		optionsPassthrough: c.OptionsPassthrough,
	}

	if len(c.AllowedOrigins) == 0 && c.AllowOriginFunc == nil {
		m.allowOriginsAll = true
	} else if len(c.AllowedOrigins) > 0 {
		m.allowedOrigins = make(map[string]struct{}, len(c.AllowedOrigins))
		for _, origin := range c.AllowedOrigins {
			originLower := strings.ToLower(origin)
			if originLower == "*" {
				m.allowOriginsAll = true
				m.allowedOrigins = nil
				m.allowedWOrigins = nil
				break
			}
			if before, after, ok := strings.Cut(originLower, "*"); ok {
				m.allowedWOrigins = append(m.allowedWOrigins, wildcard{before, after})
			} else {
				m.allowedOrigins[originLower] = struct{}{}
			}
		}
	}

	if len(c.AllowedHeaders) == 0 {
		m.allowedHeaders = map[string]struct{}{
			"Origin":       {},
			"Accept":       {},
			"Content-Type": {},
		}
	} else {
		m.allowedHeaders = make(map[string]struct{}, len(c.AllowedHeaders)+1)
		for _, h := range c.AllowedHeaders {
			if h == "*" {
				m.allowHeadersAll = true
				m.allowedHeaders = nil
				break
			}
			m.allowedHeaders[http.CanonicalHeaderKey(h)] = struct{}{}
		}
		if !m.allowHeadersAll {
			m.allowedHeaders["Origin"] = struct{}{}
		}
	}

	if len(c.AllowedMethods) == 0 {
		m.allowedMethods = map[string]struct{}{
			http.MethodGet:  {},
			http.MethodPost: {},
			http.MethodHead: {},
		}
	} else {
		m.allowedMethods = make(map[string]struct{}, len(c.AllowedMethods)+1)
		for _, method := range c.AllowedMethods {
			m.allowedMethods[strings.ToUpper(method)] = struct{}{}
		}
		m.allowedMethods[http.MethodOptions] = struct{}{}
	}

	if len(c.ExposedHeaders) > 0 {
		m.exposedHeaders = strings.Join(canonicalHeaders(c.ExposedHeaders), ", ")
	}

	return m.Middleware(), nil
}

func CORSWithDefaults() server.ServeHTTPMiddleware {
	mw, err := CORS(CORSConfig{
		AllowedOrigins: []string{"https://example.com"},
		AllowedMethods: []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch, http.MethodOptions},
		AllowedHeaders: []string{"Accept", "Authorization", "Content-Type", "X-Requested-With"},
		MaxAge:         86400,
	})
	if err != nil {
		panic(err)
	}
	return mw
}

func (m *corsMiddleware) Register(r *server.Router) {
	r.UseServeHTTP(m.Middleware())
}

func (m *corsMiddleware) Middleware() server.ServeHTTPMiddleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				}
			}()

			_, hasOrigin := r.Header["Origin"]

			if r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != "" && hasOrigin {
				m.handlePreflight(w, r)
				if m.optionsPassthrough {
					next.ServeHTTP(w, r)
				}
				return
			}

			m.handleActualRequest(w, r)
			next.ServeHTTP(w, r)
		})
	}
}

func (m *corsMiddleware) handlePreflight(w http.ResponseWriter, r *http.Request) {
	h := w.Header()
	origin := r.Header.Get("Origin")

	h.Set("Vary", "Origin, Access-Control-Request-Method, Access-Control-Request-Headers")

	if !m.isOriginAllowed(r, origin) {
		return
	}

	method := r.Header.Get("Access-Control-Request-Method")

	if m.allowOriginsAll && !m.allowCredentials {
		h.Set("Access-Control-Allow-Origin", "*")
	} else {
		h.Set("Access-Control-Allow-Origin", origin)
	}

	if m.isMethodAllowed(method) {
		h.Set("Access-Control-Allow-Methods", strings.ToUpper(method))
	}

	reqHeaders := parseHeaderList(r.Header.Get("Access-Control-Request-Headers"))
	if len(reqHeaders) > 0 {
		if !m.areHeadersAllowed(reqHeaders) {
			return
		}
		h.Set("Access-Control-Allow-Headers", strings.Join(reqHeaders, ", "))
	}

	if m.allowCredentials {
		h.Set("Access-Control-Allow-Credentials", "true")
	}

	if m.maxAge > 0 {
		h.Set("Access-Control-Max-Age", strconv.Itoa(m.maxAge))
	}

	w.WriteHeader(http.StatusNoContent)
}

func (m *corsMiddleware) handleActualRequest(w http.ResponseWriter, r *http.Request) {
	h := w.Header()
	_, hasOrigin := r.Header["Origin"]

	h.Add("Vary", "Origin")

	if !hasOrigin {
		return
	}

	origin := r.Header.Get("Origin")
	if !m.isOriginAllowed(r, origin) {
		return
	}

	if m.allowOriginsAll && !m.allowCredentials {
		h.Set("Access-Control-Allow-Origin", "*")
	} else {
		h.Set("Access-Control-Allow-Origin", origin)
	}

	if m.exposedHeaders != "" {
		h.Set("Access-Control-Expose-Headers", m.exposedHeaders)
	}

	if m.allowCredentials {
		h.Set("Access-Control-Allow-Credentials", "true")
	}
}

func (m *corsMiddleware) isOriginAllowed(r *http.Request, origin string) bool {
	if m.allowOriginFunc != nil {
		return m.allowOriginFunc(r, origin)
	}
	if m.allowOriginsAll {
		return true
	}
	if m.allowedOrigins != nil {
		if _, ok := m.allowedOrigins[strings.ToLower(origin)]; ok {
			return true
		}
	}
	for _, w := range m.allowedWOrigins {
		if w.match(origin) {
			return true
		}
	}
	return false
}

func (m *corsMiddleware) isMethodAllowed(method string) bool {
	if m.allowedMethods == nil {
		return false
	}
	_, ok := m.allowedMethods[strings.ToUpper(method)]
	return ok
}

func (m *corsMiddleware) areHeadersAllowed(requested []string) bool {
	if m.allowHeadersAll || len(requested) == 0 {
		return true
	}
	for _, h := range requested {
		canonical := http.CanonicalHeaderKey(h)
		if _, ok := m.allowedHeaders[canonical]; !ok {
			return false
		}
	}
	return true
}

func canonicalHeaders(headers []string) []string {
	out := make([]string, 0, len(headers))
	for _, h := range headers {
		out = append(out, http.CanonicalHeaderKey(h))
	}
	return out
}

func parseHeaderList(headerList string) []string {
	if headerList == "" {
		return nil
	}
	l := len(headerList)
	headers := make([]string, 0, 4)
	h := make([]byte, 0, l)
	for i := range l {
		b := headerList[i]
		if b >= 'A' && b <= 'Z' {
			h = append(h, b)
		} else if b >= 'a' && b <= 'z' {
			h = append(h, b-32)
		} else if b == '-' || b == '_' || b == '.' || (b >= '0' && b <= '9') {
			h = append(h, b)
		}
		if b == ' ' || b == ',' || i == l-1 {
			if len(h) > 0 {
				headers = append(headers, string(h))
				h = h[:0]
			}
		}
	}
	return headers
}
