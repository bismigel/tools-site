# Security

Written as *control → why → where*. If a control is not implemented, the milestone that owes it is
named. A PR that weakens one of these must say so in its description.

## Threat model (short, honest)

| Threat | Real here because | Primary control |
| --- | --- | --- |
| **SSRF / open proxy** | a link-preview-style tool fetches user-chosen URLs from inside the cluster | resolve + reject non-public IPs, no redirect to non-public, egress allowlist |
| **Abuse / cost** | public unauthenticated tools are free compute for anyone | per-tool rate limit, input size caps, timeouts, ingress `limit_req` already at 40 r/s |
| **Auth bypass / header spoofing** | `apps` namespace pods can reach us and set any header | verify the Access **JWT**, never the email header; fail closed |
| **CSRF on gated POSTs** | Access login is a cookie, so the browser attaches it to any site's POST | same-origin/`Sec-Fetch-Site` check + double-submit CSRF token |
| **XSS via tool output** | some tool output is user-controlled text | `html/template` escaping; `Result` has no raw-HTML field; strict CSP |
| **Secret leakage** | Notion/other creds | ExternalSecret only; never logged; never in `/api` responses |
| **Supply chain** | Go modules, htmx, base images | `go.sum`, `govulncheck`, pinned digests, Renovate, no CDN |
| **Information disclosure** | error messages, logs, endpoint enumeration | sentinel errors, no stack traces, gated tools absent from the public manifest |

Out of scope (state it rather than pretend): DDoS beyond Cloudflare's defaults, compromise of the
cluster itself, and anything an attacker with `kubectl` in `apps` can do — that actor is trusted by
definition (`docs/design/deployment.md` explains the residual risk).

## Controls

### 1. Authentication and authorisation (M3b)

- Public zone: no auth, no cookie, no session.
- Gated zone (`/lab*`, `/api/lab/*`): Cloudflare Access at the edge **and** verification in-process:
  - read `Cf-Access-Jwt-Assertion`;
  - verify RS256 with JWKS from `https://${CF_ACCESS_TEAM_DOMAIN}/cdn-cgi/access/certs`,
    cached 1 h, refetched on unknown `kid` (with a 1-per-minute floor);
  - check `aud` == `CF_ACCESS_AUD`, `exp` (+60 s leeway), `iat` not in the future, `iss` matches;
  - **missing header ⇒ 401**, malformed ⇒ 401, unknown `kid` after refresh ⇒ 401.
- Config in a ConfigMap: `CF_ACCESS_TEAM_DOMAIN`, `CF_ACCESS_AUD`. Neither is a secret.
- Dev bypass `TOOLS_DEV_INSECURE=1` **refuses to start** if `KUBERNETES_SERVICE_HOST` is set, and
  logs a warning on every request.
- The email claim is used only for audit logging, never for an authorisation decision.

### 2. CSRF (gated POSTs, M3b)

Access authenticates with a cookie, so a POST from another origin arrives authenticated.

- Require `Sec-Fetch-Site: same-origin` (or, if absent, an `Origin` header matching the configured
  base URL).
- Require `X-Tools-CSRF: <token>` where the token is per-session, issued into the page and echoed by
  htmx via `hx-headers`. Token = HMAC(session id from the JWT `sub` + per-process secret, 10 min).
- Reject with 403 and a logged `request_id`. Public POSTs need no token (no ambient credential to
  abuse) but still get the origin check.

### 3. SSRF guard (only for tools that fetch, M6)

Applies to **every** outbound request made on a user's behalf:

- scheme allowlist `http`, `https`;
- resolve the host, then reject if any address is loopback, private, link-local, multicast,
  unspecified, or IPv6 ULA/link-local — **and dial the vetted IP**, so DNS rebinding between check
  and dial fails;
- redirects: max 3, re-vetted each hop;
- timeouts: 5 s total, 3 s connect; response body cap 2 MiB; decompressed cap 4 MiB;
- no proxies from the environment (`Proxy: nil`), no `file://`, no `gopher://`;
- user agent fixed and honest (`tools-site/1.0 (+https://tools.ciceley.cc)`).

An outbound tool also requires: an egress rule in the NetworkPolicy naming the destination and port,
and the tool's entry in the table above. Both in the same commit.

### 4. Input handling

- Request body cap **64 KiB** (`http.MaxBytesReader`) on every POST; JSON decoder with
  `DisallowUnknownFields` where a struct is decoded.
- Per-field caps from `Input.Min/Max`; string length counted in runes; 20 fields max per tool.
- Number inputs bounded; `Select` values must be in `Options`.
- Anything that becomes a filesystem path, header value, URL or command is refused unless the tool
  documents it — no tool shells out (`exec`) at all in v1.

