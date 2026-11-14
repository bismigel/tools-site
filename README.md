# tools-site — `tools.ciceley.cc`

Repository: <https://github.com/bismigel/tools-site> (public) · CI image:
`ghcr.io/bismigel/tools-site` · ArgoCD Application: `tools-site` in namespace `apps`.

A small tool host: **Go + htmx**, styled by the same Stack theme as `ciceley.cc`, with every
tool registered with **WebMCP** so browser-capable AI agents can call it.

**Status: implemented and tested locally.** M1, M2, M3, M5, M6, M7 and basic M8 are on disk and
`make ci` is green (unit + race, JS, 68 Playwright tests, kubeconform, govulncheck). Not deployed yet:
M4 needs the cluster and the Cloudflare dashboard. See [`docs/plan.md`](docs/plan.md). A second app, the HR assistant,
lives at `hire.ciceley.cc` (`docs/spec/hire-assistant.md`).

The two tools that must exist: **currency / rate converter** and **timezone meeting planner**.

```
content/posts/  ← ciceley.cc (Hugo, separate repo, untouched by this one)
tools-site/     ← this repo: Go binary + templates + k8s manifests   ← you are here
```

Read in this order:

1. [`AGENTS.md`](AGENTS.md) — rules an AI or human must follow here.
2. [`DESIGN.md`](DESIGN.md) — the readable design document: what it does, why, and what you see at
   each step. Start here if you are not an engineer.
3. [`spec.md`](spec.md) — what the system must do, as testable requirements. Wins over `docs/design/`
   on conflict; ADRs win over it.
4. [`TECHNICAL.org`](TECHNICAL.org) — the whole engineering picture in one Org file (routes, types,
   tool contracts, security, deploy, CI, tests).
5. [`docs/plan.md`](docs/plan.md) — milestones in delivery order, what is done, what is next.
6. [`docs/context/`](docs/context/) — the estate this deploys into, and the two neighbour repos.
7. [`docs/design/`](docs/design/) — architecture, security, theme, deployment, CI.
8. [`docs/adr/`](docs/adr/) — why, with dates and rejected alternatives.
9. [`docs/glossary.md`](docs/glossary.md) — every piece of jargon used above.

Non-negotiables, in one line each:

- Public tools are **public by default**, gated tools sit behind Cloudflare Access **and** are
  re-verified in Go (JWT), never trusted from an email header.
- **API-first**: every capability has a JSON contract and reaches MCP; the HTML is a client of the
  same definition, and nothing may require JavaScript to work.
- Nothing in this repo may hand-edit the image tag in `deploy/k8s/` — CI owns it.
- The theme is `CaiJimmy/hugo-theme-stack` **v4.0.3** pinned as a submodule; the palette lives in
  a copied `custom.scss`. Forking the theme's SCSS is forbidden.
- No secrets in git. Ever. Bitwarden via External Secrets owns them.
