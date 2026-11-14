# Spec — tools-site

Status: **Draft v0.2** · Owner: @bismigel · Date: 2026-11-14 · Last reviewed: 2026-11-14
(planning session: addresses, API-first, MCP, the `hire` app)

> This file is the **behavioural contract**. It says what the system must do, in testable terms.
> It does **not** say when (that is `docs/plan.md`) or why a choice was made (that is `docs/adr/`).
>
> **Precedence when documents disagree:** ADR > this spec > `docs/design/*` > code comments.
> If the code disagrees with this spec, the code is wrong, unless the spec is amended in the same PR.
>
> Every requirement has an ID. Tests and PR descriptions cite the ID (`FR-4.2`, `NFR-2.3`).
> Every test that verifies a requirement names it in a comment or subtest name.

---

## 1. Purpose

`tools.ciceley.cc` is a small, self-hosted host for **small deterministic tools**. A visitor gets a
themed form, submits it, gets a result. The same definition of a tool is also exposed as JSON and to
browser AI agents via WebMCP, and later to MCP clients over `/mcp`.

The problem it solves: a personal, ad-free place to put the tiny utilities that otherwise end up in
a bookmarklet, a random website, or a shell script nobody else can run.

### 1.1 Users

| User | Needs | Constraint |
| --- | --- | --- |
| **Owner** (primary) | private tools (Notion search, link preview) without exposing them publicly | one login, no second password system |
| **Visitor** | use public tools with no account, no cookie banner, no JS required | works with JS off, fast on mobile |
| **AI agent, in-browser** | discover and call public tools via WebMCP | only what the page advertises |
| **AI agent, headless** (M7) | same tools over MCP/JSON-RPC | same schemas, same visibility |

### 1.2 Two applications

The client's goal moved: the tool host is the foundation, and a **technical-recruiter HR assistant** is
the product that sits on top. They are two apps, two repos, two hostnames, one deployment pattern.

| App | Address | Repo | Spec |
| --- | --- | --- | --- |
| **tools-site** (this) | `tools.ciceley.cc`, private tools under `/lab` | this repo | this file |
| **hire** (HR assistant) | `hire.ciceley.cc` | separate repo | `docs/spec/hire-assistant.md` |

Why two: the tool host is public, stateless and stable; the assistant is private, model-dependent and
handles candidate personal data. Sharing a binary would put the second one's risk inside the first
one's trust boundary (ADR-0008).

**They talk over the API, never over a shared database**: `hire` calls `tools-site`'s MCP endpoint
(FR-9) with its own Access service token. That is the whole reason this spec is API-first (FR-8).

### 1.3 Non-goals (anti-requirements — this is scope control, not modesty)

These are **out of scope permanently** unless a new ADR says otherwise. A PR adding any of them is
a design change, not a feature.

- **NG-1** No database, no queue, no background jobs, no scheduled tasks.
- **NG-2** No user accounts of our own. Identity is Cloudflare Access or nobody.
- **NG-3** No tool that shells out (`os/exec`) in v1.
- **NG-4** No per-tool templates, CSS files, JS files, or handlers. One definition, six surfaces.
- **NG-5** No front-end framework or build step. htmx is the only JS library.
- **NG-6** No public URL fetcher (an open proxy). Any fetching tool is `Gated` and SSRF-guarded.
- **NG-7** No CORS for `/api/*`. Same-origin only; agents use WebMCP or MCP.
- **NG-8** No analytics, no tracking pixels, no third-party fonts/CDNs at runtime.
- **NG-9** No write path that persists user input anywhere except logs (and gated values are never logged).
- **NG-10** No `Namespace` object, no cluster-scoped resources in this repo.

---

## 2. Functional requirements

### FR-1 — Tool definition (the single source of truth)

- **FR-1.1** A tool is exactly one `tool.Tool` value registered in the registry. Name, summary,
  description, inputs, visibility and behaviour live there and nowhere else.
- **FR-1.2** `Tool.Name` MUST match `^[a-z][a-z0-9_]{0,63}$`. No hyphens: the same string is the
  URL segment, the JSON key, the MCP tool name and the WebMCP tool name, and WebMCP allows only
  `[a-z0-9_]`. Being stricter than any one surface is deliberate; it removes a mapping layer.
