# DESIGN — `tools.ciceley.cc` + `hire.ciceley.cc`

**Status: v1.1 · 2026-11-14 · owner: @bismigel**

This is the readable design document: what the system does, why it is shaped this way, and what you
will see at each step. It is written so that a non-engineer can follow it and an engineer can build
from it.

**What outranks what:** `docs/adr/` (decisions, with reasons) → `spec.md` (testable requirements) →
this document (narrative) → `docs/design/*` (implementation notes). If they disagree, the higher one
wins, and the lower one is a bug.

---

## 1. What this is

Three layers, one deployment pattern:

| Layer | Address | Who uses it | State |
| --- | --- | --- | --- |
| **Public tools** | `tools.ciceley.cc` | anyone, no account | none |
| **Private tools** | `tools.ciceley.cc/lab` | the owner, via Cloudflare Access | none |
| **HR assistant** | `hire.ciceley.cc` | the owner, via Cloudflare Access | none (the thread lives in the browser) |

The public tools are the front door and the proof: a recruiter comparing a day rate in Berlin
against a salary in London, or hunting for a meeting slot that works in Sydney, should get an answer
in one page, on a phone, without an account, and without JavaScript.

The assistant is the product that sits on top, and it consumes the tools rather than reimplementing
them.

### The one rule everything follows

**A capability is defined once.** From that one definition come: the web page, the plain JSON
endpoint, the WebMCP listing (browser assistants), and the MCP listing (server-side assistants).
Nothing is written twice, so nothing can disagree with itself. This is why adding a tool is one file
plus one registration line, and why "API-first" is an architecture rule rather than a marketing
phrase.

### The three non-negotiables from the client

1. **Currency + rate converter** — one amount, converted and normalised to hourly, weekly, monthly
   and annual, in a chosen currency.
2. **Timezone / meeting planner** — WorldTimeBuddy-style: pick a date and 2–6 cities, see the hours
   side by side and the best overlap window.
3. **The HR assistant** — private, for one technical recruiter, with a chat window last.

Everything else in this document exists to serve those three.

---

## 2. The workflow it supports

A technical recruiter's day, and where the system helps:

| Moment | The question | The tool |
| --- | --- | --- |
| Comparing roles | "£550/day in London — what is that annually, and in euros?" | `rate_converter` |
| Scheduling | "When can I talk to the Sydney candidate and still keep my morning?" | `meeting_planner` |
| Before the first read | "The CV and the LinkedIn profile disagree on dates and titles — what is actually true?" | `merge_profile` |
| First read | "Is this CV worth a call? What does the CV actually evidence?" | `screen_candidate` |
| Before the call | "What should I ask to close the gaps?" | `interview_kit` |
| After the call | "Write the next-step or the no-thank-you email." | `draft_email` |
| Writing the ad | "Is this job description too long?" | `reading_time` (later) |

The first two are public and need no login. The last four are the assistant, and they are private.

---

## 3. The tools

### 3.1 `slugify` — M1, the plumbing proof

Text in, URL slug out. It exists to prove the pipes with something that cannot fail, and it is
specified with test vectors in `spec.md` §5 (including what it deliberately does *not* do).

### 3.2 `rate_converter` — non-negotiable

**The problem it solves:** a rate is useless without its basis. "€85/hour", "£550/day" and
"$150,000/year" are three ways of stating one number, and recruiters compare them all day across
currencies.

**Inputs**

| Input | Kind | Required | Rules | Default |
| --- | --- | --- | --- | --- |
| `amount` | Number | yes | 0.01 – 10,000,000 | — |
| `currency` | Select | yes | supported currency list | GBP |
| `period` | Select | yes | `hour`, `day`, `week`, `month`, `year` | `day` |
| `target` | Select | yes | supported currency list | GBP |
| `basis` | Select | no | `contractor` = 8 h × 5 d × 46 wk (1,840 h/yr); `employee` = 7.5 h × 5 d × 47 wk (1,762.5 h/yr — 52 weeks less 25 days leave); `custom` | `contractor` |
| `hours_per_day` | Number | no | 1–24 | 8 |
| `days_per_week` | Number | no | 1–7 | 5 |

