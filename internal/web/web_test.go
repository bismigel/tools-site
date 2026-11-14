package web

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/bismigel/tools-site/internal/tool"
)

func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()

	css, err := os.ReadFile("../../theme/tools.css")
	if err != nil {
		t.Fatalf("theme/tools.css missing — run `make css`: %v", err)
	}

	reg := tool.NewRegistry()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(reg.Register(tool.NewSlugify()))
	must(reg.Register(tool.Tool{
		Name:       "boom",
		Summary:    "Panics on purpose to test recovery",
		Visibility: tool.Public,
		Run: func(context.Context, tool.Values) (tool.Result, error) {
			panic("boom")
		},
	}))
	must(reg.Register(tool.Tool{
		Name:       "secret_tool",
		Summary:    "A gated tool that must never appear in the public zone",
		Visibility: tool.Gated,
		Run: func(context.Context, tool.Values) (tool.Result, error) {
			return tool.Result{Title: "secret"}, nil
		},
	}))

	srv, err := New(Config{
		Registry:  reg,
		BaseURL:   "https://tools.example",
		Templates: os.DirFS("../.."),
		Static:    os.DirFS("../../web/static"),
		CSS:       css,
		Log:       slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatal(err)
	}

	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)
	return ts
}

func get(t *testing.T, ts *httptest.Server, path string) (*http.Response, string) {
	t.Helper()
	res, err := ts.Client().Get(ts.URL + path)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	return res, string(body)
}

func postJSON(t *testing.T, ts *httptest.Server, path, body string, hdr map[string]string) (*http.Response, string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, ts.URL+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	res, err := ts.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	out, _ := io.ReadAll(res.Body)
	res.Body.Close()
	return res, string(out)
}

func postForm(t *testing.T, ts *httptest.Server, path string, vals url.Values, hx bool) (*http.Response, string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, ts.URL+path, strings.NewReader(vals.Encode()))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if hx {
		req.Header.Set("HX-Request", "true")
	}
	res, err := ts.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	out, _ := io.ReadAll(res.Body)
	res.Body.Close()
	return res, string(out)
}

func TestIndexAndPages(t *testing.T) {
	ts := newTestServer(t)

	res, body := get(t, ts, "/")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET / = %d", res.StatusCode)
	}
	for _, want := range []string{"slugify", "/t/slugify", "boom"} {
		if !strings.Contains(body, want) {
			t.Errorf("index missing %q", want)
		}
	}
	if strings.Contains(body, "secret_tool") {
		t.Error("gated tool leaked into the public index")
	}

	res, body = get(t, ts, "/t/slugify")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET /t/slugify = %d", res.StatusCode)
	}
	for _, want := range []string{`action="/t/slugify"`, `name="text"`, `hx-post="/t/slugify"`} {
		if !strings.Contains(body, want) {
			t.Errorf("tool page missing %q", want)
		}
	}

	res, body = get(t, ts, "/t/nope")
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("GET /t/nope = %d", res.StatusCode)
	}
	if !strings.Contains(body, "Not found") {
		t.Error("404 page is not themed")
	}
	if !strings.Contains(body, "<!DOCTYPE") {
		t.Error("404 is not a full page")
	}
}

// The gated zone is not mounted without a verifier: FR-4.3.
func TestGatedZoneIsAbsent(t *testing.T) {
	ts := newTestServer(t)

	for _, path := range []string{"/lab", "/lab/slugify", "/lab/secret_tool"} {
		res, _ := get(t, ts, path)
		if res.StatusCode != http.StatusNotFound {
			t.Errorf("GET %s = %d, want 404", path, res.StatusCode)
		}
	}
	res, _ := postJSON(t, ts, "/api/lab/secret_tool", `{}`, nil)
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("POST /api/lab/secret_tool = %d, want 404", res.StatusCode)
	}
}

