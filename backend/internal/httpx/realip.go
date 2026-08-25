package httpx

import (
	"net"
	"net/http"
	"strings"
)

// ClientIP resolves the caller's address. Proxy headers are only consulted
// when the deployment sits behind a reverse proxy, because otherwise any
// client could forge them and sidestep rate limiting entirely.
func ClientIP(r *http.Request, trustProxy bool) string {
	if trustProxy {
		if ip := firstValidIP(r.Header.Get("X-Forwarded-For")); ip != "" {
			return ip
		}
		if ip := firstValidIP(r.Header.Get("X-Real-IP")); ip != "" {
			return ip
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func firstValidIP(header string) string {
	for _, part := range strings.Split(header, ",") {
		candidate := strings.TrimSpace(part)
		if candidate == "" {
			continue
		}
		if host, _, err := net.SplitHostPort(candidate); err == nil {
			candidate = host
		}
		candidate = strings.Trim(candidate, "[]")
		if net.ParseIP(candidate) != nil {
			return candidate
		}
	}
	return ""
}
