# ADR-0003 — visibility is a property of the tool, not of the host or the route

Date: 2026-11-12 · Status: accepted · Blocks: M3b

## Context

Requirement: "some features gated to Zero Trust, the rest public." Options were (a) gate the whole
host, (b) gate individual paths chosen ad hoc per tool, (c) make visibility a field on the tool and
derive the URL from it.

## Decision

(c). `Visibility` ∈ {`Public`, `Gated`}. Public tools live under `/t/{name}` and `/api/tools/{name}`;
gated tools under `/lab/{name}` and `/api/lab/{name}`. The router mounts two subtrees, so a gated
tool is *unreachable* under the public prefix. Cloudflare Access therefore needs exactly two static
path patterns (`/lab*`, `/api/lab*`), not one rule per tool.

Authentication is verified twice: Access at the edge, and the signed `Cf-Access-Jwt-Assertion`
inside the app (JWKS + `aud` + `exp`). `Cf-Access-Authenticated-User-Email` is never trusted for a
decision.

## Reasons

1. **Two rules can be reviewed once**; per-tool rules drift the moment someone adds a tool and forgets
   the dashboard.
2. **Fail-closed by construction**: forgetting to gate a tool is impossible in the dangerous
   direction — the gated zone is the only place gated tools exist.
3. **Edge-only gating is not Zero Trust.** Any pod in `apps` can call the service and set headers;
   only the JWT is unforgeable.
4. The manifest endpoint can be honest: anonymous requests see public tools only.

## Costs (accepted)

- Two URL shapes to document.
- A `Gated` tool that should have been public is invisible to agents; fixing it is a one-word change
  plus a deploy.

## Rejected

- **Gate the whole host** (`ingress-nginx-admin`): kills the point of a public tool host.
- **One Access app per tool**: N dashboard objects, N chances to forget.
- **Cookie/session auth implemented in Go**: reinventing Access, and worse.