**Output** — a table, not a paragraph. Rows are the same money expressed five ways; columns are the
source and target currency.

```
£550.00 per day                          assumptions: 8h/day · 5d/week · 46wk/year = 1,840h/year
┌──────────┬──────────────┬──────────────┐
│ Period   │ GBP          │ EUR          │
├──────────┼──────────────┼──────────────┤
│ Hourly   │ £68.75       │ €80.60       │
│ Daily    │ £550.00      │ €644.77      │
│ Weekly   │ £2,750.00    │ €3,223.83    │
│ Monthly  │ £10,541.67   │ €12,358.00   │
│ Annual   │ £126,500.00  │ €148,295.95  │
└──────────┴──────────────┴──────────────┘
FX: 1 GBP = 1.1723 EUR · ECB reference rate, 2026-11-13 · rates are daily, not live
```

**The assumptions are part of the answer.** Annualising is where the dishonesty creeps in, so the
three numbers are visible, editable, and echoed back above the table. A `contractor` basis (46 weeks)
versus `employee` (52 weeks) changes an annual figure by ~13 %, which is more than most FX moves.

**Where the exchange rates come from** — and this is a real design decision:

- Rates are the **European Central Bank daily reference rates**: free, no API key, published on
  working days, EUR-based, cross-rates computed.
- They are fetched **once a day by the build pipeline**, checked in as a dated file, and **embedded
  in the binary**. The tool itself makes no network call at runtime.
- Consequences, all good: the tool stays **public** (it needs no gating), the cluster needs **no
  outbound access**, there is no API key to leak, there is no server-side request forgery surface,
  and the tests are deterministic.
- The honest limitation is printed on the page: these are daily reference rates, not a trading feed.
  Weekends and holidays show the last working day's rate, with the date attached.

**Deliberate non-features:** no live rates, no crypto, no historical charts, no tax or IR35
deductions, no "take-home pay". This converts a rate; it does not advise on a contract.

**Convenience details that matter**

- Every input is in the URL, so a rate comparison can be bookmarked, shared in Slack, or opened on a
  phone. (`?amount=550&currency=GBP&period=day&target=EUR`)
- A **copy as text** action and a **CSV download** — the two things that actually get pasted into a
  spreadsheet.
- The last-used inputs are remembered in the browser (not on the server).

### 3.3 `meeting_planner` — non-negotiable

**The problem it solves:** finding an hour that works for London, New York and Sydney, without
mentally adding offsets and getting the day wrong.

**Inputs**

| Input | Kind | Required | Rules | Default |
| --- | --- | --- | --- | --- |
| `cities` | Text | yes | 2–6 names, comma-separated, matched against a known city list with aliases ("New York" → `America/New_York`) | `London` |
| `date` | Text (ISO date) | yes | today ± 366 days | today |
| `work_start` | Text (time) | no | `00:00`–`23:59`, interpreted in **each city's** local time | `09:00` |
| `work_end` | Text (time) | no | after `work_start`, same interpretation | `17:00` |
| `length` | Select | no | 30, 45, 60 minutes | 60 |

**Output**, in this order, because the answer comes before the detail:

0. **Where you are now** — each city's current time, and a `now` marker on the matching hour row when
   the date is today. Without it the grid is a beautiful abstraction of a day that is already happening.
1. **The verdict**, one line: *"Best overlap: 3 hours — 14:00–17:00 London = 09:00–12:00 New York."*
   If there is no overlap, say so plainly and name the closest near-miss.
2. **The grid** — 24 rows, one column per city, each cell showing the local time with a `+1` / `−1`
   day badge where the date differs. Cells inside the working window are marked. This is the
   WorldTimeBuddy view, server-rendered, so it works without JavaScript and prints correctly.
3. **Caveats** — any city with a daylight-saving change on or near that date, and any city whose
   working window falls entirely outside the anchor day.

