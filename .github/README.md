# A shop window, not a CI system

Since 2026-11-16 the source of truth is **Forgejo** (`git.bismigel.dev`); GitHub receives every push through a push
mirror. CI lives in `.forgejo/workflows/`.

**What is not ported, and why.** The GitHub workflow ran `make ci` (Go build, Node/npm build, dart-sass, govulncheck, a
Playwright E2E suite) and `make image`. The in-cluster runner has none of those toolchains and no browser. What runs here
is the manifest render. The rest is a workstation job until the toolchain question is answered - see ticket 14.7 in the
`homelab-context` plan store. A workflow that looks green while testing almost nothing would be worse than this note.
