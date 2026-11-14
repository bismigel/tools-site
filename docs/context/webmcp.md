# WebMCP — what it is, what we do with it

WebMCP lets a **web page** hand an AI agent a set of callable tools. The page is the manifest; the
browser brokers the call. It is the browser-side sibling of MCP (`docs/adr/0004-webmcp-native-first.md`).

## The API we code against

```js
// presence check — the ONLY thing the page may assume
if (navigator.modelContext?.provideContext) { /* register */ }

navigator.modelContext.provideContext({
  tools: [{
    name: "slugify",                     // ^[a-z0-9_]{1,64}$, unique
    description: "Turn text into a URL slug.",
    inputSchema: { type: "object", properties: { text: { type: "string" } }, required: ["text"] },
    execute: async ({ text }) => ({ content: [{ type: "text", text: await run(text) }] })
  }]
})
```

- `provideContext({tools})` **replaces** the whole set for the document. There is no reliable
  incremental API to build on, so we always send the full list in one call.
- `execute` returns `{content: [{type:"text", text}]}` (and may set `isError`). Errors must be
  returned, not thrown.
- Nothing is persisted; a page reload re-registers.
- Useful read-back for tests/dev: `navigator.modelContext.getTools?.()` when present. Treat it as
  optional — it is not guaranteed by every implementation.

Details move fast. If the shape above disagrees with what the browser exposes, **trust the browser**
and update this file in the same PR.

## Our implementation: one shim, zero per-tool code

`web/static/js/webmcp.js` (served same-origin, no build step):

1. if `!navigator.modelContext?.provideContext` → `console.debug` once and return (no polyfill, no error);
2. `fetch('/api/tools')` → `{tools:[{name, description, inputSchema}]}`;
3. map each entry to `{...entry, execute}` where `execute` POSTs JSON to `/api/tools/{name}` and
   adapts the JSON result into the `{content:[…]}` shape;
4. `provideContext({tools})`, wrapped in try/catch — a failure to register must never break the page.

The manifest is generated from the same `tool.Tool` registry that renders the forms
(`docs/design/architecture.md`), so a tool cannot be "in the UI but not in WebMCP" or vice versa.

### Gated tools

`/lab/*` and `/api/lab/*` sit behind Cloudflare Access, so an unauthenticated page cannot fetch the
gated manifest — by construction. Therefore:

- `/api/tools` returns **public tools only**, unless the request carries a valid Access JWT, in which
  case it appends the gated ones.
- `webmcp.js` needs no special case: same-origin `fetch` carries the Access cookie automatically, so
  a gated page simply receives the larger list.
- Consequence to accept deliberately: an anonymous agent sees only the public tools. That is the
  point of gating (`docs/adr/0003-per-tool-visibility.md`).

## Why no polyfill

A polyfill (`@mcp-b/global`, `@mcp-b/webmcp-polyfill`, others) defines `navigator.modelContext` in
browsers that lack it and speaks MCP over `postMessage`. It buys availability now, and costs:
third-party code in the trust path of every tool call, a version to track, and a second semantics to
test against. Since our pages are fully usable without WebMCP (htmx forms, plain HTML), the failure
mode of "no native API" is *no agent integration*, not *broken site*.

Reversal cost is one file plus a `<script>` tag, so the decision is cheap to revisit. If it is
revisited: pin the exact version, vendor it (no CDN), record its licence and SRI hash in
`docs/design/security.md`, and keep `webmcp.js` as the only caller.

## Test matrix

| Environment | Expected |
| --- | --- |
| browser without the API | page works; one debug line in console; no JS error |
| browser with the API, public page | `getTools()` lists exactly the public tools; executing returns the same text as the HTML form |
| browser with the API, `/lab` page not logged in | Access challenge before any JS runs |
| browser with the API, `/lab` page logged in | gated tools present; `execute` succeeds with the Access cookie |
| `curl /api/tools` | public tool list only; `Cache-Control: no-store` |
| `curl /api/lab/{tool}` | 401 without a JWT, even with a forged `Cf-Access-Authenticated-User-Email` |

## Relationship to `/mcp` (M7)

Same registry, different transport: `/mcp` is JSON-RPC over streamable HTTP for agents that are not
in a browser. It is **not** public by default — think about exposure before enabling it, and reuse
the same `Visibility` field so the two surfaces cannot disagree.
