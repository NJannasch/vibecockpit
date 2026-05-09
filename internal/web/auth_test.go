package web

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestIsLoopbackBind(t *testing.T) {
	cases := map[string]bool{
		"":            true,
		"localhost":   true,
		"127.0.0.1":   true,
		"127.0.0.42":  true,
		"::1":         true,
		"0.0.0.0":     false,
		"192.168.1.5": false,
		"10.0.0.1":    false,
	}
	for in, want := range cases {
		if got := isLoopbackBind(in); got != want {
			t.Errorf("isLoopbackBind(%q) = %v; want %v", in, got, want)
		}
	}
}

func newAuthRequest(remote string, header string, query string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "/api/sessions"+query, nil)
	r.RemoteAddr = remote
	if header != "" {
		r.Header.Set("Authorization", header)
	}
	return r
}

func TestBearerMiddleware_LoopbackBypass(t *testing.T) {
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(200)
	})
	h := bearerAuthMiddleware("the-secret", next)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, newAuthRequest("127.0.0.1:54321", "", ""))
	if rec.Code != 200 || !called {
		t.Errorf("loopback should bypass token; got code=%d called=%v", rec.Code, called)
	}
}

func TestBearerMiddleware_RemoteRequiresToken(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	})
	h := bearerAuthMiddleware("the-secret", next)

	cases := []struct {
		name   string
		header string
		query  string
		want   int
	}{
		{"no header", "", "", 401},
		{"wrong scheme", "Basic abc", "", 401},
		{"wrong token", "Bearer wrong", "", 401},
		{"correct token", "Bearer the-secret", "", 200},
		{"query token correct", "", "?token=the-secret", 200},
		{"query token wrong", "", "?token=nope", 401},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, newAuthRequest("10.0.0.1:54321", c.header, c.query))
			if rec.Code != c.want {
				body, _ := io.ReadAll(rec.Body)
				t.Errorf("got code=%d body=%q; want %d", rec.Code, string(body), c.want)
			}
		})
	}
}

func TestBearerMiddleware_NoTokenConfigured_Remote401(t *testing.T) {
	// Defensive case: even if Start() fails to refuse a non-loopback bind
	// without a token, the middleware itself must still 401 remote
	// requests rather than silently allowing them through.
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	})
	h := bearerAuthMiddleware("", next)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, newAuthRequest("10.0.0.1:54321", "", ""))
	if rec.Code != 401 {
		t.Errorf("expected 401 when no token configured + remote request; got %d", rec.Code)
	}
}