- **FR-1.3** `Tool.Summary` is one sentence, ≤ 120 characters, no trailing period. It is the card
  text, the manifest description and the MCP description.
- **FR-1.4** A tool MUST declare 0–20 inputs. `Input.Name` MUST match `^[a-z][a-z0-9_]{0,31}$`.
- **FR-1.5** Registry construction MUST fail at startup (non-zero exit, named error) if any rule in
  FR-1.2–FR-1.4 is violated, or if two tools share a name. No tool is silently dropped.
- **FR-1.6** `Tool.Run` receives a `context.Context` that is cancelled at the per-tool timeout
  (NFR-1.3) and returns `(Result, error)`. It MUST NOT write to the response.
- **FR-1.7** `Result` carries text `Lines`, an optional generic `Table` (caption, columns, rows of
  `Cell{Text, Tone, Note}`), optional `Files`, optional `JSON` and `Warnings`. It has **no raw-HTML
  field** and no per-tool markup: the renderer escapes every string and styles every table the same
  way. `Table` exists because grids (a 24×6 timezone table, a 5×3 rate table) cannot be honest text
  rows — it is a primitive, not a per-tool template.
- **FR-1.8** Tool inputs are validated by generated code-path from `Input` rules before `Run` is
  called: required, min/max length in runes, numeric range, `Select` membership. `Run` MUST NOT
  need to re-validate.
- **FR-1.9** A tool that makes an outbound request MUST be `Gated` and MUST use the shared
  guarded HTTP client (NFR-2.4). It MUST name its destination and port in
  `docs/design/security.md` and in the NetworkPolicy in the same commit.
- **FR-1.10** A tool MAY depend on **build-time data** (exchange rates, time zone database) only if
  that data is pinned in the repo, dated, embedded in the binary, and refreshed by CI — never fetched
  at runtime. Such a tool stays `Public` and the runtime needs no egress (ADR-0009). If a tool ever
  needs a *runtime* fetch, FR-1.9 applies and it is `Gated`.
- **FR-1.11** `Input` MAY carry `Suggestions []string`, rendered as an HTML `<datalist>` on text
  inputs. Suggestions are convenience only and MUST NOT affect validation.

### FR-2 — Pages (HTML surfaces)

- **FR-2.1** `GET /` lists every `Public` tool as a card: name, summary, link. Order is registry
  order, which MUST be deterministic (registration order, not map iteration).
- **FR-2.2** `GET /t/{name}` renders the tool page for a `Public` tool: summary, generated form,
  empty result region, no-JS fallback copy.
- **FR-2.3** `POST /t/{name}` validates and runs the tool.
  - With `HX-Request: true` → `200` and **only** the result fragment (`#result` inner HTML).
  - Without it → a full page, same content, so the no-JS path works.
- **FR-2.4** Invalid input re-renders the form with field-level messages. The response is `200`
  with `data-invalid` on the form and a visible summary, **not** `422` (htmx does not swap 4xx
  bodies by default). The status-code choice MUST be identical for htmx and no-JS requests.
- **FR-2.5** Gated tools are reachable only under `/lab/{name}` (GET) and `/api/lab/{name}`.
  `GET /lab` lists gated tools. A gated tool is **404** under `/t/`, and a public tool is **404**
  under `/lab/`. A test asserts both directions for every registered tool.
- **FR-2.6** Every page MUST render and every form MUST submit with JavaScript disabled.
- **FR-2.7** Unknown paths render the themed 404 page, not a bare server error.
- **FR-2.8** A tool page MUST be a deep link: reload and direct navigation produce the same page.

### FR-3 — Manifest and execute API

- **FR-3.1** `GET /api/tools` returns `{"tools":[{"name","description","inputSchema"}]}` for
  `Public` tools only. `description` is the summary (FR-1.3). `inputSchema` is JSON Schema
  (`type: object`, `properties`, `required`) generated from `Tool.Inputs`.
- **FR-3.2** If and only if the request carries a valid Access JWT, `GET /api/tools` appends the
  `Gated` tools. There is no separate public flag.
- **FR-3.3** `POST /api/tools/{name}` accepts a JSON object matching the tool's `inputSchema`,
  runs it, and returns `Result` as JSON. Unknown fields are rejected (`DisallowUnknownFields`).
