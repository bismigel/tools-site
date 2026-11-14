# ADR-0011 — stay on htmx 2.0.11, do not move to 4.x yet

Date: 2026-11-14 · Status: accepted · Revisit: when a feature below is needed

## Context

htmx 4.0.1 exists and is a stable GitHub release, but npm's `latest` tag still points at **2.0.11**.
4.x is a dialect change: `hx-action` / `hx-source`, `<hx-partial>`, `hx-live`, built-in SSE and
WebSocket extensions, morphing, and an `htmx-2-compat` shim to soften the move. Both `hx-get` and
`hx-post` still appear in the 4.0.1 build (legacy path), so a migration is possible — it is just not
free.

This site's entire interaction surface is: a form `hx-get`s or `hx-post`s one URL, the response is a
fragment swapped into `#result`, with `hx-push-url`, `hx-indicator`, `hx-headers` and one response
header (`HX-Retarget`) for validation. No out-of-band swaps, no morphing, no SSE, no WebSockets, no
boosted navigation, no client-side state.

## Decision

- **Vendor htmx 2.0.11** (from 2.0.4), with the version in `web/static/js/VERSION`.
- Use two 2.x features we were not using, because they fix real behaviour:
  - `hx-disabled-elt="find button"` — a double click cannot fire two runs.
  - `hx-swap="… transition:true"` — the swap goes through the View Transitions API where the browser
    has it, and is a normal swap where it does not. No JavaScript of ours, no inline styles (CSP).
- **Do not migrate to 4.x** until a feature is needed that 2.x does not have.

## Reasons

1. **No feature we need is in 4.** Migrating buys capabilities this app does not use, and costs a
   rewrite of every attribute plus a compatibility shim to keep behaviour identical.
2. **`latest` is still 2.0.11.** The ecosystem (CDNs, extensions, documentation, other people's bug
   reports) is still centred on 2.x; a lone site on 4.x debugs alone.
3. **The 2.0.4 → 2.0.11 span contains fixes we want** (`hasHeader()` TypeError, history normalisation,
   OOB on detached elements, `hx-disabled-elt` restore), so staying put was not free either.
4. **Reversibility.** `webmcp.js` and the form attributes are the only htmx consumers; a later
   migration is a handful of templates, not an architecture.

## Revisit when

- we need morphing (e.g. a live-updating table), `<hx-partial>`, or SSE/WebSockets; or
- htmx 4 becomes npm `latest` and the 2-compat extension has a stable release; or
- a security advisory affects 2.0.11 with no backport.

Whichever happens, the migration is: bump the vendored file, switch attributes, keep the Playwright
suite green, and record it here.
