# ADR-0008 — the HR assistant is a separate app

Date: 2026-11-14 · Status: accepted · Blocks: M9, M10

## Context

The client's first product is not a utility: it is a **technical-recruiter HR assistant** — a chat
interface aimed at the owner, who does technical recruiting. It reads a job description and a CV and
returns fit, gaps, interview questions and a draft reply. Later it should be able to call `tools-site`
tools over MCP.

The existing plan defines a tool as a pure-ish function with declared inputs, no database, no
background job, no persistent state. A chat assistant violates all of that: it has a model provider,
a per-message cost, a conversation, and — via CVs — **other people's personal data**.

## Decision

1. **Separate repo, separate app, `hire.ciceley.cc`.** It reuses the same deployment pattern as
   everything else in the estate: ArgoCD only, image from GHCR, tunnel in, two replicas, PDB,
   non-root, read-only root, no service-account token, ExternalSecret for `hermes-api-key`,
   `Egress` deny-by-default with **only the in-cluster Hermes gateway** (`duwende:8642`) allowed.
2. **Private by default.** Cloudflare Access on the whole host, verified in-process (ADR-0003's rule
   applies unchanged). No candidate accounts, no public chat. Audience = the owner.
3. **API-first, chat last.** Capabilities are registry tools with JSON endpoints and schemas
   (`extract_cv`, `score_against_jd`, `draft_questions`, `draft_email`, `redact_cv`). The chat window
   is the *last* milestone (M10) and is just another client. This matches the client's explicit
   instruction and ADR-0007.
4. **No database, no server-side history.** The thread lives in the browser and is posted per turn.
   Nothing about a candidate is retained by us. If history is ever wanted, that is a new ADR with a
   retention policy, not a feature commit.
5. **Provider behind configuration, reached through Hermes.** OpenAI-compatible HTTP to the in-cluster
   gateway (`hermes.duwende.svc:8642`), key `hermes-api-key` via ExternalSecret. Which upstream model
   answers is Hermes's configuration, not this app's; `hire` names a model only because the API
   requires the field. Swapping provider touches Hermes, not this repo.
6. **Text only in v1.** No file upload, no resume parsing from PDF, no storage, no virus scanning.
   Uploading files is a different threat model and a different milestone.
7. **Spend guard.** Per-user daily message cap plus a monthly provider budget; alert at 80 %, hard
   stop at 100 %. A public-ish surface with a paid API and no cap is a wallet with no lid.
8. **No prompt or output logging.** Same rule as gated tools (spec NFR-2.3). Request metadata only.

## Reasons

- The tool host is public and stateless; the assistant is private and handles third-party personal
  data. One binary would put the CV path inside the public app's trust boundary and its blast radius.
- The model, prompts and provider will change far more often than the tool host. Separate repos mean
  the assistant can ship ten times a week without touching the thing that serves public tools.
- No server-side history removes the entire "we hold a database of candidates" question. That is a
  product feature (privacy) and an engineering simplification at the same time.

## Costs (accepted)

- Two repos, two CI pipelines, two ArgoCD Applications, two hostnames — roughly double the
  housekeeping for double the products.
- A service token (ADR-0007) to manage between them.
- No conversation history across devices; closing the tab loses the thread. Deliberate.
- Model quality is the provider's, not ours; the value is in the prompts and the tool decomposition.

## Rejected

- **A chat page inside `tools-site`**: breaks the tool definition, mixes personal data with a public
  host, and makes every public tool deploy a risk to the assistant.
- **A candidate-facing chatbot**: a different product, with consent, retention and legal weight.
  Not this milestone and not this ADR.
- **Storing conversations** "for convenience": converts a stateless service into a personal-data
  processor with a retention obligation, for a feature the owner did not ask for.
- **A local model on the homelab**: attractive on cost and privacy, but a GPU and a serving stack are
  a different project; the provider stays swappable so this can be revisited cheaply.