- **FR-3.4** `/api/*` never returns HTML, never renders a template, and always sends
  `Cache-Control: no-store`.
- **FR-3.5** A gated tool under `/api/tools/{name}` is 404, never 403: the public zone must not
  confirm that a hidden tool exists.
- **FR-3.6** Errors use one envelope: `{"error":{"code","message","request_id"}}` where `code` is a
  stable machine string (`bad_input`, `not_found`, `unauthorized`, `timeout`, `upstream`, `internal`)
  and `message` is user-safe (no internal detail).

### FR-4 — Visibility and gating

- **FR-4.1** `Visibility` is a field on the tool. The router derives the mount points from it
  (ADR-0003). There are no per-tool Access rules, redirects or handlers.
- **FR-4.2** Authentication for the gated zone is verified **in-process** from
  `Cf-Access-Jwt-Assertion` (RS256, JWKS, `aud`, `exp` +60 s, `iat`, `iss`). The
  `Cf-Access-Authenticated-User-Email` header is used for audit logging only and MUST NOT affect an
  authorisation decision. Missing/malformed/unknown-`kid`/expired ⇒ `401`.
- **FR-4.3** **Fail closed on missing configuration.** If `CF_ACCESS_TEAM_DOMAIN` or
  `CF_ACCESS_AUD` is unset, the gated routes are **not mounted at all**: `/lab*` and `/api/lab*`
  return 404 and startup logs one warning. This is what makes M3 (deployed, no Access yet) safe.
- **FR-4.4** `TOOLS_DEV_INSECURE=1` bypasses JWT verification locally and MUST refuse to start when
  `KUBERNETES_SERVICE_HOST` is set. It logs a warning on every request it lets through.
- **FR-4.5** Every state-changing request (`POST`) requires a same-origin check
  (`Sec-Fetch-Site: same-origin`, else `Origin` equals the configured base URL). Gated `POST`s
  additionally require the CSRF token; failures are `403` with a logged `request_id`.

### FR-5 — WebMCP

- **FR-5.1** No per-tool JavaScript exists. `web/static/js/webmcp.js` fetches `/api/tools`, maps
  each entry to `{name, description, inputSchema, execute}` and calls
  `navigator.modelContext.provideContext({tools})` **once** per page load (the API replaces the set).
- **FR-5.2** `execute` posts to `/api/tools/{name}` (or `/api/lab/{name}` for a gated page), adapts
  the JSON result to `{content:[{type:"text",text}]}`, and returns errors as `{isError:true}` —
  it never throws.
- **FR-5.3** If `navigator.modelContext?.provideContext` is absent, the script logs one
  `console.debug` and does nothing else. No polyfill, no error, page unaffected.
- **FR-5.4** Registration failure (throw from the browser API) MUST NOT break the page.
- **FR-5.5** For the same input, WebMCP `execute` output and the HTML form output MUST be
  textually identical (single source: `Result`).

### FR-6 — Operational endpoints

- **FR-6.1** `GET /healthz` → `200 ok`, no dependency checks, no registry access, never touches
  the network. Used by liveness.
- **FR-6.2** `GET /readyz` → `200` only when the registry is built and templates/CSS are loaded;
  `503` otherwise. It MAY cause a cached JWKS fetch but MUST NOT block longer than 2 s. Used by
  readiness.
- **FR-6.3** Neither endpoint requires auth, logs at debug level only, and returns a plain-text body.
- **FR-6.4** Graceful shutdown: on SIGTERM, stop accepting, drain in-flight requests up to 10 s,
  exit 0.

### FR-7 — Adding a tool (the M6 contract)

A new tool MUST be exactly: one file in `internal/tool/` (definition + `Run` + tests) and one
`Register` line. If a reviewer sees any other file in the diff, the change is rejected (AGENTS.md
rule 1). Every tool ships with the per-tool spec block in §5 filled in.

### FR-8 — API-first (the contract is the product)

- **FR-8.1** Every capability is reachable as a JSON endpoint. A capability that exists only as an
  HTML form is a defect: the form is a *client* of the same `tool.Tool` definition.
- **FR-8.2** The JSON shapes are the external contract: `inputSchema` for inputs, the `Result` JSON
  for outputs, the error envelope (FR-3.6) for failures. Endpoints and field names are stable.