**Rules**

- The **first city is the anchor**: it defines the rows and which calendar day is shown. The working
  window is **not** anchored: 09:00–17:00 means 9-to-5 *wherever you are*, so the verdict is the
  intersection of every city's own window, converted back to each city's local time.
- Time zone data is **embedded in the binary** (`time/tzdata`), so the answer cannot depend on the
  host's installed time zone files and daylight saving is handled by the database, not by us.
- Maximum 6 cities. Beyond that the answer stops being readable on a phone, and the page says why.
- On a narrow screen the verdict stays at the top and the grid scrolls **inside its own card** — the
  page itself never scrolls sideways.

**Deliberate non-features:** no drag-and-drop editing, no calendar integration, no attendee
availability, no video links. It finds a time; you send the invitation.

**Convenience details:** shareable URL containing every input; CSV of the grid; a "copy the good
slots" action.

### 3.4 The assistant's five tools (private)

| Tool | Input | Output | Notes |
| --- | --- | --- | --- |
| `merge_profile` | CV text + LinkedIn profile **text** (+ optional profile URL, **cited, never fetched**) | field-by-field merge with **provenance** (agreed / conflict / single source / missing) and a discrepancy list | a URL pasted *as the profile* is rejected with copy instructions; no fetching, no scraping (ADR-0010) |
| `screen_candidate` | job description, CV text | one-line verdict + rubric with **quoted evidence**, gaps, and an explicit "not stated" bucket | no bare score; every claim traceable to a quote |
| `interview_kit` | the screening result, seniority | 5–10 questions, at least one per gap, each tagged with what it is testing | derived from gaps, not generic lists |
| `draft_email` | decision, reasons, tone | a subject and body, ready to edit | never invents facts; never auto-sends |
| `redact_cv` | CV text | the same CV with names, emails, phones, links and addresses removed | **optional** (client decision), offered at the top of the screening form |

**Why five and not six:** extraction and scoring are one job from the recruiter's point of view, and
splitting them means paying to re-read the same CV twice. `merge_profile` is separate because it runs
**before** the read, on two inputs, and its output is the thing the screening step consumes. Why not
one tool: a single "do everything" call cannot be improved in pieces or judged on its own.

**The accuracy claim, precisely:** the merge does not decide who is right. It shows both versions
side by side and lists the conflicts, so the recruiter asks the question instead of trusting a
silently chosen answer. "Enrichment" here means *more structured and cross-checked*, not *fetched
from elsewhere* — see ADR-0010 for why the other meaning is rejected. WebMCP does not change this:
it is a serving API, so the most it can do is let *your* browser agent hand us the text instead of
you pasting it. The server's behaviour is identical either way.

### 3.5 One addition to the result type: a table

The grid and the rate table are genuinely two-dimensional. Today a tool result can return text rows,
files and JSON — all of which render badly as a 24×7 grid or a 5×3 money table. So `Result` gains a
**generic** table: columns, rows, and a small tone marker per cell (normal / emphasised / muted) plus
an optional note. The template renders it; no tool owns markup.

This is a generic primitive, not a per-tool template — the rule that a tool never ships HTML is
intact (`spec.md` FR-1.7).

---

## 4. Prompts and the fixed test set (assistant)

- Prompts are **versioned text files in the repo**, one per capability. A prompt change is a code
  change: reviewed, tested, revertible.
- The test set is fixed: **3 job descriptions × 5 CVs**, all invented or the owner's own, with
  properties the output must satisfy:
  - every claim about the CV quotes it;
  - every gap in the job description is named;
  - no invented employer, title, date or qualification;
  - "not stated" is used when the CV is silent, not replaced with a guess;
  - the vetting line never says more than the rubric supports.
- The model is a **configuration value** (base URL, model, key), and the base URL points at the
  **in-cluster Hermes gateway** (`hermes.duwende.svc:8642`), authenticated with `hermes-api-key`.
  Swapping provider is a Hermes config change, not this app's: the provider is a commodity, the
  prompts and the test set are the asset. As a bonus, `hire` needs no internet egress.