func TestManifest(t *testing.T) {
	ts := newTestServer(t)

	res, body := get(t, ts, "/api/tools")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", res.StatusCode)
	}
	if got := res.Header.Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control = %q", got)
	}
	if !strings.Contains(res.Header.Get("Content-Type"), "application/json") {
		t.Errorf("Content-Type = %q", res.Header.Get("Content-Type"))
	}

	var m manifest
	if err := json.Unmarshal([]byte(body), &m); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	names := make([]string, 0, len(m.Tools))
	for _, x := range m.Tools {
		names = append(names, x.Name)
	}
	if strings.Join(names, ",") != "slugify,boom" {
		t.Fatalf("manifest names = %v (want registration order, public only)", names)
	}
	for _, x := range m.Tools {
		if x.Description == "" {
			t.Errorf("%s has no description", x.Name)
		}
		if x.InputSchema.Type != "object" {
			t.Errorf("%s schema type = %q", x.Name, x.InputSchema.Type)
		}
	}
	if _, ok := m.Tools[0].InputSchema.Properties["text"]; !ok {
		t.Error("slugify schema has no text property")
	}
	if len(m.Tools[0].InputSchema.Required) != 1 || m.Tools[0].InputSchema.Required[0] != "text" {
		t.Errorf("required = %v", m.Tools[0].InputSchema.Required)
	}
}

func TestAPIExecute(t *testing.T) {
	ts := newTestServer(t)

	res, body := postJSON(t, ts, "/api/tools/slugify", `{"text":"Hello, World!"}`, nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d body=%s", res.StatusCode, body)
	}
	var out resultJSON
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatal(err)
	}
	if out.RequestID == "" {
		t.Error("no request_id in the result")
	}
	if data, ok := out.Data.(map[string]any); !ok || data["slug"] != "hello-world" {
		t.Errorf("data = %#v", out.Data)
	}

	t.Run("bad input is 400", func(t *testing.T) {
		res, body := postJSON(t, ts, "/api/tools/slugify", `{"text":""}`, nil)
		if res.StatusCode != http.StatusBadRequest {
			t.Fatalf("status = %d", res.StatusCode)
		}
		if !strings.Contains(body, `"bad_input"`) {
			t.Errorf("body = %s", body)
		}
	})

	t.Run("unknown field is 400", func(t *testing.T) {
		res, body := postJSON(t, ts, "/api/tools/slugify", `{"nope":"x"}`, nil)
		if res.StatusCode != http.StatusBadRequest || !strings.Contains(body, "nope") {
			t.Fatalf("status = %d body=%s", res.StatusCode, body)
		}
	})

	t.Run("malformed JSON is 400", func(t *testing.T) {
		res, _ := postJSON(t, ts, "/api/tools/slugify", `{`, nil)
		if res.StatusCode != http.StatusBadRequest {
			t.Fatalf("status = %d", res.StatusCode)
		}
	})

	t.Run("wrong zone is 404, not 403", func(t *testing.T) {
		res, _ := postJSON(t, ts, "/api/tools/secret_tool", `{}`, nil)
		if res.StatusCode != http.StatusNotFound {
			t.Fatalf("status = %d", res.StatusCode)
		}
	})

	t.Run("unknown tool is 404", func(t *testing.T) {
		res, _ := postJSON(t, ts, "/api/tools/nope", `{}`, nil)
		if res.StatusCode != http.StatusNotFound {
			t.Fatalf("status = %d", res.StatusCode)
		}
	})

	t.Run("cross-origin POST is 403", func(t *testing.T) {
		res, _ := postJSON(t, ts, "/api/tools/slugify", `{"text":"x"}`,
			map[string]string{"Origin": "https://evil.example"})
		if res.StatusCode != http.StatusForbidden {
			t.Fatalf("status = %d", res.StatusCode)
		}
	})

	t.Run("same-origin POST succeeds", func(t *testing.T) {
		res, _ := postJSON(t, ts, "/api/tools/slugify", `{"text":"x"}`,
			map[string]string{"Origin": "https://tools.example"})
		if res.StatusCode != http.StatusOK {
			t.Fatalf("status = %d", res.StatusCode)
		}
	})
}

