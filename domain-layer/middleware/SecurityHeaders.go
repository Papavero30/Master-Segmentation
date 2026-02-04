package middleware

import (
	"net/http"
	"strings"
)

type SecurityHeaders struct{}

func NewSecurityHeaders() *SecurityHeaders {
	return &SecurityHeaders{}
}

func (s *SecurityHeaders) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-XSS-Protection", "1; mode=block")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")

		hostLower := strings.ToLower(r.Host)
		if r.TLS != nil && !strings.Contains(hostLower, "localhost") && !strings.Contains(hostLower, "127.0.0.1") {
			w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains; preload")
		}

		w.Header().Set("Content-Security-Policy", "default-src 'self'; connect-src 'self' http://localhost:* https://localhost:*; img-src 'self' data: blob:; style-src 'self' 'unsafe-inline'; script-src 'self' 'unsafe-inline' 'unsafe-eval'")
		w.Header().Set("Server", "BrainNav-API")

		if r.URL.Path != "/health" && r.URL.Path != "/debug" {
			w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, proxy-revalidate")
			w.Header().Set("Pragma", "no-cache")
			w.Header().Set("Expires", "0")
		}

		if next != nil {
			next.ServeHTTP(w, r)
		}
	})
}
