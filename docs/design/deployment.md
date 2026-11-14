# Deployment

`git push main` → GitHub Actions → GHCR → bump PR → ArgoCD → tunnel → Cloudflare.

## Image

`tools/Dockerfile` at repo root, multi-stage, context = repo root (so it can see `theme/`):

```dockerfile
# syntax=docker/dockerfile:1
FROM golang:1.27-alpine@sha256:<pin> AS build
WORKDIR /src
ENV CGO_ENABLED=0 GOFLAGS=-mod=readonly GOTOOLCHAIN=auto
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -trimpath -ldflags="-s -w" -o /out/toolsd ./cmd/toolsd

FROM gcr.io/distroless/static-debian12:nonroot@sha256:<pin>
COPY --from=build /out/toolsd /toolsd
USER 65532:65532
EXPOSE 8080
ENTRYPOINT ["/toolsd", "-addr=:8080"]
```

- No shell in the runtime image → **no `HEALTHCHECK` CMD**; Kubernetes probes do that job.
- No writable paths are needed; if something wants scratch space it uses `/tmp` (`emptyDir`).
- `EXPOSE 8080`; container port, service port, and ingress all agree (see manifests).

## Manifests (`deploy/k8s/`)

`kustomization.yaml` → deployment, service, ingress, networkpolicy, pdb, configmap. **No
`namespace.yaml`** — `apps` is owned centrally (`docs/context/homelab.md`).

### Deployment (hardened defaults, template-derived)

```yaml
spec:
  replicas: 2
  strategy: { rollingUpdate: { maxSurge: 0, maxUnavailable: 1 } }
  template:
    spec:
      automountServiceAccountToken: false
      securityContext:
        runAsNonRoot: true
        runAsUser: 65532
        runAsGroup: 65532
        fsGroup: 65532
        seccompProfile: { type: RuntimeDefault }
      topologySpreadConstraints:
        - maxSkew: 1
          topologyKey: kubernetes.io/hostname
          whenUnsatisfiable: ScheduleAnyway
          labelSelector: { matchLabels: { app: tools-site } }
      containers:
        - name: tools-site
          image: ghcr.io/bismigel/tools-site:main-…   # CI owns this line
          ports: [{ containerPort: 8080 }]
          readinessProbe: { httpGet: { path: /readyz, port: 8080 }, initialDelaySeconds: 3, periodSeconds: 5 }
          livenessProbe:  { httpGet: { path: /healthz, port: 8080 }, initialDelaySeconds: 10, periodSeconds: 10 }
          securityContext:
            allowPrivilegeEscalation: false
            readOnlyRootFilesystem: true
            capabilities: { drop: ["ALL"] }
          resources:
            requests: { cpu: 25m, memory: 32Mi }
            limits:   { cpu: 200m, memory: 96Mi }
          envFrom: [{ configMapRef: { name: tools-site } }]
          volumeMounts: [{ name: tmp, mountPath: /tmp }]
      volumes: [{ name: tmp, emptyDir: { sizeLimit: 16Mi } }]
```

`maxUnavailable: 1` + `maxSurge: 0` + PDB `minAvailable: 1` keeps a rolling update from taking both
replicas down with only two nodes.

### NetworkPolicy — with egress deny (the template does not do this)

```yaml
spec:
  podSelector: { matchLabels: { app: tools-site } }
  policyTypes: [Ingress, Egress]
  ingress:
    - from:
        - namespaceSelector: { matchLabels: { kubernetes.io/metadata.name: ingress-nginx } }
  egress:
    - to:                                  # DNS only, until a tool needs more
        - namespaceSelector: { matchLabels: { kubernetes.io/metadata.name: kube-system } }
      ports: [{ protocol: UDP, port: 53 }, { protocol: TCP, port: 53 }]
```

Add `monitoring` on `:8080` for `/metrics` at M8. Add specific destinations (with ports) for an
outbound tool at M6 — never a bare `to: []`.

### Ingress

```yaml
spec:
  ingressClassName: nginx
  rules:
    - host: tools.ciceley.cc
      http:
        paths: [{ path: /, pathType: Prefix, backend: { service: { name: tools-site, port: { number: 80 } } } }]
```

No `tls:` block. TLS ends at Cloudflare, same as `ciceley.cc`.

## ArgoCD registration (two edits in `bakuran-gitops`, one file here)

1. `clusters/prod/apps/workloads/tools-site.yaml` — the Application (YAML in
   `docs/context/homelab.md`).
2. `clusters/prod/projects/projects.yaml` — add `https://github.com/bismigel/tools-site` under
   `sourceRepos`. Skipping this yields `InvalidSpecError: … repo is not permitted`.
3. Private repo? Add an ArgoCD repository credential (or make the repo public; nothing here is
   secret).

`syncOptions: [CreateNamespace=true]` is harmless but does not create anything — `apps` exists.

## Edge (Cloudflare) — in this order, once M3 is green

1. **Ingress live** (`kubectl get ingress -n apps tools-site` has an address; a port-forward returns
   200).
2. **Tunnel hostname**: add `tools.ciceley.cc` → `http://127.0.0.1:32080` to the remote-managed
   tunnel config. Procedure/IDs: `~/projects/homelab-context/CREDENTIALS.md` (do not copy values
   into this repo).
3. **DNS**: proxied `CNAME tools` → `<tunnel-id>.cfargotunnel.com` in zone `ciceley.cc`.
4. **Access app** (M3b): application on `tools.ciceley.cc` with paths `/lab*` and `/api/lab*`,
   policy = one-time PIN to the owner's email; record team domain + AUD tag into the ConfigMap.
5. **Host check**: add `tools.ciceley.cc` to the `for h in …` list in
   `bakuran-gitops/manifests/base/monitoring/ingress-host-check.yaml`.

Verify each step with `curl -sSI https://tools.ciceley.cc/{,/lab,/api/tools}` and expect
`200 / 302 (Access) / 200`.

## Rollback

| Situation | Action |
| --- | --- |
| Bad image, app crashing | `argocd app set tools-site -p …` no: instead revert the bump PR in this repo (the tag is a commit) — ArgoCD syncs back on the next poll |
| Bad manifest | revert the commit in `deploy/k8s/`, or `kubectl -n apps rollout undo deploy/tools-site` for an immediate stop-gap (self-heal will re-reconcile; say so in the incident note) |
| Broken edge change | remove the tunnel hostname first, then the DNS record — that takes the site offline cleanly instead of serving 502s |
| Site must go away entirely | delete the Application (prune removes the namespace-scoped objects), then the tunnel row, then DNS |

The blog is not in this path at any point: different repo, different Deployment, different Ingress.

## What "healthy" looks like

- `kubectl -n apps get deploy,pods -l app=tools-site` → 2/2 Ready, pods on different nodes.
- `/healthz` 200 from in-cluster; `/readyz` 200 within ~2 s of process start.
- `IngressHostBroken` never fires for this host.
- After a node drain: one pod keeps serving (PDB), the other reschedules.