func TestHTMLForm(t *testing.T) {
	ts := newTestServer(t)

	t.Run("full page without htmx", func(t *testing.T) {
		res, body := postForm(t, ts, "/t/slugify", url.Values{"text": {"Hello, World!"}}, false)
		if res.StatusCode != http.StatusOK {
			t.Fatalf("status = %d", res.StatusCode)
		}
		if !strings.Contains(body, "<!DOCTYPE") {
			t.Error("not a full page")
		}
		if !strings.Contains(body, "hello-world") {
			t.Error("result missing")
		}
	})

	t.Run("fragment with htmx", func(t *testing.T) {
		res, body := postForm(t, ts, "/t/slugify", url.Values{"text": {"Hello, World!"}}, true)
		if res.StatusCode != http.StatusOK {
			t.Fatalf("status = %d", res.StatusCode)
		}
		if strings.Contains(body, "<!DOCTYPE") || strings.Contains(body, "<html") {
			t.Error("htmx response is a full page")
		}
		if !strings.Contains(body, "hello-world") {
			t.Error("result missing from fragment")
		}
	})

	t.Run("validation is 200 with data-invalid", func(t *testing.T) {
		res, body := postForm(t, ts, "/t/slugify", url.Values{"text": {""}}, false)
		if res.StatusCode != http.StatusOK {
			t.Fatalf("status = %d (spec FR-2.4 wants 200)", res.StatusCode)
		}
		if !strings.Contains(body, `data-invalid="true"`) {
			t.Error("no data-invalid on the form")
		}
		if !strings.Contains(body, "Required") {
			t.Error("no field message")
		}
	})

	t.Run("htmx validation retargets the form", func(t *testing.T) {
		res, body := postForm(t, ts, "/t/slugify", url.Values{"text": {""}}, true)
		if got := res.Header.Get("HX-Retarget"); got != "#tool-form" {
			t.Errorf("HX-Retarget = %q", got)
		}
		if strings.Contains(body, "<!DOCTYPE") {
			t.Error("htmx validation returned a full page")
		}
	})

	t.Run("run failure keeps 200 for html", func(t *testing.T) {
		res, body := postForm(t, ts, "/t/slugify", url.Values{"text": {"!!!"}}, false)
		if res.StatusCode != http.StatusOK {
			t.Fatalf("status = %d", res.StatusCode)
		}
		if !strings.Contains(strings.ToLower(body), "slug") {
			t.Error("no user-safe message")
		}
	})
}

func TestLimitsAndRecovery(t *testing.T) {
	ts := newTestServer(t)

	t.Run("body over 64 KiB is 413", func(t *testing.T) {
		res, _ := postForm(t, ts, "/t/slugify",
			url.Values{"text": {strings.Repeat("a", maxBody+10)}}, false)
		if res.StatusCode != http.StatusRequestEntityTooLarge {
			t.Fatalf("status = %d", res.StatusCode)
		}
	})

	t.Run("panic is 500 and does not leak", func(t *testing.T) {
		res, body := postForm(t, ts, "/t/boom", url.Values{}, false)
		if res.StatusCode != http.StatusInternalServerError {
			t.Fatalf("status = %d", res.StatusCode)
		}
		if strings.Contains(body, "goroutine") || strings.Contains(body, "panic") {
			t.Error("panic detail leaked into the response")
		}
	})
}