- **FR-8.3** Changes to the contract are **additive only** (new optional input, new result field,
  new tool). A breaking change requires a new path version and an ADR; it is never a silent edit.
- **FR-8.4** Every endpoint has a shape test, and the manifest is generated from the registry — never
  hand-written, never cached at build time.
- **FR-8.5** HTML and JSON MUST return the same result for the same input. One code path, two
  renderings; a divergence is a bug in the renderer, not a second implementation.
- **FR-8.6** API-first does **not** mean JavaScript-first: no capability may require JS or a browser
  (FR-2.6 still holds). The browser gets server-rendered HTML; agents get JSON; both are complete.

### FR-9 — MCP (the assistant's tool-calling surface, M7)

- **FR-9.1** `POST /mcp` speaks streamable HTTP and implements `tools/list` and `tools/call` from the
  same registry. No tool is defined for MCP separately.
- **FR-9.2** `tools/list` advertises public tools to anonymous callers; gated tools appear only when
  the request carries a valid Access JWT **or** a valid Access service token.
- **FR-9.3** The schemas in `tools/list` are byte-identical in meaning to `GET /api/tools`
  (FR-3.1): one generator, one output.
- **FR-9.4** Server-to-server callers (the `hire` app) authenticate with a **Cloudflare Access service
  token** (`CF-Access-Client-Id` / `CF-Access-Client-Secret`), stored via ExternalSecret, never in
  git, never logged. The in-process verifier MUST accept a service-token assertion only on `/mcp` and
  `/api/lab*`, and MUST NOT treat a service token as a human identity in audit logs. The token is
  required only when a **gated** tool is called; v0 of `hire` uses public tools and needs none
  (`DESIGN.md` appendix).
- **FR-9.5** `/mcp` is never a bypass: a gated tool without a JWT or service token is `401`, not
  `404` (the endpoint is known to exist; the caller is not authorised). It is rate-limited per token.
- **FR-9.6** `tools/call` returns the same `Result` JSON as FR-3.3. No MCP-specific result shape.

---

## 3. Non-functional requirements

### NFR-1 — Performance and limits

- **NFR-1.1** A pure tool's p95 handler latency MUST stay under 20 ms locally; the HTML page under
  50 ms. Measured in a benchmark or a `-race` test, not by feel.
- **NFR-1.2** Request body cap **64 KiB** on every `POST` (`http.MaxBytesReader`).
- **NFR-1.3** Per-tool timeout: 5 s default, 10 s for outbound tools, enforced with
  `context.WithTimeout`.
- **NFR-1.4** Per-tool concurrency cap 4 (semaphore); over the cap ⇒ queue up to the timeout, then
  `503` with `Retry-After`.
- **NFR-1.5** Per-client rate limit (token bucket, `CF-Connecting-IP` then socket address) for
  expensive/outbound tools only: 10 req/min default ⇒ `429` + `Retry-After`. Ingress handles the
  global limit; do not duplicate it.
- **NFR-1.6** Container memory limit is 96 MiB (`docs/design/deployment.md`). A tool that cannot
  run in that budget is out of scope.

### NFR-2 — Security

- **NFR-2.1** Response headers on **every** response are exactly the set in
  `docs/design/security.md` §5. No inline `style=`/`<script>` in templates (CSP `'self'` only).
- **NFR-2.2** Secrets are only ever read from the environment, provided by ExternalSecret. They
  never appear in logs, `/api` responses, error messages, or the repo.
- **NFR-2.3** Logs contain: timestamp, `request_id`, method, **path template** (never raw path),
  status, duration, tool name, outcome, `cf_ray`, and for gated requests the JWT `sub`. Never tool
  input values or output for gated tools; for public tools, sizes only.
- **NFR-2.4** All outbound requests go through one guarded client: scheme allowlist, resolve-then-
  reject non-public IPs **and dial the vetted IP**, ≤ 3 redirects re-vetted per hop, 5 s total /
  3 s connect, 2 MiB body cap, no proxy from env, fixed honest User-Agent.
- **NFR-2.5** Egress is default-deny in the NetworkPolicy. Adding an outbound destination is a
  reviewable change in the same commit as the tool (FR-1.9).
- **NFR-2.6** Panics are recovered per request, logged with route + `request_id`, returned as a
  `500` envelope. No stack trace ever reaches a response.

