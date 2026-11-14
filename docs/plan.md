# Plan

`tools.ciceley.cc` — Go + htmx tool host, Stack-themed, WebMCP- and MCP-registered.
`hire.ciceley.cc` — private HR assistant (separate repo; `docs/spec/hire-assistant.md`).
Readable overview of the whole system: `DESIGN.md`.

Status legend: **done** · **next** · **todo** · **blocked**

**Row order is delivery order.** Milestone IDs are historical and are referenced by
`docs/checklists/` — do not renumber them.

| Milestone | Content | Done when | Status |
| --- | --- | --- | --- |
| **M0** | Decisions + this context pack | a fresh session can start M1 from `AGENTS.md` alone | **done** (2026-11-12) |
| **M1** | Repo skeleton: `flake.nix` (go, sass/dart-sass, kustomize, kubeconform, jq, curl), `go.mod`, `internal/tool` registry with `Visibility`, `internal/web` router + templates, ported `layout.html`, `theme/` CSS pipeline, the generic `Table` result primitive (`spec.md` FR-1.7, needed by M6), one public pure tool (`slugify`), `/healthz`, `/readyz`, **`-dev` hot reload** (disk-backed templates/CSS, `make dev`, `make list`, `make call` — NFR-8), **the JSON manifest and execute endpoint as part of M1** (FR-8), `make run` | `make run` serves the themed index, the tool form and the same tool over `/api/tools/slugify` on `:8080`; `make test` green; the `slugify` test vectors in `spec.md` §5 pass; editing a template reloads without a restart | **done** (2026-11-14) — dark toggle click + 360 px still to eyeball |
| **M5** | WebMCP: `web/static/js/webmcp.js` on top of the M1 endpoints; `make call` stays the no-browser fallback | `navigator.modelContext.getTools()` lists the public tools in a WebMCP-capable browser; executing one returns the same text as the HTML form | **next** — bridge + JS tests landed; needs one WebMCP-capable browser check |
| **M2** | `Dockerfile`, `.github/workflows/{check,release}.yml`, `make image` | PR check builds the image; a `main` push publishes `ghcr.io/bismigel/tools-site:main-…` and opens the manifest-bump PR | todo |
| **M3** | `deploy/k8s/` (Deployment, Service, Ingress, NetworkPolicy with **egress deny**, PDB, ConfigMap, kustomization) + ArgoCD registration + **basic `/metrics` and an alert on 5xx rate** | pod `Ready` in `apps`; `kubectl port-forward` + `curl` returns 200; `kubeconform` clean; Prometheus scrapes the target | todo |
| **M4** | Edge: tunnel hostname rule, DNS CNAME, `ingress-host-check` host list | `https://tools.ciceley.cc/` serves the themed index over Cloudflare TLS; the host-check CronJob has no new `IngressHostBroken` alert | todo |
| **M6** | **`rate_converter`** (currency + hourly/weekly/monthly/annual, visible assumptions, ECB rates embedded at build time — ADR-0009) and **`meeting_planner`** (2–6 cities, one date, anchor working window, best-overlap verdict + grid). Client non-negotiables. | both live; usable from a phone with JS disabled; a shared URL reproduces the same answer; the FX date is visible; `meeting_planner` flags a DST change and a no-overlap day correctly; each diff is one file + one `Register` line | todo |
| **M3b** | Cloudflare Access app on `/lab*`, `/api/lab*`; JWT verification middleware; `Visibility: Gated` zone live | anonymous `GET /lab/<tool>` is challenged by Access; a forged `Cf-Access-Authenticated-User-Email` gets 401; a valid JWT gets 200 | blocked on: CF team domain + AUD tag — **and on there being a private tool** (none yet) |
| **M7** | `/mcp` streamable-HTTP endpoint (`tools/list`, `tools/call`) reusing the same registry; Access service-token acceptance for server-to-server callers | an MCP client lists and calls every tool with the same schema the manifest advertises; a gated `tools/call` is 401 without a token and 200 with one | todo |
| **M8** | Observability, completed: dashboards (traffic, p95, errors, per-tool use), request-id log search, FX file-age alert, and **provider spend alerts** once `hire` exists | the dashboards exist; a log line can be found by `request_id`; a spend alert fires on a forced test; a stale FX file alerts before the page lies | todo — basic metrics land at M3 |
| **M9** | **`hire` v0**: separate repo, `hire.ciceley.cc`, API-first tools only (`merge_profile`, `screen_candidate`, `interview_kit`, `draft_email`, optional `redact_cv`), no chat UI | `hire.ciceley.cc/healthz` 200 behind Access; each capability returns a schema-valid answer on the 3×5 fixture set with the required properties; `merge_profile` marks every field agreed/conflict/single-source/missing and rejects a pasted URL with instructions; redaction is offered and warned about; spend guard enforced | todo — blocked on `DESIGN.md` §4–§5 sign-off + M7 (optional) |
| **M10** | **`hire` chat UI** — last, deliberately | the owner can hold a multi-turn session in the browser; the transcript never reaches the server beyond the turn in flight; copy/download works; the wizard fallback still passes the same fixtures | todo |

