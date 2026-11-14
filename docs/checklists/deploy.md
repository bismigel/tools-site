# Checklist — M1 to production

Tick in order. Each line is a command whose output you can paste into a PR body. If a step is not
applicable yet, write why instead of deleting the line.

## 0. Repo

- [ ] `gh repo create bismigel/tools-site --public --source . --push` (or copy `homelab-app-template`
      and run `./scripts/init.sh tools tools.ciceley.cc 8080 apps ciceley` **in a fresh tree**, then
      read the diff)
- [ ] `git submodule add https://github.com/CaiJimmy/hugo-theme-stack theme/hugo-theme-stack`
- [ ] `cd theme/hugo-theme-stack && git checkout v4.0.3` · `git -C ../.. commit`
- [ ] `cp ~/projects/ciceleywebsite/assets/scss/custom.scss theme/site-scss/custom.scss`
- [ ] first commit: docs + `theme/site-scss/custom.scss` + submodule

## 1. M1 — it runs and it looks right

- [ ] `nix develop` · `make css` → `grep -c -- --rose-500 theme/tools.css` ≥ 1
- [ ] `make test` → vet + `-race` + govulncheck clean
- [ ] `make run` → `curl -sS localhost:8080/healthz` = `ok`
- [ ] `curl -sS localhost:8080/api/tools | jq .` → exactly the public tools
- [ ] browser: index is themed, dark toggle works, `slugify` returns a result
- [ ] gated tool (if any) 404s under `/t/`; public tool 404s under `/lab/` (test asserts this)
- [ ] no-JS check: `<form method=post action=/t/slugify>` works with JS disabled
- [ ] visual checklist in `docs/design/theme.md` all ticked
- [ ] `git diff --exit-code theme/tools.css` after a second `make css` (determinism)

## 2. M2 — image and CI

- [ ] `make image` → image builds locally
- [ ] `docker run --rm -p 8080:8080 tools-site:check` → same responses as `make run`
- [ ] PR → `check.yml` green (CSS drift, vet, test, govulncheck, image, manifests)
- [ ] merge to `main` → `release.yml` pushed `ghcr.io/bismigel/tools-site:main-…`
- [ ] bump PR opened automatically (and `paths-ignore` prevents a second release)
- [ ] GHCR package visibility set deliberately (public ⇒ no pull secret)

## 3. M3 — cluster

- [ ] `kustomize build deploy/k8s | kubeconform -strict -ignore-missing-schemas -summary -` → 0 errors
- [ ] `kubectl diff -f <(kustomize build deploy/k8s) --namespace apps` reviewed
- [ ] `bakuran-gitops`: `clusters/prod/projects/projects.yaml` → repo added to `sourceRepos`
- [ ] `bakuran-gitops`: `clusters/prod/apps/workloads/tools-site.yaml` added
- [ ] `argocd app get tools-site` → `Synced` / `Healthy`
- [ ] `kubectl -n apps get pods -l app=tools-site -o wide` → 2/2 Ready, different nodes
- [ ] `kubectl -n apps port-forward svc/tools-site 8080:80` → `curl localhost:8080/robots.txt`
- [ ] `kubectl -n apps rollout restart deploy/tools-site` → new pod serves (read-only rootfs ok)
- [ ] NetworkPolicy proof: from an unrelated pod, `curl` the service on `:8080` — document the result
      (same namespace is allowed by design; JWT is what protects gated tools)

## 4. M3b — gated zone

- [ ] Access application created on `tools.ciceley.cc` for `/lab*` and `/api/lab*`
- [ ] ConfigMap has `CF_ACCESS_TEAM_DOMAIN`, `CF_ACCESS_AUD` (not secrets)
- [ ] anonymous `curl -I https://tools.ciceley.cc/lab` → 302 to Access
- [ ] forged header `Cf-Access-Authenticated-User-Email: me@example.com` → **401** from the app
- [ ] valid JWT → 200; expired JWT → 401; unknown `kid` → one JWKS refresh then 401
- [ ] `TOOLS_DEV_INSECURE=1` refuses to start with `KUBERNETES_SERVICE_HOST` set
- [ ] CSRF: cross-origin `POST /api/lab/x` → 403; same-origin with token → 200

## 5. M4 — edge

- [ ] Ingress has an address; in-cluster probe is 200 (before touching Cloudflare)
- [ ] tunnel hostname `tools.ciceley.cc` → `http://127.0.0.1:32080`
- [ ] proxied CNAME `tools` → `<tunnel>.cfargotunnel.com`
- [ ] `curl -sSI https://tools.ciceley.cc/` → 200, `cf-ray` present, HSTS at the edge
- [ ] `bakuran-gitops`: host added to `ingress-host-check`'s public list, CronJob ran clean
- [ ] `ciceleywebsite`: menu entry PR (optional, last)

## 6. M5 — WebMCP

- [ ] `curl -sS https://tools.ciceley.cc/api/tools | jq '.tools[].name'`
- [ ] `Cache-Control: no-store` on the manifest
- [ ] WebMCP-capable browser: `navigator.modelContext.getTools()` lists the public tools
- [ ] `execute` output == HTML form output for the same input
- [ ] browser without the API: no console error, page fully usable
- [ ] logged-in `/lab` page exposes gated tools; anonymous page does not

## Always

- [ ] no secrets in `git diff` (`git grep -nE 'ghp_|github_pat_|NOTION_API_KEY='` is empty)
- [ ] image tag in `deploy/k8s/deployment.yaml` was not hand-edited
- [ ] `theme/hugo-theme-stack` unchanged (`git -C theme/hugo-theme-stack status --porcelain` empty)
- [ ] PR body names the milestone and pastes the "done when" command output
