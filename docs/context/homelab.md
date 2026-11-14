# The estate this deploys into

Facts gathered from `~/projects/homelab-context`, `~/projects/bakuran-gitops` and
`~/projects/homelab-app-template` on 2026-11-12. **No secret values are reproduced here** — they
live in `~/projects/homelab-context/CREDENTIALS.md` and Bitwarden. Cite that file instead of
copying from it.

## Cluster

- k3s, three nodes: `192.168.2.80` (.80), `.81`, `.82`; the cluster is addressed as `sitaw`.
- ArgoCD is the only deployer. Nothing is applied with `kubectl apply` except the app-of-apps root
  and the two AppProjects.
- Workloads for this app run in namespace **`apps`**, which is owned centrally — **do not create a
  `Namespace` object in this repo**. (History: `ciceleywebsite` had a commit removing exactly that.)

## ArgoCD registration (three files, two repos)

1. **This repo**: `deploy/k8s/` (kustomize).
2. **`bakuran-gitops`**: `clusters/prod/apps/workloads/tools-site.yaml`:

```yaml
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: tools-site
  namespace: argocd
  annotations:
    argocd.argoproj.io/sync-wave: "1"
  finalizers:
    - resources-finalizer.argocd.argoproj.io
spec:
  project: prod
  source:
    repoURL: https://github.com/Ciceley/tools-site
    targetRevision: main
    path: deploy/k8s
  destination:
    server: https://kubernetes.default.svc
    namespace: apps
  syncPolicy:
    automated:
      prune: true
      selfHeal: true
    syncOptions:
      - CreateNamespace=true
```

3. **`bakuran-gitops`**: add the repo to the `prod` AppProject's `sourceRepos` in
   `clusters/prod/projects/projects.yaml`. **Without this the Application fails with
   `InvalidSpecError`** — the file's own comments record that exact failure for `vaultwarden`.

Repo access: a **public** repo needs nothing. A **private** repo needs an ArgoCD repository
credential/deploy key, which is a manual step.

## Images

- `ghcr.io/<owner>/<repo>`, tag `main-<YYYYmmdd-HHMMSS>-<sha7>`, plus `:latest`.
- Public package → `imagePullSecrets` is ignored, nothing to do. Private → the shared `ghcr-secret`
  is created centrally in `bakuran-gitops` from Bitwarden via ESO. Never commit a pull token.

## Cloudflare edge

- One tunnel, **remote-managed** (its ingress list is edited in the dashboard or via the API —
  procedures and IDs are in `homelab-context/CREDENTIALS.md`).
- Every public hostname is a **proxied CNAME** to the tunnel; the tunnel routes to
  `http://127.0.0.1:32080`, which is ingress-nginx's NodePort. NodePorts are loopback-only, so the
  tunnel is the only path in.
- **TLS terminates at Cloudflare.** No `tls:` block, no cert-manager object — matching `ciceley.cc`.
- Order matters: **Ingress first, then tunnel hostname, then DNS.** Reversed, you get a Cloudflare
  error page that reads like a broken application.
- Access (~Zero Trust) is applied per hostname *and can include paths*: `tools.ciceley.cc/lab*` is a
  valid Access application. That is how per-tool gating stays at two static rules
  (`docs/adr/0003-per-tool-visibility.md`).

## ingress-nginx

- Public controller, class `nginx`. Rate limiting already exists **at the edge of the app**: per
  `host:clientIP` `limit_req` 40 r/s with burst 200, returning 429. So do not build a global
  request-rate limiter in Go; app-level limits are for *expensive* tools only.
- `ingress-nginx-admin` is the Access-fronted controller; this app does **not** use it (that would
  gate the whole host, and the host is mostly public).

## Observability and housekeeping

- Prometheus/Grafana/Alertmanager run in `monitoring`; `Alertmanager` is reachable at
  `alertmanager.monitoring.svc:9093` from inside the cluster.
- `manifests/base/monitoring/ingress-host-check.yaml` is a CronJob that probes a **hardcoded** list
  of public hostnames against the ingress Services (bypassing Cloudflare, because a check from
  outside sees only the Access redirect). Add `tools.ciceley.cc` to the `for h in …` list in the
  same PR that adds the Ingress. Otherwise a 404 from a missing Ingress is invisible for hours —
  that exact failure happened on 2026-10-28.
- A ValidatingAdmissionPolicy blocks deletion of PVC/PV/Namespace unless annotated
  `bakuran.dev/allow-delete: "true"`. Not normally relevant (this app is stateless) but it explains a
  deletion that "should work" and does not.

## Template conventions worth keeping

From `homelab-app-template` (the origin of this repo's wiring):

- container: `runAsNonRoot`, `seccompProfile: RuntimeDefault`,
  `allowPrivilegeEscalation: false`, `readOnlyRootFilesystem: true`, `drop: [ALL]`,
  `automountServiceAccountToken: false`, requests/limits, readiness + liveness probes.
- `deploy/k8s/{deployment,service,ingress,networkpolicy,kustomization}.yaml`.
- CI: `check.yml` (PR: build image, render manifests) and `release.yml` (main: build, push, open a
  bump PR because `main` is expected to require PRs).
- `scripts/init.sh <app> <domain> <port> [namespace] [owner]` substitutes `APPNAME`, `APP_DOMAIN`,
  `APP_PORT`, `APPNAMESPACE`, `OWNER` across the repo. **Run it once, in a fresh copy, and read the
  diff** — it rewrites every matching file.

### Two things the template gets wrong for us

1. Its NetworkPolicy declares `policyTypes: [Ingress]` only, so **egress is unrestricted**. If a
   tool needs no network, the policy should say so (add `Egress` + DNS, see
   `docs/design/deployment.md`).
2. Its default `Dockerfile` is nginx serving static files. This repo needs a Go build stage.

## Sibling repos (local checkouts)

| Path | What it is |
| --- | --- |
| `~/projects/bakuran-gitops` | ArgoCD app-of-apps, AppProjects, manifests, `manifests/base/monitoring` |
| `~/projects/homelab-context` | Runbook + facts + `CREDENTIALS.md` (secrets, do not copy) |
| `~/projects/homelab-app-template` | the template this repo starts from |
| `~/projects/ciceleywebsite` | the Hugo blog; source of the theme + palette |
