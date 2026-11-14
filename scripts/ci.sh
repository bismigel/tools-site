#!/usr/bin/env bash
# Local mirror of .github/workflows/check.yml — "shift left": CI runs here first.
# One source of truth: the workflow calls this script.
set -euo pipefail
cd "$(dirname "$0")/.."

step() { printf '\n== %s ==\n' "$1"; }

step "css drift"
make --no-print-directory css
out=$(git status --porcelain -- theme/tools.css || true)
if [ -n "$out" ]; then
  echo "theme/tools.css is stale — run make css and commit it"
  exit 1
fi

step "gofmt"
out=$(gofmt -l .)
if [ -n "$out" ]; then echo "$out"; exit 1; fi

step "vet"
go vet ./...

step "test -race"
go test -race ./...

step "webmcp (js)"
if command -v node >/dev/null 2>&1; then
  node scripts/webmcp.test.mjs
else
  echo "node not on PATH — skipping (check.yml adds setup-node at M2)"
fi

step "e2e (playwright)"
if command -v npx >/dev/null 2>&1 && [ -d node_modules ]; then
  npx playwright test
else
  echo "skipped: run 'make e2e-install' (CI installs node + chromium)"
fi

step "build"
CGO_ENABLED=0 go build ./...

step "vuln"
if command -v govulncheck >/dev/null 2>&1; then
  govulncheck ./...
else
  echo "govulncheck not on PATH — CI installs it; skipping locally"
fi

step "result"
echo "all local CI steps passed"
