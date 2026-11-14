# Glossary

Every piece of jargon in this repo, in one sentence. Written because the plan was first discussed
with someone who reasonably asked what "slugify", "link preview" and "polyfill" meant.

**ADR** — Architecture Decision Record: a short dated file saying what was chosen, why, and what was
rejected. See `docs/adr/`. Purpose is to stop a future session re-litigating a settled choice.

**AppProject (ArgoCD)** — an allowlist that says which git repos and namespaces an Application may
use. Ours is `prod`; it is why a new repo must be added to `sourceRepos` before ArgoCD will sync.

**App-of-apps** — the ArgoCD pattern where one Application creates the other Applications, by
pointing at a directory of them. `bakuran-gitops/clusters/prod/root.yaml` is ours.

**CF-Access-Jwt-Assertion** — the signed JWT Cloudflare Access attaches to a request it has already
authenticated. The only trustworthy "who is this" signal we receive. The accompanying
`Cf-Access-Authenticated-User-Email` header is *not* signed and is therefore forgeable.

**CSRF** — Cross-Site Request Forgery: another site making your browser perform a state-changing
POST to this one. A risk here because Access authentication is cookie-based, so any state-changing
tool must check origin/CSRF token.

**distroless** — a base image containing the app and its runtime but no shell, package manager or
coreutils. Smaller attack surface; the cost is that you cannot `exec sh` into it — use probes and
logs instead.

**emptyDir** — an ephemeral Kubernetes volume. Used here as `/tmp` because the root filesystem is
read-only.

**ESO / External Secrets Operator** — pulls secrets from Bitwarden into Kubernetes Secrets. This
repo only ever commits an `ExternalSecret` reference, never a value.

**GHCR** — GitHub Container Registry, `ghcr.io/bismigel/tools-site`. CI pushes
`main-<timestamp>-<sha>` tags; a public package means the cluster needs no pull secret.

**Hugo** — the static-site generator behind `ciceley.cc`. Not used to *serve* this app; it is used
at build time to compile the theme CSS.

**htmx** — a ~14 kB script that lets HTML attributes (`hx-post`, `hx-target`) do what would
otherwise need a front-end framework: it fetches an HTML fragment from the server and swaps it in.
Server renders HTML; no JSON-by-JS rendering, no build step.

**ingress-nginx** — the cluster's HTTP entry point. Public hostnames reach it only through the
Cloudflare tunnel, on a loopback NodePort.

**JSON Schema** — the machine-readable description of a tool's inputs. `Input` structs generate it;
it is what the WebMCP manifest advertises and what the HTML form is rendered from.

**JWKS** — the public-key set used to verify JWTs, fetched from
`https://<team>.cloudflareaccess.com/cdn-cgi/access/certs` and cached.

**JWT** — a signed token. Here: Cloudflare Access's proof that you logged in, verified with RS256
against the team's JWKS and checked against the Access application's `aud` tag.

**Link preview** — paste a URL, server fetches the page, shows title/description/image. Needs
outbound HTTP, therefore an SSRF guard and therefore a **gated** placement (a public URL fetcher is
an open proxy).

**MCP** — Model Context Protocol: the JSON-RPC standard agents use to list and call tools. WebMCP is
its browser-facing sibling.

**Polyfill** — a JavaScript file that implements a browser API the browser does not have yet, so
code written against the new API works today. Chosen against, for now
(`docs/adr/0004-webmcp-native-first.md`): it means shipping third-party code in front of the tools.

**Registry (tool)** — the in-memory list of `tool.Tool` values, built once at startup. The single
source of truth for the form, the API, the manifest and MCP.

**Slugify** — turn free text into a URL-safe slug: `Writing With a Full Stomach!` →
`writing-with-a-full-stomach`. Chosen as the first tool because it needs no network, no secret and
no configuration — it proves the plumbing and nothing else.

**SSRF** — Server-Side Request Forgery: tricking the server into fetching an internal address
(`127.0.0.1`, `169.254.169.254`, `10.0.0.0/8`, `*.svc.cluster.local`). Mitigation: resolve, reject
non-public IPs, dial the resolved IP, cap redirects and body size.

**Stack theme** — `CaiJimmy/hugo-theme-stack`, pinned `v4.0.3`. Provides the CSS this app reuses and
the `baseof.html` markup that `web/templates/layout.html` was ported from.

**submodule** — a git repo nested inside another, pinned to a commit. `theme/hugo-theme-stack` is one.

**WebMCP** — a proposed browser API, `navigator.modelContext`, letting a page register tools an AI
agent can call. Opt-in, not universally available; absent → our shim does nothing and the page
still works.

**Zero Trust** — no implicit trust from network position. Concretely here: Cloudflare Access decides
who may reach `/lab*`, and the app re-verifies the signed JWT instead of believing a header.
