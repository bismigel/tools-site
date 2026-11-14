package web

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/bismigel/tools-site/internal/tool"
)

// Config is what main wires up. Auth is nil until M3b: with no verifier, the
// gated routes are not mounted at all (spec FR-4.3), so /lab* is a 404 rather
// than an unauthenticated page.
type Config struct {
	Registry  *tool.Registry
	BaseURL   string
	Dev       bool
	Access    *Verifier
	Templates fs.FS // paths start at the repo root: web/templates/...
	Static    fs.FS // rooted at web/static
	CSS       []byte
	// CSSFile, when set in dev mode, is read on every request so an edited
	// stylesheet appears on refresh without restarting the server.
	CSSFile string
	Log     *slog.Logger
	Timeout time.Duration
}

// Server is the whole HTTP surface.
type Server struct {
	cfg          Config
	assetVersion string
	log          *slog.Logger
	r            *Renderer
	mux          http.Handler
	now          func() time.Time
	metrics      *metrics
}

// New validates the configuration and builds the mux. It fails loudly rather
// than serving a half-built site.
func New(cfg Config) (*Server, error) {
	if cfg.Registry == nil {
		return nil, errors.New("web: nil registry")
	}
	if cfg.Templates == nil {
		return nil, errors.New("web: nil templates fs")
	}
	if cfg.Static == nil {
		return nil, errors.New("web: nil static fs")
	}
	if len(cfg.CSS) == 0 {
		return nil, errors.New("web: empty css (run `make css`)")
	}
	if cfg.Log == nil {
		cfg.Log = slog.Default()
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 5 * time.Second
	}
	s := &Server{
		cfg: cfg, log: cfg.Log, r: NewRenderer(cfg.Templates),
		now: time.Now, metrics: newMetrics(), assetVersion: assetVersion(cfg),
	}
	s.mux = s.routes()
	return s, nil
}

// assetVersion fingerprints the stylesheet and every static file. It is appended
// to asset URLs because they are served immutable for a year: without a content
// hash, a CSS change never reaches a browser or the CDN. Found the hard way on
// the first post-deploy style change, when Cloudflare kept serving the old file.
func assetVersion(cfg Config) string {
	h := sha256.New()
	h.Write(cfg.CSS)
	_ = fs.WalkDir(cfg.Static, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		b, rerr := fs.ReadFile(cfg.Static, p)
		if rerr != nil {
			return nil
		}
		fmt.Fprintf(h, "|%s|%d|", p, len(b))
		h.Write(b)
		return nil
	})
	return hex.EncodeToString(h.Sum(nil))[:10]
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

func (s *Server) routes() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /{$}", s.indexFor(tool.Public))
	mux.HandleFunc("GET /t/{name}", s.toolPageFor(tool.Public))
	mux.HandleFunc("POST /t/{name}", s.toolRunFor(tool.Public))
	mux.HandleFunc("GET /api/tools", s.manifestFor(tool.Public))
	mux.HandleFunc("POST /api/tools/{name}", s.apiRunFor(tool.Public))

	mux.HandleFunc("GET /healthz", s.healthz)
	mux.HandleFunc("GET /readyz", s.readyz)
	mux.HandleFunc("GET /robots.txt", s.robots)
	mux.HandleFunc("GET /static/css/tools.css", s.stylesheet)
	mux.HandleFunc("GET /static/", s.static)
	mux.HandleFunc("GET /metrics", s.metricsHandler)
	mux.HandleFunc("POST /mcp", s.mcp)
	mux.HandleFunc("/mcp", methodNotAllowed)
	mux.HandleFunc("GET /dev/reload", s.devReload)

	// The gated zone exists only when a verifier is configured (FR-4.3).
	if s.cfg.Access != nil {
		auth := s.cfg.Access.Middleware
		mux.Handle("GET /lab", auth(http.HandlerFunc(s.indexFor(tool.Gated))))
		mux.Handle("GET /lab/{name}", auth(http.HandlerFunc(s.toolPageFor(tool.Gated))))
		mux.Handle("POST /lab/{name}", auth(http.HandlerFunc(s.toolRunFor(tool.Gated))))
		mux.Handle("GET /api/lab", auth(http.HandlerFunc(s.manifestFor(tool.Gated))))
		mux.Handle("POST /api/lab/{name}", auth(http.HandlerFunc(s.apiRunFor(tool.Gated))))
	}

	mux.HandleFunc("/", s.notFound)

	return s.withRequestID(s.withLog(s.withRecover(s.withHeaders(mux))))
}

func (s *Server) notFound(w http.ResponseWriter, r *http.Request) {
	p := s.basePage()
	p.Title = "Not found"
	p.Tools = s.cfg.Registry.All(tool.Public)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusNotFound)
	if err := s.r.Page(w, "404.html", p); err != nil {
		s.log.Error("render 404", "err", err)
	}
}

// methodNotAllowed answers a known path with the wrong verb (e.g. GET /mcp).
func methodNotAllowed(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Allow", "POST")
	http.Error(w, "Method not allowed.", http.StatusMethodNotAllowed)
}

func (s *Server) metricsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	s.metrics.write(w)
}

func (s *Server) healthz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte("ok\n"))
}

// readyz is allowed to touch more than healthz, but must not block on the
// network: everything checked here is in-process.
func (s *Server) readyz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if s.cfg.Registry.Len() == 0 || len(s.cfg.CSS) == 0 {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte("not ready\n"))
		return
	}
	_, _ = w.Write([]byte("ok\n"))
}

func (s *Server) robots(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = fmt.Fprint(w, "User-agent: *\nAllow: /\n")
}

// stylesheet serves the generated CSS. It lives at theme/tools.css, which is not
// under web/static, so it gets its own route rather than a copy on disk.
func (s *Server) stylesheet(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/css; charset=utf-8")
	w.Header().Set("Cache-Control", s.cacheControl())
	if s.cfg.Dev && s.cfg.CSSFile != "" {
		if data, err := os.ReadFile(s.cfg.CSSFile); err == nil {
			_, _ = w.Write(data)
			return
		}
	}
	_, _ = w.Write(s.cfg.CSS)
}

// cacheControl keeps prod assets immutable and makes dev edits show on refresh.
func (s *Server) cacheControl() string {
	if s.cfg.Dev {
		return "no-store"
	}
	return "public, max-age=31536000, immutable"
}

func (s *Server) static(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", s.cacheControl())
	http.StripPrefix("/static/", http.FileServerFS(s.cfg.Static)).ServeHTTP(w, r)
}

// devReload is the dev-only SSE stream: when a template changes on disk, the
// page reloads. Unreachable without -dev (NFR-8.3) and tested as such.
func (s *Server) devReload(w http.ResponseWriter, r *http.Request) {
	if !s.cfg.Dev {
		s.notFound(w, r)
		return
	}
	fl, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	fl.Flush()

	last := s.templateStamp()
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-tick.C:
			if stamp := s.templateStamp(); stamp != last {
				last = stamp
				_, _ = fmt.Fprint(w, "event: reload\ndata: 1\n\n")
			} else {
				_, _ = fmt.Fprint(w, ": keepalive\n\n")
			}
			fl.Flush()
		}
	}
}

func (s *Server) templateStamp() int64 {
	var sum int64
	_ = fs.WalkDir(s.cfg.Templates, "web/templates", func(_ string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if info, e := d.Info(); e == nil {
			sum += info.ModTime().UnixNano()
		}
		return nil
	})
	return sum
}
