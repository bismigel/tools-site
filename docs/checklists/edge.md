# Checklist — edge changes (Cloudflare), do them in this order

Everything here happens **outside this repo** and touches hostnames shared with other sites. Do one
step at a time and verify before the next. Values (tunnel id, account id, token) live in
`~/projects/homelab-context/CREDENTIALS.md` — read them there, never copy them into a repo, an
issue, or a chat log.

## Pre-flight

- [ ] `kubectl -n apps get ingress tools-site` → has an address
- [ ] `kubectl -n apps port-forward svc/tools-site 8080:80` + `curl -sS localhost:8080/healthz` → `ok`
- [ ] The in-cluster probe path works: from any pod,
      `curl -sS -H 'Host: tools.ciceley.cc' http://ingress-nginx-controller.ingress-nginx.svc.cluster.local/`
      → 200 (not 404; 404 means the Ingress class/host does not match)

If the pre-flight fails, **stop**. Cloudflare will only add a confusing error page on top.

## 1. Tunnel hostname

- [ ] remote-managed tunnel config gains `tools.ciceley.cc` → `http://127.0.0.1:32080`
- [ ] other hostnames still work (`curl -sSI https://ciceley.cc/`, `https://bismigel.dev/`)
- [ ] `curl -sSI https://tools.ciceley.cc/` → 502 is expected *and correct* until DNS exists; the
      tunnel is not reachable by name yet

## 2. DNS

- [ ] `ciceley.cc` zone: proxied `CNAME tools → <tunnel-id>.cfargotunnel.com`
- [ ] `dig +short tools.ciceley.cc` returns Cloudflare addresses
- [ ] `curl -sSI https://tools.ciceley.cc/` → 200; certificate is Cloudflare's wildcard/edge cert

## 3. Access (M3b only)

- [ ] Access application: `tools.ciceley.cc`, paths `/lab*` and `/api/lab*`
- [ ] policy: allow, one-time PIN, owner's email (+ service token for scripts if needed)
- [ ] team domain + AUD tag recorded into the app's ConfigMap (PR in this repo)
- [ ] `curl -sSI https://tools.ciceley.cc/lab` → 302 to `https://<team>.cloudflareaccess.com/…`
- [ ] `curl -sSI https://tools.ciceley.cc/` → 200 (public zone not challenged: the Access app must
      not accidentally cover `/*`)
- [ ] from **inside** the cluster (bypasses Access), `curl -sS -H 'Host: tools.ciceley.cc'
      http://ingress-nginx-controller.ingress-nginx.svc.cluster.local/lab` → the app's own 401, not a
      redirect — proves the app enforces auth itself

## 4. Monitoring

- [ ] `bakuran-gitops`: `tools.ciceley.cc` added to the public host list in
      `manifests/base/monitoring/ingress-host-check.yaml`
- [ ] after the next run (≤5 min): no `IngressHostBroken` alert for the host

## 5. Rollback

- [ ] remove the tunnel hostname **first** (stops public traffic cleanly), then the DNS record
- [ ] only then remove/scale the workload; leaving an Ingress with no tunnel is harmless

## Never

- Never make `tools.ciceley.cc` an Access-gated host overall — the public tools must stay reachable
  by agents and by search engines' fetchers.
- Never add a `*` wildcard path to the Access application "to be safe": it silently turns the public
  zone private, and the failure looks like "WebMCP stopped working".
- Never touch `ciceley.cc` / `www` rules while doing this; different app, same zone.
