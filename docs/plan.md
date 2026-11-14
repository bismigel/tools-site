# Plan

`tools.ciceley.cc` — Go + htmx tool host, Stack-themed, WebMCP-registered.

Status legend: **done** · **next** · **todo** · **blocked**

| Milestone | Content | Done when | Status |
| --- | --- | --- | --- |
| **M0** | Decisions + this context pack | a fresh session can start M1 from `AGENTS.md` alone | **done** (2026-11-12) |
| **M1** | Repo skeleton: `flake.nix` (go, sass/dart-sass, kustomize, kubeconform, jq, curl), `go.mod`, `internal/tool` registry with `Visibility`, `internal/web` router + templates, ported `layout.html`, `theme/` CSS pipeline, one public pure tool, `/healthz`, `/readyz`, `make run` | `make run` serves the themed index and the tool form on `:8080`; `make test` green | **next** |
| **M2** | `Dockerfile`, `.github/workflows/{check,release}.yml`, `make image` | PR check builds the image; a `main` push publishes `ghcr.io/bismigel/tools-site:main-…` and opens the manifest-bump PR | todo |
| **M3** | `deploy/k8s/` (Deployment, Service, Ingress, NetworkPolicy with **egress deny**, PDB, ConfigMap, kustomization) + ArgoCD registration | pod `Ready` in `apps`; `kubectl port-forward` + `curl` returns 200; `kubeconform` clean | todo |
| **M3b** | Cloudflare Access app on `/lab*`, `/api/lab*`; JWT verification middleware; `Visibility: Gated` zone live | anonymous `GET /lab/<tool>` is challenged by Access; a forged `Cf-Access-Authenticated-User-Email` gets 401; a valid JWT gets 200 | blocked on: CF team domain + AUD tag (+ the tool list) |
| **M4** | Edge: tunnel hostname rule, DNS CNAME, `ingress-host-check` host list | `https://tools.ciceley.cc/` serves the themed index over Cloudflare TLS; the host-check CronJob has no new `IngressHostBroken` alert | todo |
| **M5** | WebMCP: `GET /api/tools` manifest, `POST /api/tools/{name}`, `web/static/js/webmcp.js` | `navigator.modelContext.getTools()` lists the public tools in a WebMCP-capable browser; executing one returns the same result as the HTML form | todo |
| **M6** | Second tool (gated, e.g. Notion search) and third (public) | the diff for each is one file in `internal/tool/` + one `Register` line, plus tests | todo |
| **M7** | `/mcp` streamable-HTTP endpoint (`tools/list`, `tools/call`) reusing the same registry | an MCP client lists and calls every tool with the same schema the manifest advertises | todo |
| **M8** | Observability: `/metrics` (Prometheus), request-id → structured logs, NetworkPolicy allowing only `monitoring` | metrics appear in Prometheus; logs carry `request_id` and never tool input values for gated tools | todo |

## Sequencing constraints (why this order)

- **M1 before M2**: the image is trivial once the binary is; the CSS pipeline has to exist before
  the Dockerfile can `COPY` its output.
- **M2 before M3**: ArgoCD must point at a path whose Deployment references an image that exists,
  otherwise the first sync is `ErrImagePull` and looks like an ingress problem.
- **M3 before M3b**: the Access app has no meaning without an Ingress behind it.
- **M4 last of the infrastructure**: if the tunnel hostname is added before the Ingress exists,
  Cloudflare returns an error page that reads like a broken app (see `docs/context/homelab.md`).
- **M5 can be done in parallel with M3b** — it only needs the manifest endpoint, not Access.

## Open (need a human answer; none block M1)

1. **Repo coordinates — decided 2026-11-12**: `github.com/bismigel/tools-site`, **public**. Public
   matches the estate's site/service repos (`bismigel-site`, `status-api`) and keeps the GHCR package
   public, so the cluster needs neither a pull secret nor an ArgoCD repository credential. It holds
   nothing secret. Flip with `gh repo edit bismigel/tools-site --visibility private` only if that
   changes: it then needs `ghcr-secret` plus a deploy key in ArgoCD.
2. **Tool list** — candidates in `docs/design/architecture.md`. Default for M1: `slugify` (pure,
   public). Confirm the gated ones before M3b.
3. **Cloudflare Access team domain + AUD tag** — created by a human in the Cloudflare dashboard for
   `tools.ciceley.cc/lab*` and `/api/lab*`. Not a secret, but I cannot create it.
4. **Blog menu link** — `[[menu.main]]` "Tools" → `https://tools.ciceley.cc` in
   `ciceleywebsite/hugo.toml` (weight 3). Separate one-line PR in the other repo.

## Definition of "tool" (keeps the scope from creeping)

A tool is a **pure-ish function with declared inputs**. It is not a page, not a dashboard, not a
wrapper around a CLI, and it has no database. If it needs persistence or a background job, it is a
different app.
