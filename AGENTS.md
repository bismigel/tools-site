# AGENTS.md — instructions for AI/LLM agents

You are in **`tools-site`**, the repo behind `https://tools.ciceley.cc`. It is a separate repo
from `ciceleywebsite` (the Hugo blog) on purpose — see `docs/adr/0001-separate-repo.md`.

## Start here (5 minutes, in this order)

1. `spec.md` — what the system must do, as testable requirements. If code and spec disagree, one of
   them is a bug; amend the spec in the same PR as the behaviour change.
2. `docs/plan.md` — the milestone table. **Find the first milestone that is not done.** Do that.
3. `docs/design/architecture.md` — how a tool becomes a page, an API, and a WebMCP entry.
4. `docs/design/security.md` — the controls that are not optional.
5. `docs/context/homelab.md` — the cluster, ArgoCD, GHCR, Cloudflare facts you will otherwise guess wrong.
6. `docs/spec/hire-assistant.md` — only when the milestone is M9/M10.

Do not read the whole repo before starting. M1 needs: `internal/tool/`, one tool,
`web/templates/layout.html`, `Makefile`, `theme/`.

## Hard rules

1. **One definition per tool.** A tool is a `tool.Tool` value: name, summary, inputs, `Run`.
   Forms, htmx fragments, the JSON manifest, the WebMCP entry and the MCP tool all derive from
   it. If you find yourself writing a per-tool template, handler, CSS rule or JS file, stop —
   the design is wrong.
2. **Never hand-edit the image tag** in `deploy/k8s/deployment.yaml`. `.github/workflows/release.yml`
   owns it and will fight you.
3. **Never edit files under `theme/hugo-theme-stack/`.** It is a submodule pinned to `v4.0.3`.
   Palette changes go in `theme/site-scss/custom.scss` (a copy of the blog's file, with the same
   contents — see `docs/design/theme.md`).
4. **No secrets, no tokens, no `.env` values in git or in Kubernetes manifests.** Non-secret config
   → ConfigMap. Secrets → ExternalSecret backed by Bitwarden (`docs/context/homelab.md`).
5. **Public vs gated is a property of the tool**, not of the route you happen to build. Set
   `Visibility`; the router and the WebMCP manifest follow it. Adding an Access rule per tool is a bug.
6. **Do not add a dependency** without recording it in `docs/design/security.md` ("Dependency
   policy") and preferring stdlib. htmx is the one allowed front-end library.
7. **Do not enable egress** for a tool unless `docs/design/security.md` lists the destination and
   the NetworkPolicy is updated in the same commit, with the SSRF guard in place.
8. **Do not create a `Namespace` object.** `apps` is owned centrally in `bakuran-gitops`.
9. **Behaviour changes edit `spec.md` first.** Architecture or technology decisions get an ADR.
   `docs/plan.md` carries dates and status; the spec carries no dates on purpose.

## Commands

There is **no `flake.nix` yet** — writing it is the first task of M1 (packages: `go`, `sass`
(dart-sass), `kustomize`, `kubeconform`, `jq`, `curl`, `git` for the submodule, `watchexec` for
the dev loop, and `nodejs` for the WebMCP shim test). Until it exists, the commands below assume those binaries are on `PATH`.

```bash
nix develop                     # go, sass (CSS), kustomize, kubeconform, jq, curl, watchexec
make css                        # regenerate theme/tools.css from the pinned theme + palette
make dev                        # hot reload: rebuild on .go change, templates/CSS from disk
make list                       # list tools via /api/tools (no browser, no WebMCP)
make call TOOL=slugify TEXT=hi  # call a tool via JSON (the no-browser fallback)
make run                        # build + serve on :8080 (no hot reload)
make test                       # go vet, go test -race ./..., govulncheck
make check                      # vet, test -race, css drift, build
make e2e                        # Playwright: desktop + mobile, JS and no-JS
make ci                         # scripts/ci.sh — the same script the PR workflow runs
make image                      # docker build (does not push)
```

`make css` is not optional after touching `theme/site-scss/custom.scss`, and CI fails if the
committed `theme/tools.css` differs from a fresh generation.

## Validate before pushing

```bash
make check                                     # vet, test, govulncheck, css drift, kustomize
kubeconform -strict -ignore-missing-schemas <(kustomize build deploy/k8s)
```

## Things that will bite you (learned, not guessed)

- **ArgoCD will reject the Application** with `InvalidSpecError` if the repo URL is not in
  `sourceRepos` of the `prod` AppProject in `bakuran-gitops`. That is a required, separate change
  (`docs/checklists/deploy.md`).
- **A hostname that resolves but has no Ingress returns 404** and *looks* fine from outside if
  anything sits in front of it. `bakuran-gitops` has a CronJob (`ingress-host-check`) with a
  hardcoded host list; add `tools.ciceley.cc` or breakage is invisible.
- **The template's NetworkPolicy is ingress-only**, so egress is *allowed* by default. "We have a
  NetworkPolicy" does not mean egress is restricted.
- **Read-only root filesystem**: the container gets no writable `/` — anything that needs scratch
  space uses `/tmp` (`emptyDir`) or memory.
- **`/readyz` is not `/healthz`.** Liveness must stay dependency-free or a Cloudflare hiccup
  restarts a healthy pod.
- **Cloudflare Access header spoofing**: `Cf-Access-Authenticated-User-Email` is trivially forged by
  anything inside the `apps` namespace. Only the signed JWT counts.

## Definition of done

Every milestone in `docs/plan.md` has a "done when" column. Do not mark a milestone done from
reading code — run the command in that column and paste the output in the PR body.