---

## 5. Personal data, plainly

- **Nothing is stored.** No database, no server-side conversation history, no uploaded files. The
  browser holds the thread; the server sees one turn at a time and forgets it.
- **Redaction is optional** (client decision). It is offered, not enforced, and it is built and
  tested. The screening form warns when it detects an email address, phone number or link in the
  pasted text, and suggests `redact_cv`.
- **What redaction cannot do:** it cannot reliably detect a *name*. That is stated in the UI rather
  than pretended otherwise.
- **The residual responsibility is the owner's.** If a real candidate's CV is pasted, that is a
  transfer of personal data to the model provider under that provider's terms — Hermes is a hop, not
  a boundary: it forwards the text to the same provider. The system makes the
  safe path easy and visible; it does not make the decision, and it does not log the content (only
  sizes, timings and token counts).
- **A LinkedIn profile is personal data too**, and it is only ever **pasted as text**. The system does
  not fetch a profile URL, does not scrape, does not keep a LinkedIn session, and does not call a
  data-enrichment vendor (ADR-0010). A pasted URL is rejected with instructions rather than quietly
  fetched.
- **A human decides.** There is no automated rejection, no bulk screening, no unattended run. The
  assistant drafts; the recruiter sends.

---

## 6. The chat window (last on purpose)

- **Two panes:** the job description pinned on the left; the current candidate and the conversation
  on the right. Switching candidates keeps the job description. This is the shape of the actual job;
  a generic chat box would make the recruiter re-paste the ad every time.
- **The thread lives in the browser.** A reload with the tab open may survive; a closed tab does not.
  That is the price of storing nothing, and it is the right price for candidate data.
- **Copy / download** on every result, because the next stop is an email, a document or an ATS.
- **The fallback is a wizard.** If the chat turns out to be the unreliable part, the same tools
  rendered as a three-step form are the honest alternative. Because the tools came first, that is a
  UI change, not a rebuild.
- **Cost is visible**: a per-turn token count and a running monthly total, so the spend guard is
  never a surprise.

---

## 7. How it is built and deployed