func TestOperationalEndpoints(t *testing.T) {
	ts := newTestServer(t)

	for _, path := range []string{"/healthz", "/readyz"} {
		res, body := get(t, ts, path)
		if res.StatusCode != http.StatusOK || strings.TrimSpace(body) != "ok" {
			t.Errorf("%s = %d %q", path, res.StatusCode, body)
		}
	}

	res, body := get(t, ts, "/robots.txt")
	if res.StatusCode != http.StatusOK || !strings.Contains(body, "Allow: /") {
		t.Errorf("robots = %d %q", res.StatusCode, body)
	}

	t.Run("dev reload is 404 outside dev", func(t *testing.T) {
		res, _ := get(t, ts, "/dev/reload")
		if res.StatusCode != http.StatusNotFound {
			t.Errorf("status = %d", res.StatusCode)
		}
	})
}

func TestStylesheetAndHeaders(t *testing.T) {
	ts := newTestServer(t)

	res, body := get(t, ts, "/static/css/tools.css")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", res.StatusCode)
	}
	if !strings.Contains(res.Header.Get("Content-Type"), "text/css") {
		t.Errorf("Content-Type = %q", res.Header.Get("Content-Type"))
	}
	if !strings.Contains(body, "--accent-color") {
		t.Error("stylesheet is not the generated theme CSS")
	}

	_, _ = get(t, ts, "/")
	// Headers must be present on every response; check one page and one API call.
	for _, path := range []string{"/", "/api/tools"} {
		res, _ := get(t, ts, path)
		checks := map[string]string{
			"Content-Security-Policy":    "default-src 'none'",
			"X-Content-Type-Options":     "nosniff",
			"Referrer-Policy":            "strict-origin-when-cross-origin",
			"Cross-Origin-Opener-Policy": "same-origin",
			"Permissions-Policy":         "geolocation=()",
		}
		for h, want := range checks {
			if !strings.Contains(res.Header.Get(h), want) {
				t.Errorf("%s: %s = %q", path, h, res.Header.Get(h))
			}
		}
	}
}

func TestDevModeOnlySurface(t *testing.T) {
	css, err := os.ReadFile("../../theme/tools.css")
	if err != nil {
		t.Fatal(err)
	}
	reg := tool.NewRegistry()
	if err := reg.Register(tool.NewSlugify()); err != nil {
		t.Fatal(err)
	}
	srv, err := New(Config{
		Registry:  reg,
		Dev:       true,
		Templates: os.DirFS("../.."),
		Static:    os.DirFS("../../web/static"),
		CSS:       css,
		Log:       slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatal(err)
	}

	// The dev script tag appears only with Dev.
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if !strings.Contains(rec.Body.String(), "/static/js/dev.js") {
		t.Error("dev mode does not reference dev.js")
	}

	prod, err := New(Config{
		Registry:  reg,
		Templates: os.DirFS("../.."),
		Static:    os.DirFS("../../web/static"),
		CSS:       css,
		Log:       slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatal(err)
	}
	rec = httptest.NewRecorder()
	prod.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if strings.Contains(rec.Body.String(), "dev.js") {
		t.Error("production page references dev.js")
	}
}

func TestWebMCPBridgeIsServedAndReferenced(t *testing.T) {
	ts := newTestServer(t)

	_, page := get(t, ts, "/")
	if !strings.Contains(page, "/static/js/webmcp.js") {
		t.Error("layout does not reference the WebMCP bridge")
	}

	res, js := get(t, ts, "/static/js/webmcp.js")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("bridge status = %d", res.StatusCode)
	}
	for _, want := range []string{"provideContext", "/api/tools/"} {
		if !strings.Contains(js, want) {
			t.Errorf("bridge missing %q", want)
		}
	}
}

func postRPC(t *testing.T, ts *httptest.Server, body string) (*http.Response, string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, ts.URL+"/mcp", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := ts.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	out, _ := io.ReadAll(res.Body)
	res.Body.Close()
	return res, string(out)
}

func TestMCPInitializeAndList(t *testing.T) {
	ts := newTestServer(t)

	res, body := postRPC(t, ts, `{"jsonrpc":"2.0","id":1,"method":"initialize"}`)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("initialize status = %d", res.StatusCode)
	}
	for _, want := range []string{"protocolVersion", "capabilities", "serverInfo"} {
		if !strings.Contains(body, want) {
			t.Errorf("initialize missing %q: %s", want, body)
		}
	}

	res, body = postRPC(t, ts, `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`)
	if res.StatusCode != http.StatusOK || !strings.Contains(body, `"slugify"`) {
		t.Fatalf("tools/list = %d %s", res.StatusCode, body)
	}
	if strings.Contains(body, "secret_tool") {
		t.Error("gated tool advertised over MCP")
	}
	if !strings.Contains(body, `"inputSchema"`) {
		t.Error("tools/list has no schemas")
	}
}

