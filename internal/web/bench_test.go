package web

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/bismigel/tools-site/internal/fx"
	"github.com/bismigel/tools-site/internal/tool"
)

// NFR-1.1: the HTML page must stay under 50 ms p95, template parse included —
// the renderer parses the set per request on purpose (it removes the dev/prod
// branch), so the cost of that decision is measured rather than assumed.

func benchServer(b *testing.B) *Server {
	b.Helper()
	css, err := os.ReadFile("../../theme/tools.css")
	if err != nil {
		b.Fatalf("run `make css`: %v", err)
	}
	rates, err := fx.Embedded()
	if err != nil {
		b.Fatal(err)
	}
	reg := tool.NewRegistry()
	for _, t := range []tool.Tool{tool.NewSlugify(), tool.NewRateConverter(rates), tool.NewMeetingPlanner()} {
		if err := reg.Register(t); err != nil {
			b.Fatal(err)
		}
	}
	srv, err := New(Config{
		Registry:  reg,
		BaseURL:   "https://tools.example",
		Templates: os.DirFS("../.."),
		Static:    os.DirFS("../../web/static"),
		CSS:       css,
		Log:       slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		b.Fatal(err)
	}
	return srv
}

func benchmarkPage(b *testing.B, path string) {
	srv := benchServer(b)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusOK {
			b.Fatalf("%s = %d", path, rec.Code)
		}
	}
}

func BenchmarkIndexPage(b *testing.B) { benchmarkPage(b, "/") }
func BenchmarkToolForm(b *testing.B)  { benchmarkPage(b, "/t/rate_converter") }
func BenchmarkToolResult(b *testing.B) {
	benchmarkPage(b, "/t/rate_converter?amount=550&currency=GBP&period=day&target=AED")
}
func BenchmarkPlannerGrid(b *testing.B) {
	benchmarkPage(b, "/t/meeting_planner?city1=Dubai&city2=Manila&date=2026-11-16")
}
