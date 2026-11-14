# ADR-0002 — Go + htmx, not a JS framework and not static pages

Date: 2026-11-12 · Status: accepted

## Context

Tools are small: take a few inputs, produce text or a file. Some need the server (outbound HTTP,
secrets), most do not. A previous sketch defined tools as YAML + a JS function, rendered by Hugo.

## Decision

One Go binary: `net/http` + `html/template` + htmx. A tool is a `tool.Tool` value; forms, fragments,
JSON API, WebMCP manifest and MCP tools all derive from it.

## Reasons

1. **Single source of truth.** YAML + JS pairs let the schema, the UI and the agent manifest drift;
   a Go value is compiled, so a missing input is a build error.
2. **The server is where gating can be enforced.** Cloudflare Access is edge convenience; verifying
   the JWT needs code.
3. **htmx needs no build step** and degrades to plain form posts, which keeps the pages usable
   without JS — including by agents that do not speak WebMCP.
4. **Operational simplicity**: one static binary, distroless image, no Node runtime, no
   `node_modules`, `CGO_ENABLED=0`.
5. Server-rendered HTML is inspectable and cacheable; nothing to hydrate.

## Costs (accepted)

- HTML is written as Go templates, and htmx idioms (fragments, `HX-Request` branching) must be
  learned.
- No type-checked client code; the JS surface is deliberately tiny (theme toggle + `webmcp.js`).

## Rejected

- **Svelte/React/Vue**: build tooling and hydration for forms; CSP hostile (`unsafe-inline`/eval).
- **Static pages + client-side tools**: cannot fetch arbitrary URLs safely, cannot hold secrets, and
  gating would be cosmetic.
- **YAML+JS tool definitions under Hugo**: two sources of truth, no compile-time checks.
- **Python/Node server**: fine technically; Go gives a static binary and matches the estate's
  familiarity with small single-purpose binaries.
