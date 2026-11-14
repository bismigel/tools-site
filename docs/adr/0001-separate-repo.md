# ADR-0001 — a separate repo, not a directory in `ciceleywebsite`

Date: 2026-11-12 · Status: accepted

## Context

The tools subsite could live in `ciceleywebsite/tools/` (one repo, one PR can retheme both). The
blog's release workflow builds the whole repo on every push and then `sed`s the image tag in
`deploy/k8s/deployment.yaml` and pushes to `main`.

## Decision

Separate repo, `github.com/bismigel/tools-site` (**public**, created 2026-11-12), based on
`homelab-app-template`.

## Reasons

1. **Two bumpers in one repo cross-trigger.** Site bump → commit → tools build → tools bump → commit
   → site build → … The only fix is a single workflow with path filters, permanently coupling two
   deploy paths.
2. **Blast radius.** A bad tools deploy must not be able to touch `ciceley.cc`.
3. **One Dockerfile today.** Adding a Go service means a build matrix and two images from one repo.
4. **The estate's convention is one repo per app** (`homelab-context/docs/05-deployment-patterns.md`,
   Pattern 2), and the template already ships hardened pod defaults this blog does not have.
5. `homelab-context/NEXT-STEPS.md` already records "ciceleywebsite CI redeploys on docs-only
   pushes" as a defect; adding a second app enlarges it.

## Costs (accepted)

- Theme sync becomes cross-repo: one copied `custom.scss` + a pinned submodule + a generated
  `tools.css` with a CI drift check (`ADR-0005`).
- Two ArgoCD Applications instead of one.

## Rejected

- **Monorepo with unified CI**: works, costs a day of CI work and a shared release pipeline for one
  small site.
- **Second Hugo site in this repo**: wrong stack — the tools need a server (outbound fetches,
  secrets, gating), not static generation.
