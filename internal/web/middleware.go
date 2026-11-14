package web

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net/http"
	"runtime/debug"
	"strings"
	"time"
)

type ctxKey int

const reqIDKey ctxKey = iota

// reqID is the CF-Ray when present, otherwise a generated id. It is echoed in
// error bodies and every log line (spec §7.7).
func reqID(r *http.Request) string {
	if v, ok := r.Context().Value(reqIDKey).(string); ok {
		return v
	}
	return ""
}

func newID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "unknown"
	}
	return hex.EncodeToString(b[:])
}

func (s *Server) withRequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("Cf-Ray")
		if id == "" {
			id = newID()
		}
		r = r.WithContext(context.WithValue(r.Context(), reqIDKey, id))
		w.Header().Set("X-Request-Id", id)
		next.ServeHTTP(w, r)
	})
}

// pathTemplate keeps user text out of logs: only fixed routes and the
// documented path shapes are logged (spec §7.7).
func pathTemplate(p string) string {
	switch {
	case p == "/":
		return "/"
	case strings.HasPrefix(p, "/t/"):
		return "/t/{name}"
	case strings.HasPrefix(p, "/api/tools/"):
		return "/api/tools/{name}"
	case strings.HasPrefix(p, "/lab/"):
		return "/lab/{name}"
	case strings.HasPrefix(p, "/api/lab/"):
		return "/api/lab/{name}"
	case strings.HasPrefix(p, "/static/"):
		return "/static/*"
	case strings.HasPrefix(p, "/dev/"):
		return "/dev/*"
	case p == "/api/tools", p == "/api/lab", p == "/lab",
		p == "/healthz", p == "/readyz", p == "/robots.txt", p == "/metrics":
		return p
	default:
		return "unmatched"
	}
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

// Flush lets the dev SSE stream flush through the wrapper.
func (w *statusWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (s *Server) withLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sw, r)
		s.metrics.observe(pathTemplate(r.URL.Path), sw.status, time.Since(start))
		s.log.Info("request",
			slog.String("request_id", reqID(r)),
			slog.String("method", r.Method),
			slog.String("path", pathTemplate(r.URL.Path)),
			slog.Int("status", sw.status),
			slog.Int64("duration_ms", time.Since(start).Milliseconds()),
			slog.String("cf_ray", r.Header.Get("Cf-Ray")),
		)
	})
}

func (s *Server) withRecover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				s.log.Error("panic",
					slog.String("request_id", reqID(r)),
					slog.String("path", pathTemplate(r.URL.Path)),
					slog.Any("panic", rec),
					slog.String("stack", string(debug.Stack())),
				)
				if sw, ok := w.(*statusWriter); !ok || sw.status == http.StatusOK {
					http.Error(w, "Something went wrong.", http.StatusInternalServerError)
				}
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// securityHeaders are the exact set from spec §7.5. No inline script or style
// is allowed anywhere, which is why templates carry no style= attributes.
func (s *Server) withHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy",
			"default-src 'none'; script-src 'self'; style-src 'self'; img-src 'self' data:; "+
				"font-src 'self'; connect-src 'self'; form-action 'self'; base-uri 'none'; "+
				"frame-ancestors 'none'")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		h.Set("Cross-Origin-Opener-Policy", "same-origin")
		h.Set("Cross-Origin-Resource-Policy", "same-origin")
		h.Set("Permissions-Policy", "geolocation=(), microphone=(), camera=()")
		next.ServeHTTP(w, r)
	})
}

func writeJSONBody(w http.ResponseWriter, v any) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(true)
	return enc.Encode(v)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = writeJSONBody(w, v)
}

// sameOrigin enforces the CSRF-adjacent origin rule for POSTs (FR-4.5). The
// public zone has no ambient credential, so no token is required; browsers
// still must not be able to post here from another site.
func (s *Server) sameOrigin(r *http.Request) bool {
	if r.Header.Get("Sec-Fetch-Site") == "same-origin" {
		return true
	}
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true // curl, MCP clients, server-to-server: no browser, no ambient auth
	}
	if s.cfg.BaseURL == "" {
		return false
	}
	return strings.EqualFold(
		strings.TrimRight(origin, "/"),
		strings.TrimRight(s.cfg.BaseURL, "/"),
	)
}
