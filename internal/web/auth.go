package web

import (
	"crypto/subtle"
	"net"
	"net/http"
	"strings"
)

// isLoopbackBind reports whether the bind address points at the local
// machine. Used to decide whether --token is mandatory at startup.
// IPv6 ::1 and the wildcard form ::1/128 are normalized via net.ParseIP.
func isLoopbackBind(addr string) bool {
	if addr == "" || addr == "localhost" {
		return true
	}
	ip := net.ParseIP(addr)
	if ip == nil {
		return false
	}
	return ip.IsLoopback()
}

// bearerAuthMiddleware enforces token auth for non-loopback requests.
//
// Loopback requests (127.0.0.1, ::1) bypass the check entirely so local
// dev, the auto-launched browser, and SSH tunnels keep working without
// having to pass the token around. When a request arrives over a real
// network and a token is configured, we require an exact-match
// Authorization header (constant-time compared) or a ?token= query
// parameter for browser-driven download flows like /api/memory/export.
//
// If token is empty AND the request isn't loopback we 401 — the
// startup check in Start() refuses to bind to a non-loopback address
// without a token, so this is defensive.
func bearerAuthMiddleware(token string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isLoopbackRequest(r) {
			next.ServeHTTP(w, r)
			return
		}
		if token == "" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if !validBearer(r, token) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func isLoopbackRequest(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return false
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func validBearer(r *http.Request, token string) bool {
	if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
		got := strings.TrimPrefix(h, "Bearer ")
		if subtle.ConstantTimeCompare([]byte(got), []byte(token)) == 1 {
			return true
		}
	}
	// Query-string fallback for download flows (e.g. <a href="/api/memory/export?token=…">).
	// Browsers can't easily send custom Authorization headers from a
	// plain link click, so this gives a workable escape hatch.
	if got := r.URL.Query().Get("token"); got != "" {
		if subtle.ConstantTimeCompare([]byte(got), []byte(token)) == 1 {
			return true
		}
	}
	return false
}
