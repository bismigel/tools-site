# ADR-0009 — exchange rates are fetched at build time, not at runtime

Date: 2026-11-14 · Status: accepted · Blocks: M6 (`rate_converter`)

## Context

`rate_converter` needs current exchange rates. The obvious implementation is a runtime HTTP call to
a rates API from the tool handler. That would drag in: an outbound destination for a *public* tool,
the SSRF guard, an egress rule in the NetworkPolicy, a provider API key or a rate-limited free tier,
a failure mode where the provider is down and the tool is broken, and non-deterministic tests.

The requirement is not a trading feed. It is "normalise a contractor rate across currencies, and be
right to within a day".

## Decision

- Source: **European Central Bank daily reference rates** (free, no key, EUR-based, published on
  working days). Cross-rates computed.
- Fetched by a **scheduled CI job** (GitHub Actions), which commits `internal/fx/rates.json` with the
  publication date and the source URL.
- The JSON is **embedded in the binary** (`//go:embed`) and shipped with the image.
- The tool makes **no network call at runtime**. It displays the rate date, always.
- A failed refresh is not an outage: the previous dated file stays, and the page says how old it is.
  A refresh older than 7 days raises a monitoring alert, not a page error.

## Reasons

1. **It removes a whole security surface from a public tool.** No runtime egress means no SSRF guard
   to get wrong, no allowlisted destination, no proxy behaviour, no DNS rebinding question.
2. **A public tool can stay public.** FR-1.9 would otherwise force `rate_converter` into the gated
   zone, which is wrong for its purpose (convenience, no login).
3. **Determinism.** Tests assert exact numbers against the pinned file. A tool whose output depends
   on what a third party served at 03:00 cannot be tested honestly.
4. **Failure is graceful.** Stale rates with a visible date are better than a 502, and much better
   than a silent fallback to a hardcoded rate.
5. **The dependency is reviewable.** The rate file is a small dated diff, visible in the history —
   unlike a runtime dependency that can change behaviour without a commit.

## Costs (accepted)

- Rates are **daily reference rates**, not live mid-market. Stated on the page. Fine for rate
  normalisation; not fine for invoicing, and the tool says so by omission of any such claim.
- **Weekend and holiday staleness**: Friday's rate applies until Monday's publication. The date is
  always shown.
- **Coverage is ECB's currency set** (~31). Currencies outside it (NGN, PKR, …) are reported as
  unsupported rather than guessed (spec §7.2 Q15).
- **One exception, and it is labelled:** AED is not an ECB quote, so it is derived at build time from
  its fixed peg to the USD (3.6725 since 1997) and every result that uses it carries the line
  "AED: derived from its fixed peg to the USD at 3.6725 (not an ECB quote)". Deriving a hard peg is
  arithmetic; deriving a floating currency would be invention.
- A CI job can fail silently; mitigated by an alert on file age and by the date being user-visible.

## Rejected

- **Runtime fetch with the guarded client**: correct, but it makes a convenience tool private and
  adds a network failure mode to a page that should always work.
- **Hardcoded rates in the source**: goes stale invisibly and needs a code review to update.
- **A provider API key (e.g. a commercial FX API)**: a secret for a public tool, for a requirement
  that daily reference rates already meet.
- **Fetching in the ingress/edge (Cloudflare Worker)**: a second runtime, a second place to debug,
  and the rate would still need to reach the Go tool.
