# Architecture

Go 1.x (stdlib-first) + `html/template` + htmx + a CSS file compiled from the Stack theme. No JS
build step, no front-end framework, no database.

## Shape

```
cmd/toolsd/main.go            flags (-addr, -base-url), registry wiring, graceful shutdown
internal/tool/
  tool.go                     Tool, Input, Values, Registry, Visibility
  validate.go                 Values -> typed getters that enforce Input rules
  slugify.go                  first tool (public, pure)
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
needs only the file `toolsd`.

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
    Files    []File   // optional download (name + bytes)
    JSON     any      // optional structured payload for /api and MCP
    Warnings []string
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
- Validation errors re-render the form partial with field-level messages and 422 for htmx (htmx
  does not swap on 4xx by default — that is why the fragment is returned with 200 and a
  `data-invalid` attribute, so the message is visible; documented in the handler).
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
| reading time / word count | Public | nothing |
| post keyword search over the blog's static JSON | Public | a build-time copy of the blog's `index.json` (decide: fetch in CI, pin by sha) |
| timezone overlap | Public | `time.LoadLocation` (tzdata embedded, `CGO_ENABLED=0`) |
| link preview | **Gated** | outbound HTTPS + SSRF guard + egress allow |
| Notion `Writing` DB search | **Gated** | `NOTION_API_KEY` via ExternalSecret + egress to `api.notion.com` |

If a candidate needs a database, a queue, or a login of its own, it is a different app.
