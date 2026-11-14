# Spec — `hire` (HR assistant)

Status: **Stub v0.2** · Owner: @bismigel · Date: 2026-11-14 · App: `hire.ciceley.cc`
Companion to `spec.md` (the tool host). Decisions: `docs/adr/0007`, `docs/adr/0008`.
**Nothing here is buildable yet** — §6 is the planning-session agenda.

## 1. Purpose

A private assistant for a technical recruiter. It turns a job description plus a CV into a
defensible first read: fit summary, gaps, interview questions, and a draft reply. It exists so the
owner can spend time on judgement instead of re-typing the same screening prose.

Not: a candidate-facing chatbot, an applicant tracking system, a CV database, or a sourcing tool. It
does not find people, does not scrape LinkedIn, and does not buy data about candidates (ADR-0010).

## 2. Users and exposure

| | |
| --- | --- |
| Audience | the owner, a technical recruiter. Nobody else, including candidates. |
| Exposure | Cloudflare Access on the whole host; in-process JWT verification (same rule as `/lab`). |
| Data | job descriptions (owner-authored) and CVs (third-party personal data — see §5). |
| Retention | none. The thread lives in the browser; the server keeps no history (ADR-0008 §4). |

## 3. Requirements (draft — confirm in the planning session)

- **H-1** Every capability is a registry tool with a JSON endpoint and schema, callable without the
  chat UI. The chat window is the last milestone and only another client (ADR-0007).
- **H-2** Five capabilities, each one tool, shaped by the recruiter's workflow rather than by
  document type: `merge_profile` (CV + LinkedIn profile **text** → field-by-field merge with
  provenance and a discrepancy list), `screen_candidate` (job description + merged profile →
  verdict, rubric with quoted evidence, gaps, "not stated"), `interview_kit` (questions derived from
  the gaps), `draft_email` (next step or rejection, no invented facts), `redact_cv` (strip names,
  emails, phones, links, addresses — **optional**, offered at the top of the screening form).
- **H-3** No storage: no database, no server-side session, no uploaded files. Text in, text out.
- **H-4** No prompt or output values in logs; request metadata and token counts only.
- **H-5** Spend guard: per-user daily cap, monthly provider budget, alert at 80 %, stop at 100 %.
- **H-6** The assistant can call `tools-site` tools over MCP with its own service token
  (`spec.md` FR-9.4), so a screening can use `slugify`-class utilities without re-implementing them.
- **H-7** Every model answer that recommends an action is traceable to quoted text from the input.
  "The CV says X" must be a quote, not a paraphrase the model invented.
- **H-8** Non-answer is allowed: when the input is too thin, say what is missing instead of guessing.
- **H-9** A LinkedIn profile is accepted as **pasted text** and merged with the CV field by field: each
  field is marked **agreed / conflict / single source / missing**, with the original wording from each
  side preserved. The discrepancy list is part of the output. A pasted **URL** is rejected with copy
  instructions, never fetched — no scraping, no stored session, no enrichment vendor (ADR-0010).
  It MAY take an optional `profile_url`, validated and echoed as the source link, **never fetched**.
  An agent MAY *hand* the profile text to `merge_profile` over WebMCP/MCP; the tool still never
  fetches, and no workflow may depend on the agent existing.

## 4. Prompt and model discipline

- Provider: OpenAI-compatible HTTP; base URL, model name and key from config (ADR-0008 §5).
  Default `deepseek-chat`; a swap is one ConfigMap line and one secret.
- Prompts live in the repo as versioned text files, with a fixed test set: **3 job descriptions ×
  5 CVs** (invented or the owner's own), and output properties rather than golden strings — every
  claim quoted, every gap named, no invented employer/title/date/qualification, "not stated" used
  where the CV is silent. Prompt edits are code changes: reviewed, tested, reversible.
- The model may not see `tools-site`'s credentials, other CVs, or the owner's email content.
- Temperature and max tokens fixed in config, not per request, so behaviour is reproducible.

## 5. Personal data

CVs contain names, contact details, employment history and sometimes health or nationality
information. Sending them to a model provider is a data transfer to that provider, under their
terms, in their jurisdiction.

**Decided 2026-11-14 (client):** redaction is **optional**, not enforced. The safe path is offered,
built and tested; the screening form warns when it detects an email address, phone number or link,
and suggests `redact_cv`. It cannot reliably detect a *name*, and the UI says so.

**Consequences, written down rather than hidden:**

- The system defaults to test documents (the owner's own CV, invented CVs). Real CVs are permitted at
  the owner's risk, not because the system made them safe.
- Nothing is stored and nothing is logged: one turn in flight, then forgotten.
- The provider is configured to retain nothing and train on nothing — verified, not assumed.
- If this ever runs on real candidates, the employer's policy is the owner's to check. A homelab is
  not an employer's IT system.

## 6. Resolved in the 2026-11-14 planning session

| Question | Answer |
| --- | --- |
| Capability split | four workflow tools, not five document tools (H-2) |
| What "good" means for screening | rubric with quoted evidence, gaps, "not stated"; no bare score |
| Whose CVs | test documents by default; real CVs at the owner's risk, warned, never logged |
| Provider and spend | `deepseek-chat` behind config; 100 messages/day, £25/month, alert at 80 % |
| Redaction | optional, offered and tested (H-2) |
| Private tools over MCP | none in v0 — public tools only, so no service token yet |
| Chat UI | last (M10); two panes, browser-held thread; wizard is the documented fallback |
| Quality bar | fixed 3×5 fixture set, run on every prompt change |
| Profile enrichment | paste-only merge with provenance; no scraping, no URL fetch, no vendor API (ADR-0010) |

**Still open:** the provider/model name (`spec.md` Q10 — an environment value, not a blocker) and
whether the assistant ever needs a gated tool (which would bring the service token back).