## Sequencing constraints (why this order)

- **M1 before M5**: WebMCP has nothing to register until the manifest and execute endpoints exist.
  M5 is a ~50-line script, so it is the cheapest thing to pull forward — the owner asked for it
  during development.
- **M1 before M2**: the image is trivial once the binary is; the CSS pipeline has to exist before
  the Dockerfile can `COPY` its output.
- **M2 before M3**: ArgoCD must point at a path whose Deployment references an image that exists,
  otherwise the first sync is `ErrImagePull` and looks like an ingress problem.
- **M3 before M4**: a hostname with no Ingress behind it returns a Cloudflare error that reads like a
  broken app (`docs/context/homelab.md`).
- **M4 before M6 — a choice, not a constraint.** The opposite is defensible: build the real tools on
  the laptop first. Deploying first is chosen because "adding a tool is one file and one line" is the
  design's central claim, and proving it on a live system is worth more than a third tool on a laptop.
- **M6 before M7**: MCP proven on one toy tool tests nothing. Two real tools with tables, selects and
  defaults is a fair test of the schema generator.
- **M3b whenever the first `Gated` tool exists.** It is not a launch blocker: `/lab*` is not mounted
  at all until the Access configuration exists (FR-4.3), so there is no window in which the private
  zone is accidentally public. With no private tool yet, there is nothing to protect.
- **M7 before M9**: `hire` consumes the tools; the MCP surface is how it does that.
- **M9 before M10** by the owner's decision — the API and the prompts are the hard part; the chat
  window is the easy part and can be swapped for the wizard without touching the tools.
- **M8 basic at M3, complete after M9**: the assistant spends money, so spend visibility cannot be
  the last milestone.

## Deliberate changes from the 2026-11-12 plan

| Change | Why |
| --- | --- |
| API endpoints and schema tests moved **into** M1 | API-first (`spec.md` FR-8); the JSON surface can no longer be an afterthought |
| WebMCP moved from "after deploy" to "right after M1" | the owner wants it as a development/debugging window |
| **M6 is now the client's two non-negotiables** (`rate_converter`, `meeting_planner`), not "reading time / timezone overlap" | they are the reason the site exists; reading time becomes a candidate for a fourth tool |
| **M4 before M6** | prove "one file, one line" on a live system |
| **M3b moved after M6** | with no private tool, the private zone protects nothing; fail-closed makes deferring it safe |
| A generic `Table` result primitive added to M1 | the two non-negotiables are grids; per-tool templates are forbidden |
| Exchange rates at build time (ADR-0009) | keeps a public convenience tool public, needs no egress or secret, makes tests deterministic |
| Observability split: basic at M3, complete at M8 | a paid model API with no spend/error visibility is a liability |
| New M9/M10 for `hire` in a separate repo | ADR-0008 |
| Redaction is **optional**, offered and warned about | client decision; the residual data-transfer responsibility is the owner's and is written down (DESIGN.md §5) |
| Profile enrichment added: paste-the-text merge with provenance | client request; the URL-fetch and vendor-API versions are rejected and recorded (ADR-0010) |
| Private tools stay on `tools.ciceley.cc/lab` | one hostname = one set of edge failure modes (`spec.md` §7.1) |

## Open (need a human answer)

1. **Repo coordinates — decided 2026-11-12**: `github.com/bismigel/tools-site`, **public**. Public
   matches the estate's site/service repos and keeps the GHCR package public, so the cluster needs
   neither a pull secret nor an ArgoCD repository credential. It holds nothing secret. The `hire` repo
   is the exception: **private** (prompts + product spec about candidate handling).
2. **Cloudflare Access team domain + AUD tag** — created by a human in the Cloudflare dashboard for
   `tools.ciceley.cc/lab*` and `/api/lab*`. Not a secret, but only a human can create it. Needed at
   M3b, and M3b is only needed when the first gated tool exists.
3. **Access service token for `hire`** — only if `hire` ever calls a **gated** tool. v0 calls public
   tools and needs none. When needed: created by a human, stored in Bitwarden, delivered by
   ExternalSecret.
4. **Model provider name** — "DeepSeek Flash" is not a model I can find; the API exposes
   `deepseek-chat` and `deepseek-reasoner`. Default is `deepseek-chat`; confirm at leisure, it is an
   environment value (`spec.md` Q10).
5. **Non-ECB currencies** (NGN, PKR, …) — unsupported at first and labelled as such. A second source
   needs its own ADR (`spec.md` Q15).
6. **Blog menu link** — `[[menu.main]]` "Tools" → `https://tools.ciceley.cc` in
   `ciceleywebsite/hugo.toml` (weight 3). Separate one-line PR in the other repo, optional, last.

## Definition of "tool" (keeps the scope from creeping)

A tool is a **pure-ish function with declared inputs**. It is not a page, not a dashboard, not a
wrapper around a CLI, and it has no database. If it needs persistence or a background job, it is a
different app — which is exactly why `hire` is one (`docs/adr/0008`). A tool may read **dated
build-time data** embedded in the binary; it may not fetch at runtime unless it is gated (FR-1.10).
