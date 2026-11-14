# ADR-0010 — profile enrichment is paste-based; no scraping, no enrichment vendors

Date: 2026-11-14 · Status: accepted · Blocks: M9

## Context

Client request: *"a way to enrich data gathered by pasting LinkedIn profile just so we can get more
accurate data."*

"Pasting a LinkedIn profile" can mean three different products:

**(a) paste the profile *text*** (copy from the page) and have the assistant structure it, merge it
with the CV, and show where the two disagree.

**(b) paste the profile *URL*** and have the server fetch it.

**(c) call a data-enrichment vendor** (People Data Labs, Apollo, Hunter and similar) to attach
employer size, contact details, social accounts.

The stated goal is *accuracy*, not reach. That distinction decides the answer.

## Decision

**Implementation (a).** The assistant accepts LinkedIn profile **text** as a second input alongside
the CV, and produces a field-by-field merge with **provenance**:

- **agreed** — both sources say the same thing;
- **conflict** — both say something, differently (title inflation, date mismatch, employer naming);
- **single source** — only one of the two mentions it;
- **missing** — neither does.

Each merged field keeps the original wording from each source. A discrepancy list is the headline
output, because *that* is the accuracy gain a recruiter can act on: it produces the questions to ask.

**A pasted URL is rejected, not fetched.** `linkedin.com/in/…` in the input returns a short
instruction: copy the profile text (or "Save to PDF") and paste that.

**Not built:** no HTTP fetch of a profile URL, no headless browser, no stored LinkedIn session, no
scraping library, no vendor enrichment API, no browser extension. Egress stays DNS-only for `hire`.

## Reasons

1. **(b) does not work.** LinkedIn serves a login wall to unauthenticated fetches. Making it work
   needs a stored session or a headless browser — a credentialed robot acting as the owner, which is
   a custody problem and a terms-of-service problem, in a repo that would then hold a session token
   for a personal account.
2. **(c) creates a lawful-basis problem the owner would inherit.** Attaching inferred employer size,
   contact details or social accounts to a named candidate means creating personal data from third
   parties, via a new processor, under a contract, for a recruitment decision. That is a
   data-protection question for the owner and their employer — not a feature flag.
3. **(a) delivers the actual goal.** The inaccuracies a recruiter cares about are date gaps, title
   inflation, overlapping roles and employer-name drift — all visible by comparing two texts the
   owner already lawfully holds.
4. **It keeps the threat model unchanged.** No new egress, no new secret, no new SSRF surface, no new
   processor. What changes is one more text input and one more table.
5. **The tool does not resolve conflicts, it surfaces them.** A model that silently picks a winner is
   confidently wrong; a table that says "CV: Engineer (2019–2022) / profile: Senior Engineer
   (2018–2022)" gives the recruiter the exact question to ask.

## Costs (accepted)

- A manual copy step, and the owner must be logged in to LinkedIn to see the profile.
- No automatic refresh: a profile changes, the paste does not. Acceptable — the merge is a
  point-in-time comparison, and it says so.
- The model sees the profile text, so the same personal-data rules as the CV apply (no storage, no
  logging of content, optional redaction).

## What about WebMCP?

WebMCP (`navigator.modelContext`, `docs/context/webmcp.md`) is a **serving** API, not a reading one.
A page offers tools to an agent *inside that document*; the browser brokers the call. It gives the
agent no access to another origin, no access to another tab, no ability to bypass a login wall, and
it does not let us register tools on `linkedin.com`. Put another way: WebMCP is how a site says
"here is what you may do to me", not how an agent says "here is what I will take from you".

So it changes nothing about scraping: the server still never fetches a profile, and there is still no
stored session.

The one legitimate pattern it *does* enable — worth naming because it is the honest answer to "can
the paste be less manual?": if the owner's **browser agent already has the profile** (they are logged
in and looking at it), that agent can call `merge_profile` with the text. That is the manual paste
with the copy step delegated to the user's own tool. Our servers receive text they still never fetch.
Whether an automated read by the user's agent is acceptable is between the owner and LinkedIn's
terms; we are not laundering it, and we do not build, store or ship a session to make it work.

It is also not guaranteed: it depends on the agent having page access and on WebMCP support, which
is experimental. No workflow may depend on it — it is a convenience, exactly like the manual paste.

## Can a URL be fetched instead?

It is buildable — a URL fetch is the same shape as any outbound tool (gated, SSRF-guarded, egress
allowlisted). The real questions are whether it *works* and whether it is *allowed*. Measured on
2026-11-14 against a public profile, anonymously, no cookie:

- `GET https://www.linkedin.com/in/williamhgates` → **`200 OK`**, ~530 kB, but the body is the
  signed-out shell ("Sign in", "Join now"). The only profile data present is the `<title>` /
  `og:title` — name and headline. The structured data (`application/ld+json`) describes the person's
  **articles**, not their employment history. No roles, no dates, no education.
- `https://www.linkedin.com/robots.txt` → for `User-agent: *`: **`Disallow: /`**, with a notice to
  email `whitelist-crawl@linkedin.com` to apply for whitelisting.

So a fetch returns enough to *look* like it worked and not enough to compare a CV against — strictly
worse than a paste for the stated goal, accuracy. And it is crawling, which the site's rules prohibit
without a whitelist.

Therefore the URL is accepted as **citation only**: validated, echoed as the source link in the
result, never fetched. Paste stays the only path that yields the roles and dates the merge exists to
compare, and it yields them with the owner's own eyes on the text.

**Non-LinkedIn URLs** (a personal site, GitHub, a company page) are a different question and are not
rejected. A general "fetch this page and extract" tool is legitimate; when it is built it is
`Gated`, uses the guarded client (NFR-2.4), names its egress destination in the NetworkPolicy,
honours the target's `robots.txt`, and gets its own ADR. It is not a quiet addition to
`merge_profile`.

## Rejected

- **Server-side fetch of the profile URL**: login wall, ToS, credentialed automation.
- **Headless browser with a stored session**: credential custody, breakage on every layout change,
  and it is scraping with extra steps.
- **LinkedIn's official API**: no profile-reading capability for this use case (partner-only, and
  not for a personal tool).
- **Vendor enrichment APIs**: new processor, new contract, personal data created about people who did
  not provide it, and accuracy we cannot verify.
- **A browser extension that reads the page**: a second codebase, a store review, and the same data
  handling questions with none of the auditability.

## Revisit if

LinkedIn offers the owner an official profile API they can legitimately use. Then it is a `Gated`
tool with OAuth, a named destination in `docs/design/security.md`, and its own ADR — not a quiet
addition to this one.
