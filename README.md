# tools-site — `tools.ciceley.cc`

Repository: <https://github.com/bismigel/tools-site> (public) · CI image:
`ghcr.io/bismigel/tools-site` · ArgoCD Application: `tools-site` in namespace `apps`.

A small tool host: **Go + htmx**, styled by the same Stack theme as `ciceley.cc`, with every
tool registered with **WebMCP** so browser-capable AI agents can call it.

**Status: planning complete, no code yet.** The next session starts at [M1](docs/plan.md).

```
content/posts/  ← ciceley.cc (Hugo, separate repo, untouched by this one)
tools-site/     ← this repo: Go binary + templates + k8s manifests   ← you are here
```

Read in this order:

1. [`AGENTS.md`](AGENTS.md) — rules an AI or human must follow here.
2. [`docs/plan.md`](docs/plan.md) — milestones, what is done, what is next.
3. [`docs/context/`](docs/context/) — the estate this deploys into, and the two neighbour repos.
4. [`docs/design/`](docs/design/) — architecture, security, theme, deployment, CI.
5. [`docs/adr/`](docs/adr/) — why, with dates and rejected alternatives.
6. [`docs/glossary.md`](docs/glossary.md) — every piece of jargon used above.

Non-negotiables, in one line each:

- Public tools are **public by default**, gated tools sit behind Cloudflare Access **and** are
  re-verified in Go (JWT), never trusted from an email header.
- Nothing in this repo may hand-edit the image tag in `deploy/k8s/` — CI owns it.
- The theme is `CaiJimmy/hugo-theme-stack` **v4.0.3** pinned as a submodule; the palette lives in
  a copied `custom.scss`. Forking the theme's SCSS is forbidden.
- No secrets in git. Ever. Bitwarden via External Secrets owns them.
