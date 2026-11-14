# CI

Two workflows, copied in spirit from `homelab-app-template` and extended. Pinned action versions;
Renovate keeps them fresh.

**Shift left (2026-11-14):** `check.yml` runs `scripts/ci.sh`, and `make ci` runs the same script
locally. There is one definition of "CI passes", so a green local run means a green PR apart from
Docker (not available in every dev environment) and network-dependent steps (govulncheck).

One gap to close at M2: the WebMCP shim test needs Node (`nodejs` in the dev shell). Until
`check.yml` adds `actions/setup-node`, `scripts/ci.sh` prints a skip line for that step instead of
pretending it ran.

## `check.yml` — pull requests

```yaml
on: { pull_request: { branches: [main] } }
jobs:
  go:
    steps:
      - checkout (submodules: recursive)   # theme submodule is needed for the CSS drift check
      - setup-go (go-version-file: go.mod, cache: true)
      - run: make css && git diff --exit-code theme/tools.css
      - run: go vet ./...
      - run: go test -race ./...
      - run: go build ./...
      - run: go run golang.org/x/vuln/cmd/govulncheck@latest ./...
  image:
    steps: [checkout, docker build -t tools-site:check .]
  manifests:
    steps: [checkout, kustomize build deploy/k8s, kubeconform -strict -ignore-missing-schemas]
```

Rationale for each:

- **CSS drift**: the theme is a pinned submodule and the palette is a copy; without this check a
  palette edit can land without regenerating `tools.css`.
- **`-race`**: tools will run handlers concurrently; a data race in a shared `Registry` or limiter is
  exactly the class of bug that survives normal tests.
- **`govulncheck`**: this app is internet-facing and behind no WAF on the API paths.
- **image build**: catches a Dockerfile that only works on the author's machine; the template did
  this and it caught real breakage.
- **manifests**: an unbuildable kustomization makes ArgoCD mark the whole Application `Unknown`.

Everything must pass before merge. `main` itself is protected by review.

## `release.yml` — push to `main`

```yaml
on:
  push: { branches: [main], paths-ignore: ["deploy/**", "docs/**", "**.md", "theme/tools.css"] }
  workflow_dispatch: {}
concurrency: { group: release-${{ github.ref }}, cancel-in-progress: false }
permissions: { contents: write, packages: write, pull-requests: write }
```

Steps: compute `TAG=main-$(date -u +%Y%m%d-%H%M%S)-${GITHUB_SHA:0:7}` → docker login → build + push
`:TAG` and `:latest` → **open a PR** that `sed`s the image line in `deploy/k8s/deployment.yaml`.

Why a PR and not a direct push (`docs/adr/0006-manifest-bump-via-pr.md`):

- `homelab-app-template` hit GH013 (`changes must be made through a pull request`) doing it directly.
- this repo's `main` protection is unknown at planning time; a PR works either way.
- if the merge is refused, the image is still published and the branch is left for one click — a
  deploy, not a failed release.

`paths-ignore` matters: without `deploy/**`, merging the bump PR would start another release, which
would open another bump PR, forever. The template learned this; keep the comment explaining it.

## Guardrails worth copying from the estate

- Nothing in CI holds cluster credentials. ArgoCD pulls; CI never pushes to the cluster.
- Secrets: only `GITHUB_TOKEN`. No PATs, no Cloudflare tokens, no Notion keys in Actions.
  (If a tool needs Notion at runtime it gets the secret via ExternalSecret in-cluster, not in CI.)
- `workflow_dispatch` always available: a re-release by hand is a normal operation, not an
  exception.

## Things CI deliberately does not do

- **No auto-merge of the bump PR.** A human sees the image tag that reaches production.
- **No `:main` floating tag** in manifests — always the timestamped tag, so a rollback is a revert.
- **No deploy from a branch other than `main`.**
- **No Cloudflare/DNS changes.** The edge is changed by a human, in the documented order
  (`docs/checklists/edge.md`), because a wrong tunnel rule takes down hostnames this repo does not own.