Two repos, one pattern (the pattern is `homelab-app-template`'s, with its two known flaws fixed):

| | `tools-site` | `hire` |
| --- | --- | --- |
| Repo visibility | public (nothing secret) | **private** (prompts + product spec) |
| Runtime | one Go binary, templates and assets embedded | one Go binary, templates embedded |
| Database | none | none |
| Secrets | none (FX data is fetched in CI) | model API key via ExternalSecret |
| Egress | **deny** — DNS only | deny + `duwende:8642` (Hermes), no internet |
| Replicas | 2, PDB `minAvailable: 1` | same |
| Ingress | `tools.ciceley.cc` | `hire.ciceley.cc` |
| Identity | Access on `/lab*` only | Access on the whole host |
| Deployer | ArgoCD only, image tag owned by CI | same |

**Both** are non-root, read-only root filesystem, all capabilities dropped, no service-account token,
`RuntimeDefault` seccomp, requests equal to limits for memory, scratch space only in `/tmp`.

**The data that needs care:**

- **FX rates** live in the repo as a dated file, refreshed by a scheduled CI job that commits
  `fx-YYYY-MM-DD.json` when the ECB publishes. A failed refresh is a stale rate shown with its date,
  not an outage. Nothing in the cluster fetches it.
- **Time zone data** is compiled into the binary; no host dependency, no tzdata package to patch.

**Monitoring** is not an afterthought: `/metrics` from the first deploy (traffic, errors, latency per
tool), and the assistant adds token counts, per-day spend and a spend alert. Health is two endpoints
with different jobs: liveness that touches nothing, readiness that confirms the app can serve.

---

## 8. Cost and limits

| Limit | Value | Why |
| --- | --- | --- |
| Request body | 64 KiB | a CV or job ad is text; anything larger is an attack or a mistake |
| Tool runtime | 5 s (10 s outbound) | a stuck tool must not hold a request open |
| Concurrency per tool | 4 | one tool cannot starve the process |
| Assistant messages | 100/day, £25/month | a paid API with no lid is a wallet with no lid |
| Assistant alert | 80 % of monthly budget | you hear about it before the stop |
| Public rate limit | ingress already does 40 r/s | a second global limiter would just double the noise |

Realistic assistant spend at a recruiter's usage is a few pounds a month; the caps exist for the bad
afternoon, not the average one.

---

## 9. What this deliberately does not do

- No database, no queue, no background job, no stored CVs or conversations.
- No accounts of our own: identity is Cloudflare Access or nobody.
- No candidate-facing chat, ever, in this design.
- No automated rejection, bulk screening or unattended runs.
- No JavaScript requirement: every page and every tool works without it.
- No third-party fonts, analytics, CDNs or tracking.
- No per-tool templates, CSS or JavaScript — one definition, four renderings.
- No public URL fetcher; any future fetching tool is private and guarded.
- **No scraping or data enrichment vendors**: no LinkedIn fetch, no headless browser, no stored
  session, no People-Data-Labs-style lookups. Profiles are pasted as text (ADR-0010).
- No mobile app, no browser extension, no Slack bot.

---

## 10. Build order, and what you will see

| Step | Work | You will see |
| --- | --- | --- |
| **M1** | Skeleton, theme, registry, `slugify`, JSON endpoints, probes | A themed page on your laptop with one working tool and a dark-mode toggle |
| **M5** | WebMCP bridge | A browser assistant can list and call `slugify` — your debugging window |
| **M2** | Container, CI | Every change tested automatically; a ready-to-run image per push |
| **M3** | Kubernetes manifests, ArgoCD, metrics | Two copies running on the cluster, restarting without downtime |
| **M4** | Tunnel, DNS, host check | `https://tools.ciceley.cc` live over Cloudflare |
| **M6** | **`rate_converter` + `meeting_planner`** | The two non-negotiable tools, live, from a phone |
| **M3b** | Access + JWT verification | `/lab` exists and refuses anyone without a login — only when there is something private to put there |
| **M7** | MCP endpoint | A server-side assistant can list and call the same tools |
| **M8** | Full monitoring | Dashboards, log search, spend alerts |
| **M9** | Assistant, API only | Behind login: screening, interview kit, email draft, optional redaction |
| **M10** | Chat UI | The two-pane window; the wizard remains the fallback |

Two ordering notes worth understanding:

- **Deployment comes before the interesting tools.** Not because deployment is more fun, but because
  adding a tool to a live site is the design's central claim ("one file, one line"). Proving that
  claim on a live system is worth more than adding two tools to a laptop.
- **`/lab` is built only when something needs it.** The private zone is not mounted at all until the
  Access configuration exists (`spec.md` FR-4.3), so there is no window in which it is accidentally
  public. That is why M3b sits late: with no private tool yet, there is nothing to protect.

---

## Appendix — decisions taken in the 2026-11-14 planning session

| Question | Decision |
| --- | --- |
| Tool breakdown | four workflow-shaped tools, not five document-shaped ones (ADR-0008) |
| Screening output | rubric with quoted evidence, gaps, "not stated"; no bare score |
| Whose CVs | defaults to test documents; real CVs permitted at the owner's risk, warned and not logged |
| Provider | `deepseek-chat`, configurable, key in Bitwarden; swap is one line (Q10 still to confirm the name) |
| Redaction | **optional**, offered and tested, never silently skipped without a warning |
| Assistant → tools-site | public tools only in v0; no service token needed until a private tool is genuinely required |
| First public tools | `rate_converter`, `meeting_planner` (client non-negotiables) |
| Chat window | two panes, last milestone; wizard is the documented fallback |
| Profile enrichment | paste-the-text merge with provenance; **no scraping, no URL fetch, no vendor API** (ADR-0010) |
| Quality bar | fixed 3×5 test set with properties, run on every prompt change |
| Private-tools address | one host, `/lab` — no second hostname |
