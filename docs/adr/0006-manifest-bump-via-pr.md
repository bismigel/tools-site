# ADR-0006 — the image bump goes through a PR

Date: 2026-11-12 · Status: accepted

## Context

`release.yml` must move the deployed tag forward. Two patterns exist in the estate:

- `ciceleywebsite`: `git push` the bump commit straight to `main` (works there — `main` carries
  `GitHub Action` commits).
- `homelab-app-template`: open a PR and squash-merge it, because that repo's `main` returned
  GH013 (`changes must be made through a pull request`).

## Decision

Follow the template: bump on a `release/<tag>` branch, open a PR, attempt `gh pr merge --squash`,
and **warn without failing** if the merge is refused.

## Reasons

1. This repo's branch protection is not known at planning time; the PR path works under both
   policies. Switching to a direct push later is a 5-line simplification, not a redesign.
2. Somebody sees the tag that reaches production, which is the point of pinning timestamps rather
   than `:latest`.
3. On refusal, the image is already published and the branch is ready — a deploy in one click, not a
   failed release.
4. `paths-ignore: ["deploy/**"]` keeps the merge from starting another release (the loop the
   template's comment warns about).

## Consequences

- One extra review click per deploy.
- Requires `permissions: { contents: write, pull-requests: write }` and `workflow_dispatch` for
  re-releases.

## Rejected

- **Direct push**: fewer steps, but breaks the moment protection is enabled or a ruleset is added.
- **ArgoCD Image Updater / mutable `:main` tag**: shifts the decision into the cluster, removes the
  reviewable diff in git, and makes rollback a cluster-side action instead of a revert.
