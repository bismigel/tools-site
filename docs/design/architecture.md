# Architecture

Go 1.x (stdlib-first) + `html/template` + htmx + a CSS file compiled from the Stack theme. No JS
build step, no front-end framework, no database.

## Shape

```
cmd/toolsd/main.go            flags (-addr, -base-url), registry wiring, graceful shutdown
internal/tool/
  tool.go                     Tool, Input, Values, Registry, Visibility, Result, Table
  validate.go                 Values -> typed getters that enforce Input rules
  slugify.go                  first tool (public, pure)
  rate_converter.go           M6: currency + period normalisation (public; embedded ECB rates)
  meeting_planner.go          M6: multi-city overlap grid (public; embedded tzdata)
internal/fx/
  rates.json                  dated ECB reference rates, refreshed by CI (ADR-0009)
internal/web/
  routes.go                   mux + middleware chain
  pages.go                    full-page handlers (index, tool page, lab page)
  api.go                      /api/tools (manifest), /api/tools/{name}, /api/lab/*
  schema.go                   []Input -> JSON Schema (manifest, WebMCP inputSchema)
  render.go                   template sets; htmx fragment vs full page
  errors.go                   sentinel -> status + user-safe message
  auth.go                     CF Access JWT verification (M3b)
  middleware.go               recover, request-id, log, security headers, body limit, timeout
web/templates/                layout.html, index.html, tool.html, lab.html, partials/*
web/static/                   css/tools.css, js/htmx.min.js, js/webmcp.js, img/avatar.svg
theme/                        submodule (theme) + site-scss/custom.scss + tools.css (generated)
deploy/k8s/                   manifests ArgoCD applies
```

Templates and static assets are `//go:embed`-ed, so the binary is self-contained and the container
needs only the file `toolsd`. Two data files are embedded the same way: the dated ECB rate file
(`internal/fx/rates.json`, refreshed by CI) and the time zone database (`import _ "time/tzdata"`).
Neither is read from the filesystem at runtime, and neither implies network access: in v1 the
process makes **no outbound request at all** (ADR-0009).

## One definition, six surfaces

```go
type Visibility int // Public, Gated

type Input struct {
    Name     string   // ^[a-z][a-z0-9_]{0,31}$
    Label    string
    Kind     Kind     // Text, Textarea, Number, Checkbox, Select
    Help     string
    Required bool
    Min, Max int      // lengths for text; value range for number
    Options  []string // Select only
    Default  string
}

type Tool struct {
    Name, Summary, Description string // Name ^[a-z][a-z0-9_-]{0,63}$
    Visibility                 Visibility
    Inputs                     []Input
    Run                        func(ctx context.Context, in Values) (Result, error)
}

type Result struct {
    Title    string   // short heading for the output card
    Lines    []Line   // text rows, escaped by the renderer
    Table    *Table   // optional grid: rate tables, timezone grids
    Files    []File   // optional download (name + bytes)
    JSON     any      // optional structured payload for /api and MCP
    Warnings []string
}

type Cell struct {
    Text string
    Tone Tone   // ToneNormal, ToneEmphasis, ToneMuted
    Note string // optional, e.g. "+1 day"
}

type Table struct {
    Caption string
    Columns []string
    Rows    [][]Cell
}
```

| Surface | Route | Derived from |
| --- | --- | --- |
| index | `GET /` (`/lab` for gated) | `Registry.All(visibility)` |
| form page | `GET /t/{name}` / `GET /lab/{name}` | `Tool.Inputs` → HTML |
| htmx fragment | `POST /t/{name}` / `POST /lab/{name}` (HX-Request) | `tool.html` partial |
| manifest | `GET /api/tools`, `GET /api/lab/tools` | `Tool` → JSON Schema |
| execute | `POST /api/tools/{name}`, `POST /api/lab/{name}` | `Tool.Run` |
| MCP (M7) | `POST /mcp` | same registry |

**Gating is not a route-level afterthought**: the router mounts the public and gated zones as two
`http.ServeMux` subtrees, so a tool with `Visibility: Gated` is unreachable under `/t/`. A test
asserts that every gated tool 404s on the public path and that every public tool 404s under `/lab/`.

### Results are data, not markup

`Result` carries strings and a small `Line` enum (`Text`, `KeyValue`, `Code`, `Warning`). The
template escapes and styles them. `tool.Result` has **no `template.HTML` field**: a tool that returns
markup is an XSS hole, and the only escape hatch (a documented `RawHTML` for first-party tools) does
not exist until something genuinely needs it.

## Request flow

```
ingress-nginx ─▶ :8080
  recover ─▶ request-id (CF-Ray or generated) ─▶ access log
    ─▶ security headers (CSP etc.) ─▶ body limit 64 KiB ─▶ timeout (per-route)
      ─▶ route
           /healthz               200, no deps
           /readyz                registry built, CSS embedded, JWT keys cached-or-fetchable
           /t|/lab/{name}         CSRF+origin check on POST, then Registry.Get -> Tool.Run
           /api/**                JSON, no HTML, no template
```

- `POST` on a gated route additionally requires a valid Access JWT **and** a CSRF token
  (`docs/design/security.md`).
- Errors: sentinel errors (`ErrNotFound`, `ErrBadInput`, `ErrUnauthorized`, `ErrUpstream`) map to
  status codes; the body is a short user-safe message plus `request_id`. Internal error text goes to
  the log only.
- Panics: recovered per request, logged with the route and `request_id`, returned as 500 — never as
  a stack trace in the response.

## htmx contract

- Form: `<form hx-post="/t/{name}" hx-target="#result" hx-swap="innerHTML" hx-indicator="#busy">`.
- The handler returns **only the `#result` partial** when `HX-Request: true`, and a full page
  otherwise (deep link, no-JS, and non-JS agents all keep working).
- Validation errors re-render the form partial with field-level messages and **`200`**, never `422`
  (htmx does not swap 4xx bodies by default). The fragment carries `data-invalid` on the form plus a
  visible summary, and the status code is the same for htmx and no-JS requests — one code path
  (`spec.md` FR-2.4).
- No `hx-on` inline JS (CSP forbids inline script; see security).

## Theme reuse

`web/templates/layout.html` is a one-time port of the theme's `baseof.html`: same container classes,
same sidebar shape, same `data-scheme` toggle. CSS comes from `theme/tools.css`, generated from the
pinned theme + the blog's palette (`docs/design/theme.md`). Anything visual that is tool-specific
goes in `theme/site-scss/custom.scss` under a `/* tools: */` comment block, using the existing
`--card-*` / `--accent-color` variables — not new colours.

## Tool candidates (scope guard)

| Tool | Visibility | Needs |
| --- | --- | --- |
| slugify (text → url slug) | Public | nothing (M1) |
| **rate converter** (amount + period → hourly/weekly/monthly/annual, two currencies) | Public | embedded ECB rates, refreshed in CI (M6) |
| **meeting planner** (2–6 cities, one date → overlap grid) | Public | embedded `time/tzdata` (M6) |
| reading time / word count | Public | nothing (candidate, once M6 exists) |
| post keyword search over the blog's static JSON | Public | a build-time copy of the blog's `index.json` (fetch in CI, pin by sha) |
| link preview | **Gated** | outbound HTTPS + SSRF guard + egress allow |
| Notion `Writing` DB search | **Gated** | `NOTION_API_KEY` via ExternalSecret + egress to `api.notion.com` |

If a candidate needs a database, a queue, or a login of its own, it is a different app.
