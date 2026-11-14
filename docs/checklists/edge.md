# Checklist — from "not deployed" to live

Order matters more than speed here. Everything outside this repo touches hostnames shared with other
sites, so do one step at a time and verify before the next. Values (tunnel id, account id, token) live
in `~/projects/homelab-context/CREDENTIALS.md` — read them there, never copy them into a repo, an
issue or a chat log.

## 0. The image must exist *before* ArgoCD is given the Application

The Deployment's `image:` line is a placeholder until the first release. An Application synced before
that is `ErrImagePull`, which reads like an ingress problem and wastes an afternoon. Do this first.

- [ ] push `main` in **this** repo → `check` runs, `release` publishes
      `ghcr.io/bismigel/tools-site:main-<YYYYmmdd-HHMMSS>-<sha7>` and opens the image-bump PR
- [ ] set the **GHCR package visibility to public** (packages created by Actions default to private;
      public means the cluster needs no `ghcr-secret`)
- [ ] if the bump PR says "workflow awaiting approval", approve it (one click): the branch was
      force-pushed by the Actions bot, so the `pull_request` run needs a human. Do **not** automate that
      approval — it is the same gate that stops a compromised token from running workflows unreviewed
- [ ] merge the image-bump PR
- [ ] prove the cluster can pull **the exact digest** before handing ArgoCD the Application:

```bash
ref=$(grep -m1 '^\s*image:' deploy/k8s/deployment.yaml | awk '{print $2}')
repo=${ref%@*}; repo=${repo%%:*}
digest=${ref#*@}
tok=$(curl -fsS "https://ghcr.io/token?scope=repository:${repo#ghcr.io/}:pull" | jq -r .token)
curl -sS -o /dev/null -w '%{http_code}\n' -H "Authorization: Bearer $tok" \
  -H 'Accept: application/vnd.oci.image.index.v1+json, application/vnd.docker.distribution.manifest.v2+json' \
  "https://ghcr.io/v2/${repo#ghcr.io/}/manifests/${digest}"
# 200 = pullable without credentials; 401/403 = private or missing.
```

Ask the registry, not a local docker daemon: `docker manifest inspect` (or the podman shim's) can answer
from cached credentials and report a false negative — it did on 2026-11-14 for a digest the registry
was serving anonymously.

Non-zero means one of **two** things — that digest does not exist, *or* the package is private — and
both make the pod `ErrImagePull`. Treat either as "go back a step". The committed digest is all
zeroes until the first release writes a real one, and that placeholder fails this check on purpose.

## 1. ArgoCD registration — `bakuran-gitops`, one PR

Prepared 2026-11-14, left uncommitted for review:

- [ ] `clusters/prod/apps/workloads/tools-site.yaml` — the Application (repo
      `https://github.com/bismigel/tools-site`, path `deploy/k8s`, namespace `apps`)
- [ ] `clusters/prod/projects/projects.yaml` — add the repo to `sourceRepos`; without it the sync
      fails with `InvalidSpecError: … repo is not permitted`
- [ ] **not in this PR:** the `ingress-host-check` line. It probes through ingress-nginx, and a host
      with no serving backend answers 404 — so adding it now makes the probe red from minute one. It
      is its own change, after the app serves (§6).

Then, from a host that can reach the API server:

- [ ] `argocd app get tools-site` → `Synced` / `Healthy`
- [ ] `kubectl -n apps get pods -l app=tools-site -o wide` → 2/2 Ready, different nodes

**On 2026-11-14 the dev box could not reach the cluster** (`https://192.168.2.82:6443` timed out),
so this section runs on the cluster host, over SSH, or after fixing routing/VPN.

## 2. Pre-flight

- [ ] `kubectl -n apps get ingress tools-site` → has an address
- [ ] `kubectl -n apps port-forward svc/tools-site 8080:80` + `curl -sS localhost:8080/healthz` → `ok`
- [ ] The in-cluster probe path works: from any pod,
      `curl -sS -H 'Host: tools.ciceley.cc' http://ingress-nginx-controller.ingress-nginx.svc.cluster.local/`
      → 200 (not 404; 404 means the Ingress class/host does not match)

If the pre-flight fails, **stop**. Cloudflare will only add a confusing error page on top.

## 3. Tunnel hostname

- [x] remote-managed tunnel config gains `tools.ciceley.cc` → `http://10.43.46.55:80` (done 2026-11-14;
      the tunnel points at ingress Service addresses, not `127.0.0.1:32080`)
- [ ] other hostnames still work (`curl -sSI https://ciceley.cc/`, `https://bismigel.dev/`)
- [ ] `curl -sSI https://tools.ciceley.cc/` → 502 is expected *and correct* until DNS exists; the
      tunnel is not reachable by name yet

## 4. DNS

- [ ] `ciceley.cc` zone: proxied `CNAME tools → <tunnel-id>.cfargotunnel.com`
- [ ] `dig +short tools.ciceley.cc` returns Cloudflare addresses
- [ ] `curl -sSI https://tools.ciceley.cc/` → 200; certificate is Cloudflare's edge certificate

## 5. Access verification (application already created 2026-11-14)

- [x] Access application **"Tools (private)"**: `tools.ciceley.cc/lab` and `/api/lab`, self-hosted,
      24 h session, launcher visible
- [x] policy **Owner**: allow `bismigel@pm.me` and `ciceleyrubio17@gmail.com`, OTP (no IdP pinned)
- [x] team domain `bakuran.cloudflareaccess.com` + AUD recorded in `deploy/k8s/configmap.yaml`
      (neither is a secret; the app verifies the signed JWT, never the email header)
- [ ] `curl -sSI https://tools.ciceley.cc/lab` → 302 to `https://bakuran.cloudflareaccess.com/…`
- [ ] `curl -sSI https://tools.ciceley.cc/` → 200, **not** challenged (the app must not cover `/*`)
- [ ] from **inside** the cluster (bypasses Access), `curl -sS -H 'Host: tools.ciceley.cc'
      http://ingress-nginx-controller.ingress-nginx.svc.cluster.local/lab` → the app's own 401, not a
      redirect — proves the app enforces auth itself rather than trusting the edge

## 6. Monitoring — only after the app serves

Do this **after** the Application is Synced/Healthy and `/healthz` answers from inside the cluster.
Adding the host earlier means a probe against a 404 and a red alert that means nothing.

- [ ] `bakuran-gitops`: add `tools.ciceley.cc` to the public list in
      `manifests/base/monitoring/ingress-host-check.yaml` (separate PR)
- [ ] after the next host-check run (≤5 min): no `IngressHostBroken` alert for the host

## 7. Rollback

- [ ] remove the tunnel hostname **first** (stops public traffic cleanly), then the DNS record
- [ ] only then remove/scale the workload; leaving an Ingress with no tunnel is harmless

## Never

- Never make `tools.ciceley.cc` an Access-gated host overall — the public tools must stay reachable
  by agents and by search engines' fetchers.
- Never add a `*` wildcard path to the Access application "to be safe": it silently turns the public
  zone private, and the failure looks like "WebMCP stopped working".
- Never touch `ciceley.cc` / `www` rules while doing this; different app, same zone.
- Never hand-edit the `image:` tag to something that does not exist yet; that is section 0's job.
