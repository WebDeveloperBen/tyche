package plugins

import (
	"net/http"

	"github.com/webdeveloperben/tyche/server"
)

type SecurityConfig struct {
	XFrameOptions         string
	XContentTypeOptions   string
	XSSProtection         string
	ContentTypeOptions    string
	ReferrerPolicy        string
	PermissionsPolicy     string
	CrossDomainPolicy     string
	HSTSMaxAge            int
	HSTSIncludeSubdomains bool
	HSTSPreload           bool
}

func Security(cfg ...SecurityConfig) server.ServeHTTPMiddleware {
	c := SecurityConfig{}
	if len(cfg) > 0 {
		c = cfg[0]
	}

	if c.XFrameOptions == "" {
		c.XFrameOptions = "DENY"
	}
	if c.XContentTypeOptions == "" {
		c.XContentTypeOptions = "nosniff"
	}
	if c.ContentTypeOptions == "" {
		c.ContentTypeOptions = "nosniff"
	}
	if c.ReferrerPolicy == "" {
		c.ReferrerPolicy = "strict-origin-when-cross-origin"
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := w.Header()

			if c.XFrameOptions != "" {
				h.Set("X-Frame-Options", c.XFrameOptions)
			}
			if c.XContentTypeOptions != "" {
				h.Set("X-Content-Type-Options", c.XContentTypeOptions)
			}
			if c.XSSProtection != "" {
				h.Set("X-XSS-Protection", c.XSSProtection)
			}
			if c.ReferrerPolicy != "" {
				h.Set("Referrer-Policy", c.ReferrerPolicy)
			}
			if c.PermissionsPolicy != "" {
				h.Set("Permissions-Policy", c.PermissionsPolicy)
			}
			if c.CrossDomainPolicy != "" {
				h.Set("X-Download-Options", c.CrossDomainPolicy)
			}

			if c.HSTSMaxAge > 0 || (len(cfg) == 0) {
				maxAge := c.HSTSMaxAge
				if maxAge == 0 {
					maxAge = 31536000
				}
				hstsValue := "max-age=" + itoa(maxAge)
				if c.HSTSIncludeSubdomains {
					hstsValue += "; includeSubDomains"
				}
				if c.HSTSPreload {
					hstsValue += "; preload"
				}
				h.Set("Strict-Transport-Security", hstsValue)
			}

			next.ServeHTTP(w, r)
		})
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var result []byte
	for n > 0 {
		result = append([]byte{byte('0' + n%10)}, result...)
		n /= 10
	}
	return string(result)
}

func SecurityWithDefaults() server.ServeHTTPMiddleware {
	return Security(SecurityConfig{})
}

func SecurityMiddleware(cfg ...SecurityConfig) server.ServeHTTPMiddleware {
	return Security(cfg...)
}
