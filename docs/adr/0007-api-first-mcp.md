# ADR-0007 — API-first, and MCP as a first-class surface

Date: 2026-11-14 · Status: accepted · Blocks: M1 (contract), M7 (MCP), M9 (hire app)

## Context

The client's requirement, in their words: *"I want everything to be API first so the AI assistant
inside should be able to tool-call it when I build its own MCP."*

Read literally that means the HTML pages should call a JSON API from the browser. Read as an
architecture rule it means something better: **the capability and its schema are the product; HTML,
JSON, WebMCP and MCP are four renderings of one definition.** The second reading keeps the site
working without JavaScript, which is already a requirement.

A second consequence: `hire` (the HR assistant, ADR-0008) is a server-side caller. It has a
browser only for its own UI, not for calling `tools-site`. So the tool host needs a machine-to-machine
way in that Cloudflare Access will accept, and the MCP surface has to arrive before the assistant
does — not after.

## Decision

1. **One definition, four renderings.** `tool.Tool` is the source. The HTML form, the JSON manifest,
   the `POST /api/...` endpoints, WebMCP and MCP are all derived. A capability that exists in only
   one rendering is a defect (spec FR-8.1).
2. **The JSON shapes are the contract**, and changes are **additive only**. No path versioning yet:
   with one consumer (`hire`) an additive rule is enough, and a breaking change gets a new path
   *and* an ADR when it happens. (Rejected: `/api/v1` today — it would touch six documents for no
   benefit yet.)
3. **No JavaScript requirement.** "API-first" does not mean the browser must run JS; the HTML is
   server-rendered from the same definition and stays complete on its own (FR-8.6).
4. **MCP is scheduled early** (M7, right after the second/third tool), because the assistant needs it
   to tool-call. Same registry, same schemas, no MCP-specific tool definitions.
5. **Server-to-server auth is a Cloudflare Access service token**, not a hand-rolled API key:
   `CF-Access-Client-Id` / `CF-Access-Client-Secret`, stored by ExternalSecret, verified in-process,
   accepted only on `/mcp` and `/api/lab*`, never treated as a human identity, rate-limited per token.

## Reasons

- Four renderings of one definition is the only version of this that survives the second tool. Any
  hand-written per-surface schema drifts within a month (AGENTS.md rule 1).
- The service token keeps identity in one place. A bespoke API-key scheme would be a second auth
  system to rotate, audit and get wrong, and would not be covered by the Zero Trust policy.
- Additive-only is enforceable cheaply: a shape test per endpoint plus a review rule. Path versions
  are only worth it when a consumer cannot be updated in lockstep, which is not the case for one
  in-house app.

## Costs (accepted)

- Every new capability needs a schema test and a manifest test; M7 lands earlier than originally
  planned.
- A service token is one more secret to rotate, and it widens what `/mcp` accepts. Mitigated by
  scope (two path prefixes), per-token rate limiting, and audit logging that distinguishes token
  callers from humans.
- No path versioning means a breaking change is a planned event, not a routine one.

## Rejected

- **HTML that fetches JSON from the browser** (literal "API-first"): breaks the no-JS requirement,
  adds a client-side rendering path, and buys nothing the server does not already do.
- **A separate hand-written MCP schema**: two definitions, guaranteed drift.
- **An internal cluster-only HTTP call that bypasses Access**: any pod in `apps` (or anything that
  reaches the Service) would be implicitly trusted — exactly the assumption ADR-0003 rejects.
- **Static API keys in a ConfigMap**: not a secret store; not rotatable without a redeploy of both
  apps.
