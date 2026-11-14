package main

import (
	"context"
	"errors"
	"flag"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	site "github.com/bismigel/tools-site"
	"github.com/bismigel/tools-site/internal/fx"
	"github.com/bismigel/tools-site/internal/tool"
	"github.com/bismigel/tools-site/internal/web"
)

func main() {
	addr := flag.String("addr", ":8080", "listen address")
	baseURL := flag.String("base-url", "", "public base URL, used for origin checks")
	dev := flag.Bool("dev", false, "serve templates/static from disk; enables /dev/reload")
	flag.Parse()

	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(log)

	// NFR-8.2: a dev build must never be deployable by accident.
	if *dev && os.Getenv("KUBERNETES_SERVICE_HOST") != "" {
		log.Error("refusing to start: -dev inside Kubernetes")
		os.Exit(1)
	}
	if *baseURL == "" && *dev {
		*baseURL = "http://localhost" + *addr
	}

	rates, err := fx.Embedded()
	if err != nil {
		log.Error("embedded exchange rates are invalid", "err", err)
		os.Exit(1)
	}

	reg := tool.NewRegistry()
	register := func(t tool.Tool) {
		if err := reg.Register(t); err != nil {
			log.Error("invalid tool definition", "err", err)
			os.Exit(1)
		}
	}
	register(tool.NewSlugify())
	register(tool.NewRateConverter(rates))
	register(tool.NewMeetingPlanner())

	access, err := buildAccess(log)
	if err != nil {
		log.Error("access configuration", "err", err)
		os.Exit(1)
	}

	cfg := web.Config{Registry: reg, BaseURL: *baseURL, Dev: *dev, Log: log, Access: access}
	if *dev {
		cfg.Templates = os.DirFS(".")
		cfg.Static = os.DirFS("web/static")
		css, err := os.ReadFile("theme/tools.css")
		if err != nil {
			log.Error("read theme/tools.css (run `make css`)", "err", err)
			os.Exit(1)
		}
		cfg.CSS = css
		cfg.CSSFile = "theme/tools.css" // dev: re-read per request, no restart
	} else {
		static, err := fs.Sub(site.Web, "web/static")
		if err != nil {
			log.Error("static sub fs", "err", err)
			os.Exit(1)
		}
		cfg.Templates = site.Web
		cfg.Static = static
		cfg.CSS = site.ThemeCSS
	}

	srv, err := web.New(cfg)
	if err != nil {
		log.Error("server", "err", err)
		os.Exit(1)
	}

	httpSrv := &http.Server{
		Addr:              *addr,
		Handler:           srv,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	if !*dev {
		// A write timeout would cut the dev SSE stream; there is no such
		// stream in production, so keep the protection there.
		httpSrv.WriteTimeout = 30 * time.Second
	}

	go func() {
		log.Info("listening", "addr", *addr, "dev", *dev, "tools", reg.Len())
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("listen", "err", err)
			os.Exit(1)
		}
	}()

	// Graceful shutdown: stop accepting, drain up to 10 s (FR-6.4).
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := httpSrv.Shutdown(ctx); err != nil {
		log.Error("shutdown", "err", err)
		os.Exit(1)
	}
	log.Info("stopped")
}

// buildAccess decides whether the gated zone exists at all (FR-4.3). With no
// team domain/audience the routes are never mounted, which is safer than
// mounting them and hoping.
func buildAccess(log *slog.Logger) (*web.Verifier, error) {
	team := os.Getenv("CF_ACCESS_TEAM_DOMAIN")
	aud := os.Getenv("CF_ACCESS_AUD")

	if os.Getenv("TOOLS_DEV_INSECURE") == "1" {
		log.Warn("TOOLS_DEV_INSECURE=1: Access JWTs are NOT verified — development only")
		return web.NewVerifier(web.AccessConfig{Bypass: true})
	}
	if team == "" || aud == "" {
		log.Warn("CF_ACCESS_TEAM_DOMAIN/CF_ACCESS_AUD unset: gated routes are not mounted")
		return nil, nil
	}
	return web.NewVerifier(web.AccessConfig{TeamDomain: team, Audience: aud})
}