### NFR-3 — Privacy

- **NFR-3.1** No cookies for public pages. No third-party requests at runtime (fonts self-hosted,
  no CDN, no analytics).
- **NFR-3.2** Query strings are never logged. Gated tool inputs are never logged.
- **NFR-3.3** The Access email is used for audit only; no profile, no history, no stored results.

### NFR-4 — Accessibility and UX

- **NFR-4.1** Every input has a `<label for>`; errors are associated via `aria-describedby` and
  announced in a live region.
- **NFR-4.2** Keyboard-only operation of every form; visible focus rings; no keyboard traps.
- **NFR-4.3** No horizontal scroll at 360 px width; readable at 200 % zoom.
- **NFR-4.4** Colour contrast meets WCAG 2.1 AA in both light and dark schemes.
- **NFR-4.5** Dark-mode choice persists across reloads (`localStorage["scheme"]`), defaults to
  `prefers-color-scheme`, and is applied before first paint (no flash).

### NFR-5 — Supply chain and build

- **NFR-5.1** Standard library first. Any new dependency needs the `docs/design/security.md`
  dependency table updated in the same PR.
- **NFR-5.2** `go.sum` committed, `GOFLAGS=-mod=readonly`, `GOTOOLCHAIN=local`, `CGO_ENABLED=0`,
  `-trimpath`, `-ldflags="-s -w"`.
- **NFR-5.3** CI (`make check`) runs: vet, `go test -race ./...`, `govulncheck`, CSS drift
  (`make css` + `git diff --exit-code`), image build, `kustomize build` + `kubeconform`.
- **NFR-5.4** Base images pinned by digest; htmx vendored with a sibling `VERSION`; no CDN.
- **NFR-5.5** The image tag in `deploy/k8s/deployment.yaml` is written only by `release.yml`.

### NFR-6 — Availability and deployability

- **NFR-6.1** 2 replicas, PDB `minAvailable: 1`, rolling update `maxSurge: 0 / maxUnavailable: 1`,
  topology spread across nodes.
- **NFR-6.2** Runs as non-root (65532), read-only root filesystem, all capabilities dropped,
  `seccompProfile: RuntimeDefault`, no service-account token. Scratch space only in `/tmp`.
- **NFR-6.3** The binary is self-contained (`//go:embed` for templates and static assets); the
  runtime image contains one file.
- **NFR-6.4** Config is environment-only: `CF_ACCESS_TEAM_DOMAIN`, `CF_ACCESS_AUD` from a
  ConfigMap; nothing requires a writable filesystem at startup.

### NFR-7 — Convenience (a client priority, so a requirement)

- **NFR-7.1** Every tool's state lives in the URL: the same inputs reload to the same result and can
  be bookmarked, shared or opened on a phone. No server-side session is needed for this.
- **NFR-7.2** Every result can be copied as text and, where tabular, downloaded as CSV.
- **NFR-7.3** Sensible defaults: a tool must be usable by changing one field, not by filling a form.
- **NFR-7.4** Public tools need no login, no cookie and no account (NFR-3.1 already forbids the
  cookie; this makes the product intent explicit).
- **NFR-7.5** Mobile first for the two non-negotiable tools: verdict above the grid, no page-level
  horizontal scroll at 360 px (NFR-4.3).

### NFR-8 — Local development

- **NFR-8.1** `-dev` serves templates and static assets **from disk**; production uses `//go:embed`
  only. One variable selects the filesystem, and both use the same paths (repo root), so there is no
  dev-only path branching. Templates are parsed per request in dev; changes appear on refresh.
- **NFR-8.2** `-dev` MUST refuse to start when `KUBERNETES_SERVICE_HOST` is set, and log one warning
  (same rule as `TOOLS_DEV_INSECURE`). A dev build must never be deployable by accident.
- **NFR-8.3** Dev-only surface — the `/dev/reload` endpoint and the dev `<script>` tag — MUST be
  absent when `-dev` is off: the route 404s and the tag does not render. Tests assert both.
- **NFR-8.4** Rebuild-on-save is a **dev-shell tool** (`watchexec`), not a Go dependency and never in
  the image. `make dev` runs the watcher and `sass --watch` together.