func TestMCPCall(t *testing.T) {
	ts := newTestServer(t)

	res, body := postRPC(t, ts, `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"slugify","arguments":{"text":"Hello, World!"}}}`)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d %s", res.StatusCode, body)
	}
	for _, want := range []string{`"hello-world"`, `"content"`, `"structuredContent"`, `"isError":false`} {
		if !strings.Contains(body, want) {
			t.Errorf("tools/call missing %q: %s", want, body)
		}
	}

	t.Run("bad arguments are a tool error, not a transport error", func(t *testing.T) {
		res, body := postRPC(t, ts, `{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"slugify","arguments":{"text":""}}}`)
		if res.StatusCode != http.StatusOK {
			t.Fatalf("status = %d", res.StatusCode)
		}
		if !strings.Contains(body, `"isError":true`) {
			t.Errorf("body = %s", body)
		}
	})

	t.Run("unknown tool is -32602", func(t *testing.T) {
		_, body := postRPC(t, ts, `{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"nope","arguments":{}}}`)
		if !strings.Contains(body, "-32602") {
			t.Errorf("body = %s", body)
		}
	})

	t.Run("gated tool is 401", func(t *testing.T) {
		res, body := postRPC(t, ts, `{"jsonrpc":"2.0","id":6,"method":"tools/call","params":{"name":"secret_tool","arguments":{}}}`)
		if res.StatusCode != http.StatusUnauthorized {
			t.Fatalf("status = %d", res.StatusCode)
		}
		if !strings.Contains(body, "-32001") {
			t.Errorf("body = %s", body)
		}
	})

	t.Run("unknown method is -32601", func(t *testing.T) {
		_, body := postRPC(t, ts, `{"jsonrpc":"2.0","id":7,"method":"resources/list"}`)
		if !strings.Contains(body, "-32601") {
			t.Errorf("body = %s", body)
		}
	})

	t.Run("a notification is accepted with no body", func(t *testing.T) {
		res, body := postRPC(t, ts, `{"jsonrpc":"2.0","method":"notifications/initialized"}`)
		if res.StatusCode != http.StatusAccepted || body != "" {
			t.Errorf("status = %d body = %q", res.StatusCode, body)
		}
	})

	t.Run("malformed JSON is 400", func(t *testing.T) {
		res, _ := postRPC(t, ts, `{`)
		if res.StatusCode != http.StatusBadRequest {
			t.Errorf("status = %d", res.StatusCode)
		}
	})
}

func TestMetrics(t *testing.T) {
	ts := newTestServer(t)

	_, _ = get(t, ts, "/")
	_, _ = postJSON(t, ts, "/api/tools/slugify", `{"text":"metrics me"}`, nil)

	res, body := get(t, ts, "/metrics")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", res.StatusCode)
	}
	for _, want := range []string{
		"# TYPE tools_http_requests_total counter",
		`tools_http_requests_total{path="/",status="200"}`,
		"# TYPE tools_tool_runs_total counter",
		`tools_tool_runs_total{tool="slugify",outcome="ok"}`,
		"tools_request_duration_seconds_count",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("metrics missing %q\n%s", want, body)
		}
	}
}