### 5. Response headers (middleware, every response)

```
Content-Security-Policy: default-src 'none'; script-src 'self'; style-src 'self';
  img-src 'self' data:; font-src 'self'; connect-src 'self'; form-action 'self';
  base-uri 'none'; frame-ancestors 'none'
X-Content-Type-Options: nosniff
Referrer-Policy: strict-origin-when-cross-origin
Cross-Origin-Opener-Policy: same-origin
Cross-Origin-Resource-Policy: same-origin
Permissions-Policy: geolocation=(), microphone=(), camera=()
```

- `Strict-Transport-Security` is set **at Cloudflare**, not here (we serve plain HTTP inside the
  tunnel; setting HSTS in-cluster has no effect and misleads readers).
- `style-src 'self'` means no inline `style=` attributes from templates. Keep tool styling in
  `tools.css`.
- Google Fonts would require `style-src`/`font-src` to include `fonts.googleapis.com` /
  `fonts.gstatic.com`. **Prefer self-hosting the two families** at M1 (a 40 kB woff2 pair) so the CSP
  stays `'self'` and no third party sees readers' IPs. If that is deferred, add the two origins and
  note it here.

### 6. Logging and privacy

- Log: timestamp, `request_id`, method, path template (`/t/{name}`, never the raw path), status,
  duration, tool name, outcome, `cf_ray`, and for gated requests the JWT `sub` (email) — nothing else.
- **Never log tool input values or output** for gated tools; for public tools log only input sizes.
- No query strings in access logs (they can carry user text).
- Errors logged with `%v` on the wrapped error, never with the request body.

### 7. Rate limiting and cost

- Ingress already rate limits per `host:clientIP` (40 r/s, burst 200, 429). Do not duplicate that
  globally in Go.
- Add a **per-tool, per-client** limiter (token bucket, keyed `CF-Connecting-IP` falling back to the
  socket address) only for tools that are expensive or outbound: default 10 req/min per client,
  configurable per tool, `429` + `Retry-After`.
- Per-tool `context.WithTimeout` (default 5 s, 10 s for outbound tools).
- Concurrency cap per tool (semaphore, default 4) so one tool cannot starve the process.

### 8. Supply chain and build

- `go.sum` committed; `GOFLAGS=-mod=readonly`; `GOTOOLCHAIN=local`.
- CI: `go vet`, `go test -race`, `govulncheck ./...`, and a fail on any diff after `make css`.
- Base images pinned by digest; builder `golang:1.x-alpine@sha256:…`, runtime
  `gcr.io/distroless/static-debian12:nonroot@sha256:…`. Renovate bumps both. Justification recorded in
  the PR, not silently floated.
- `CGO_ENABLED=0`, `-trimpath`, `-ldflags="-s -w"`.
- htmx is vendored into `web/static/js/htmx.min.js` with its version in a sibling `VERSION` file;
  upgrade = replace file + version + note here. No CDN, no SRI question (same origin).
- No `init()` side effects, no `plugin`, no cgo.

### 9. Kubernetes (see `docs/design/deployment.md` for the manifests)

`runAsNonRoot` (uid 65532), `readOnlyRootFilesystem`, `allowPrivilegeEscalation: false`,
`drop: [ALL]`, `seccompProfile: RuntimeDefault`, `automountServiceAccountToken: false`, no hostPath,
no hostNetwork, requests == limits for memory, PDB, probes.

Residual risk to write down rather than hide: NetworkPolicy is **namespaced** — any pod in `apps`
can call the tools app and can forge `Cf-Access-*` headers. The JWT check is what makes that
survivable, which is why header trust is forbidden.

## Dependency policy

Allowed: Go stdlib; `golang.org/x/…` when stdlib genuinely lacks it; htmx (front end).
Everything else needs a paragraph in the PR: what it does, why stdlib is insufficient, and its
maintenance signal. Record it in the table below **in the same PR**.

| Dependency | Version | Why | Alternative rejected |
| --- | --- | --- | --- |
| `html/template` | stdlib | escaping by construction | templ/jet: build step, extra API surface |
| `net/http` | stdlib | mux + htmx is just HTTP | chi/gin: not needed for ~10 routes |
| htmx | 2.x (vendored) | fragment swaps without JS | writing XMLHttpRequest by hand |
| (JWT verification) | TBD at M3b | prefer stdlib `crypto/rsa` + `encoding/json` over a library | `golang-jwt`: 1 dep, well known; decide with a note |