- **NFR-8.5** A no-browser way to exercise every tool exists (`make list`, `make call`), so
  troubleshooting never depends on WebMCP support in a particular browser (FR-5 is a convenience,
  not the only path).

---

## 4. Acceptance scenarios

Written as the commands a reviewer runs. A milestone is done only when its row's output is pasted
into the PR body (`docs/plan.md` "done when" is the summary of these).

### A1 — M1: it runs, it is themed, it is scriptless-safe

```bash
nix develop && make css && grep -c -- --rose-500 theme/tools.css   # ≥ 1
make check                                                        # exit 0
make run & sleep 1
curl -sS localhost:8080/healthz         # ok
curl -sS localhost:8080/readyz          # ok
curl -sS localhost:8080/api/tools | jq -r '.tools[].name'         # exactly: slugify
curl -sS -X POST localhost:8080/api/tools/slugify -d '{"text":"Hello, World!"}'   # one slug
curl -sS -X POST localhost:8080/t/slugify -d 'text=Hello'         # full page, result visible
curl -sSI localhost:8080/nope                                     # 404 + themed body
curl -sSI localhost:8080/lab                                      # 404 (FR-4.3, no AUD configured)
```

Manual: the visual checklist in `docs/design/theme.md`; JS disabled; dark toggle survives reload.

### A2 — M2/M3: same binary, in the cluster

```bash
make image && docker run --rm -p 8080:8080 tools-site:check    # A1 curls pass again
kustomize build deploy/k8s | kubeconform -strict -ignore-missing-schemas -summary -
kubectl -n apps get pods -l app=tools-site                     # 2/2 Running, different nodes
kubectl -n apps port-forward svc/tools-site 8080:80 & curl -sS localhost:8080/api/tools
```

### A3 — M3b: gating is real, not decorative

```bash
curl -sSI https://tools.ciceley.cc/lab                          # 302 to Access
curl -sS  -H 'Cf-Access-Authenticated-User-Email: me@example.com' \
          https://tools.ciceley.cc/api/lab/<gated> -X POST      # 401
curl -sS  -X POST -H "Cf-Access-Jwt-Assertion: $BAD" ...        # 401, one JWKS refresh logged
CSRF cross-origin POST                                          # 403
valid JWT, same-origin, token                                   # 200
```

### A4 — M5: agents get the same tools

```bash
curl -sSI https://tools.ciceley.cc/api/tools | grep -i 'cache-control: no-store'
# browser with WebMCP: getTools() == curl /api/tools | jq '.tools[].name'
# execute("slugify", {text:"X"}) text == HTML form result text
```

### A5 — M7: the same tools over MCP

```bash
curl -sS -X POST localhost:8080/mcp -H 'content-type: application/json' \
  -d '{"jsonrpc":"2.0","id":1,"method":"tools/list"}' | jq '.result.tools[].name'
# exactly the public tools; identical names/schemas to /api/tools
curl -sS -X POST localhost:8080/mcp -H 'content-type: application/json' \
  -d '{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"slugify","arguments":{"text":"Hi"}}}' \
  | jq -r '.result.content[0].text'                     # same text as the HTML form
curl -sS -X POST localhost:8080/mcp ... '"name":"<gated>"'   # 401 without JWT
# with CF-Access-Client-Id/Secret (service token)                  # 200
```

### A6 — unconfigured / degraded

| Condition | Expected |
| --- | --- |
| no `CF_ACCESS_AUD` | `/lab*` 404, one startup warning, public zone unaffected |
| `TOOLS_DEV_INSECURE=1` + `KUBERNETES_SERVICE_HOST` | process exits non-zero with a named error |
| JWKS endpoint unreachable | gated requests `401`/`503` (fail closed), public zone unaffected |
| unknown tool name | themed 404 page / JSON `not_found` envelope |
| oversized body (>64 KiB) | `413`, logged, no partial run |

---

## 5. Per-tool spec template (required for every new tool)

Copy this block into the tool's source file as a doc comment and into its PR description.

