# ADR-0004 — WebMCP: native API only, no polyfill (for now)

Date: 2026-11-12 · Status: accepted (revisit at M5 or when an agent integration is requested)

## Context

WebMCP (`navigator.modelContext`) is a proposed browser API. Support today is partial (flags/origin
trials). Third-party polyfills exist (`@mcp-b/global`, `@mcp-b/webmcp-polyfill`) that implement the
API and speak MCP over `postMessage`.

## Decision

Ship a ~40-line `web/static/js/webmcp.js` that feature-detects, fetches `/api/tools`, and registers
tools via `provideContext`. If the API is absent: log once at debug level and do nothing. Do not
vendor a polyfill.

## Reasons

1. **Failure is benign.** The pages work without WebMCP (htmx forms, plain posts). The cost of no
   native API is "no agent integration", not "broken site".
2. **Trust path.** A polyfill is third-party JavaScript in the path of every tool call, on a host
   that also serves gated tools.
3. **One semantics to test.** A polyfill's behaviour can diverge from the native API; we would end up
   testing the polyfill.
4. **Reversal is cheap**: one file + one `<script>` tag, because `webmcp.js` is the only caller.

## Consequences

- Agent integration is unavailable in browsers without the API. Documented in
  `docs/context/webmcp.md` with the test matrix.
- If adopted later: pin the exact version, vendor it same-origin, record licence + SRI hash in
  `docs/design/security.md`, and add a polyfill column to the test matrix.

## Rejected

- **Polyfill now**: availability today, at the cost of the above for a site with no agent traffic yet.
- **Server-side MCP only (`/mcp`)**: loses the "the page is the manifest" property, which is the
  reason to use WebMCP at all. `/mcp` is still planned at M7 as a complement.
