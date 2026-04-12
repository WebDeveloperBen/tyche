package plugins

import (
	"net"
	"net/http"
	"strings"

	"github.com/webdeveloperben/tyche/server"
)

type RealIPConfig struct {
	TrustedProxies []string
}

func RealIP(cfg ...RealIPConfig) server.Middleware {
	trusted := defaultTrustedProxies()
	if len(cfg) > 0 {
		trusted = cfg[0].TrustedProxies
	}
	m := newRealIPMiddleware(trusted)
	return m.Middleware()
}

func RealIPWithDefaults() server.Middleware {
	return RealIP(RealIPConfig{})
}

var defaultProxies = []string{"127.0.0.1", "::1", "10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16"}

func defaultTrustedProxies() []string {
	return append([]string(nil), defaultProxies...)
}

func newRealIPMiddleware(trustedProxies []string) *realIPMiddleware {
	m := &realIPMiddleware{
		trustedCIDRs: make([]*net.IPNet, 0, len(trustedProxies)),
	}

	for _, proxy := range trustedProxies {
		if strings.Contains(proxy, "/") {
			_, cidr, err := net.ParseCIDR(proxy)
			if err == nil {
				m.trustedCIDRs = append(m.trustedCIDRs, cidr)
			}
		} else {
			if ip := net.ParseIP(proxy); ip != nil {
				m.trustedCIDRs = append(m.trustedCIDRs, &net.IPNet{IP: ip, Mask: net.CIDRMask(128, 128)})
			}
		}
	}

	return m
}

type realIPMiddleware struct {
	trustedCIDRs []*net.IPNet
}

func (m *realIPMiddleware) Register(r *server.Router) {
	r.Use(m.Middleware())
}

func (m *realIPMiddleware) Middleware() server.Middleware {
	return func(next server.HandlerFunc) server.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) error {
			if len(m.trustedCIDRs) > 0 {
				rip := extractRealIP(r, m.trustedCIDRs)
				if rip != "" {
					r.Header.Set("X-Real-IP", rip)
				}
			}
			return next(w, r)
		}
	}
}

func extractRealIP(r *http.Request, trustedCIDRs []*net.IPNet) string {
	xff := r.Header.Get("X-Forwarded-For")
	if xff == "" {
		return ""
	}

	parts := strings.Split(xff, ",")
	for i := range parts {
		ipStr := strings.TrimSpace(parts[len(parts)-1-i])
		if ipStr == "" {
			continue
		}

		ip := net.ParseIP(ipStr)
		if ip == nil {
			return ipStr
		}

		isTrusted := false
		for _, cidr := range trustedCIDRs {
			if cidr.Contains(ip) {
				isTrusted = true
				break
			}
		}

		if !isTrusted {
			return ipStr
		}
	}

	return ""
}