```md
### <name> — <visibility>

**Summary:** <one sentence, ≤120 chars, no trailing period>
**Why it exists:** <one line>

| Input | Kind | Required | Rules | Default |
| --- | --- | --- | --- | --- |
| text  | Text | yes      | 1–5000 runes | — |

**Behaviour (normative):**
1. <step, deterministic>
2. <step>

**Output:** lines / files / JSON shape.
**Failure modes:** <what returns bad_input, what returns internal>.
**Outbound:** none | <host:port> via guarded client (FR-1.9), SSRF notes.
**Test vectors:** (input → exact expected output)
| Input | Expected |
| --- | --- |
| `Hello, World!` | `hello-world` |

**Requirements:** FR-1.x, FR-4.x, NFR-x.y
```

### Worked example — `slugify` (M1)

- **Visibility:** Public · **Outbound:** none · **State:** none
- **Input:** `text` (Textarea, required, 1–5000 runes)
- **Algorithm (normative):**
  1. Unicode NFKD, drop combining marks (so `Crème` → `creme`).
  2. Lowercase (`unicode.ToLower` after folding; no locale-specific rules).
  3. Replace every run of characters outside `[a-z0-9]` with a single `-`.
  4. Trim leading/trailing `-`; collapse repeats.
  5. Truncate to **80** runes, then trim a trailing `-` again.
  6. If the result is empty ⇒ `bad_input` with "Nothing to slugify".
- **Non-rules (deliberate):** no `&`→`and`, no stopword removal, no transliteration of non-Latin
  scripts (CJK/Cyrillic input yields empty ⇒ `bad_input`), no uniqueness suffix.
- **Test vectors (minimum set):**

| Input | Expected |
| --- | --- |
| `Hello, World!` | `hello-world` |
| `  Multiple   spaces  ` | `multiple-spaces` |
| `Crème Brûlée` | `creme-brulee` |
| `--Already--Slugged--` | `already-slugged` |
| `a` × 200 | 80 chars, no trailing `-` |
| `!!!` | `bad_input` |
| `日本語` | `bad_input` (documented, not a crash) |

---

## 6. Traceability

| Requirement | Milestone | Verified by |
| --- | --- | --- |
| FR-1.x registry + validation | M1 | `internal/tool` unit tests (startup failure cases) |
| FR-1.10 build-time data only | M6 | no runtime egress in the NetworkPolicy; the FX date appears in the output |
| FR-1.11 input suggestions | M6 | render test: `<datalist>` present, validation unaffected |
| FR-2.1–2.4, 2.6–2.8 pages, htmx, no-JS | M1 | handler tests + `curl` in A1 + manual JS-off |
| FR-2.5 zone isolation | M1 (scaffold), M3b (enforced) | table test over registry: every tool 404s in the wrong zone |
| FR-3.1–3.6 manifest/API | M1 | API tests; A1 `curl | jq`; A4 |
| FR-4.1 visibility derivation | M1 | router test |
| FR-4.2, 4.4, 4.5 JWT/CSRF/dev-bypass | M3b | A3 + `auth_test.go` with generated keys |
| FR-4.3 fail-closed config | M1 | A1 `/lab` 404 with no env |
| FR-5.1–5.5 WebMCP | M5 | A4 + WebMCP test matrix in `docs/context/webmcp.md` |
| FR-6.1–6.4 probes/shutdown | M1 | probe tests + SIGTERM test |
| FR-7 one-file diff | M6 | reviewer + `git diff --stat` |
| FR-8.x API-first contract | M1 | shape tests per endpoint; manifest generated-only (no fixture) |
| FR-9.1–9.6 MCP + service token | M7 | A5; gated `tools/call` 401 without token, 200 with |
| NFR-1.x limits | M1 (body/timeouts), M6 (outbound caps) | tests asserting 413/429/503 |
| NFR-2.1 headers | M1 | header test on every route |
| NFR-2.2–2.3 secrets/logging | M1 (logging), M3b (audit sub) | log capture test |
| NFR-2.4–2.5 SSRF/egress | M6 | SSRF unit tests (table of blocked IPs) |
| NFR-2.6 panic recovery | M1 | test that panics 500 with request_id |
| NFR-3.x privacy | M1 | no-set-cookie test; third-party request audit |
| NFR-4.x accessibility | M1 | a11y checklist in theme.md + manual keyboard pass |
| NFR-5.x supply chain | M1/M2 | `make check`, CI logs |
| NFR-6.x deployability | M2/M3 | A2, `kubectl` probes, drain test |
| NFR-8.x local development | M1 | test: dev routes 404 and dev tag absent without `-dev`; `make dev` reload check |

---

## 7. Open decisions (need a human; each blocks a milestone)

### 7.1 Resolved — 2026-11-14 planning session

| # | Decision | Outcome |
| --- | --- | --- |
| Q1 | invalid-input status | `200` + `data-invalid` on both paths (FR-2.4); fix the 422 sentence in `docs/design/architecture.md` |
| Q2 | tool-name charset | `^[a-z][a-z0-9_]{0,63}$` (FR-1.2) — no hyphens anywhere |
| Q3 | gated zone before Access exists | `/lab*` is not mounted without `CF_ACCESS_AUD` (FR-4.3) |
| Q4 | fonts | self-hosted in M1, CSP stays `'self'` |
| Q6 | accessibility | WCAG 2.1 AA is a **release gate** for forms, labels, focus, contrast |
| Q7 | rate limiting | first outbound/expensive tool (M6); documented exemption until then |
| Q8 | `spec.md` in the reading order | yes — `README.md` and `AGENTS.md` |
| — | private-tools address | keep one host: `tools.ciceley.cc/lab`; **no** `lab.ciceley.cc` (a second hostname doubles edge failure modes for no user benefit) |
| — | assistant address | `hire.ciceley.cc`, separate app |
| — | assistant audience | the owner as a technical recruiter; **no** candidate accounts, no public chat |
| — | chat UI timing | **last** (M10); API + tools first |
| — | AI provider | OpenAI-compatible HTTP, model configurable, key via ExternalSecret; default `deepseek-chat` |
| — | private tools in Phase A | none yet; gated tools land at M3b when a private tool exists |
| — | first two public tools | `rate_converter` and `meeting_planner` — client non-negotiables, M6 |
| — | `rate_converter` scope | amount + period + currency + target, normalised to hourly/weekly/monthly/annual, with visible assumptions; ECB daily rates fetched at build time (ADR-0009) |
| — | `meeting_planner` scope | 2–6 cities, one date, anchor-city working window, best-overlap verdict + grid, shareable URL; no drag-and-drop, no calendar integration |
| — | result rendering | one generic `Table` primitive, no per-tool templates (FR-1.7) |
| — | assistant's tools | `screen_candidate`, `interview_kit`, `draft_email`, optional `redact_cv` |
| — | redaction | **optional** — offered, tested and warned about, never silently skipped without a notice |
| — | assistant → tools-site | public tools only in v0; no service token until a private tool is genuinely needed |
| — | screening output | rubric with quoted evidence + gaps + "not stated"; no bare score; no automated rejection |
| — | quality bar | fixed 3 job descriptions × 5 CVs fixture set with output properties, run on every prompt change |
| — | assistant spend | 100 messages/day, £25/month, alert at 80 % |

### 7.2 Open

| # | Decision | Blocks | Recommendation |
| --- | --- | --- | --- |
| Q10 | Provider confirmation: I can find no "DeepSeek Flash" model — DeepSeek's API exposes `deepseek-chat` and `deepseek-reasoner`; "Flash" is a Gemini tier. Confirm the actual model or product. | M9 | Default `deepseek-chat`; base URL + model name in config so a swap is one line and one redeploy. The build must not be blocked on this: it is an env value with a default. |
| Q14 | Blog menu link. | M4 | Optional, last, separate one-line PR in `ciceleywebsite`. |
| Q15 | Where do non-EUR currencies the ECB does not publish (e.g. NGN, PKR) come from? | M6 | Ship the ECB set first and show an honest "not supported" for the rest; add a second source only when a real need appears, with its own ADR. |

See `docs/spec/hire-assistant.md` for the assistant's own requirements and open questions.

---

## 8. Change process

1. **Behaviour change** → PR edits `spec.md` first (new/edited FR/NFR, acceptance scenario), then
   the code and tests that cite the ID. A PR that changes behaviour without a spec diff is rejected.
2. **Architecture/technology decision** → ADR in `docs/adr/`, dated, with rejected alternatives.
   ADR wins over this spec on conflict.
3. **Schedule change** → `docs/plan.md` only. This spec has no dates on purpose.
4. **Ambiguity found while implementing** → add a row to §7 in the same PR, do not guess silently.
5. **A requirement is dropped** → delete it here and say why in the PR. Do not leave dead
   requirements "for later"; that is how specs rot.
