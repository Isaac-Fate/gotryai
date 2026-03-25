# Blog composing graph: reader-friendly, skim-first output

**Date:** 2026-03-25  
**Scope:** `examples/blog_composing_graph/main.go` (demo pipeline only; no new top-level examples unless explicitly requested).  
**Goal:** Generated posts should be **easy to scan**, **low cognitive load**, and **front-load the main knowledge** so a reader who only skims still gets the core takeaway. Expression must **not** feel templated or formulaic.

## Problem

The current graph produces competent but **essay-like** prose: sections are uniform 2–4 paragraphs, assembly is a straight concatenate, and the final pass optimizes flow/links but does not **design for skimming**. That yields dense, interchangeable “AI tone” and weak **information scent** in the first screenful of text.

## Design principles

1. **Inverted pyramid for the whole article:** the first markdown block after the title must answer, in plain language: what this is, who it’s for, and what to do or decide—before narrative detail.
2. **Skim layer is structured, not cosmetic:** a small snapshot block (bullets + one crisp verdict line) encodes *types* of information; **wording** stays flexible so outputs don’t all sound the same.
3. **Section-level BLUF without a fixed label:** each section opens with **one standalone sentence** that carries the section’s conclusion. **Forbidden:** repeating the same prefix every section (e.g. always “Takeaway:” / “小结：”).
4. **Anti-formula constraints in prompts:** ban stock openers and filler (“在当今时代”, “综上所述”, “值得注意的是” unless truly necessary). Require **variety** in how sections begin (assertion, contrast, short question, concrete scenario)—at least two different patterns per article.
5. **Honesty under uncertainty:** if search evidence is thin, snapshot and body may say what is **unverified** or **context-dependent** instead of padding with generic advice.

## Architecture

### State

Extend `BlogComposingGraphState` with a field, e.g. `ReaderSnapshotMarkdown string`, populated by a new node. Internally the node may use a short-lived struct matching JSON schema (see below) before rendering to markdown.

### Graph

Insert node **`reader_snapshot`** **after** `build_outline` and **before** `split_into_sections`.

Flow becomes:

`literature_review` → `build_outline` → **`reader_snapshot`** → `split_into_sections` → `[retrieve_evidence_for_section` → `draft_section`]* → `assemble_document` → `final_editorial_pass` → `save_document`

Rationale: the snapshot should align with title, thesis, audience, and section titles while remaining cheaper than post-hoc summarization of the full draft.

### Snapshot JSON schema (LLM output)

Structured output shape (field names indicative; exact Go struct names follow code style):

- `one_line_verdict` — single sentence: the article’s main claim or recommendation in concrete terms.
- `must_know_bullets` — 3–5 items; each **one line**; factual or actionable; no rhetorical questions.
- `who_its_for` — optional one short sentence.
- `not_for_or_risks` — optional one short sentence or bullet; **omit** if unsupported by draft/literature (model may leave empty).
- `common_pitfalls` — 0–2 short bullets; optional; evidence-grounded when possible.

**Rendering rules:**

- Emit markdown immediately under `# Title`: a **short `h2` chosen by the model** from an allowed set (e.g. “要点速览”, “读前快照”, “核心结论”)—**not** hardcoded to a single string in code; pass the chosen heading as a field `snapshot_h2` in JSON or derive in the same LLM call.
- Body of snapshot: `one_line_verdict` as normal paragraph or bold lead line; `must_know_bullets` as a list; optional lines merged without bloating.

### `assemble_document`

Order:

1. `# Title`
2. **Reader snapshot block** (from `ReaderSnapshotMarkdown`)
3. Optional italic thesis **only if** it adds something not already in the snapshot; otherwise omit or fold one line into snapshot (avoid redundancy—editor pass can enforce).
4. Section drafts in order.

### `draft_section` prompt changes

- First **paragraph** must be **exactly one sentence** (or one sentence + optional 3–5 word clarifier in em-dash form—implementation detail) that stands alone as the section takeaway.
- **Do not** use a repeated label prefix for that sentence across sections.
- Subsequent paragraphs: support, examples, links; keep sentences mostly short; prefer concrete nouns and steps.
- Retain existing evidence and link hygiene (no invented URLs).

### `final_editorial_pass` prompt changes

- **De-fluff:** remove filler, duplicate sayings, and redundant thesis restatements.
- **Skim coherence:** ensure the opening snapshot does not contradict the body; tighten snapshot bullets if the body refined the claim.
- **Anti-template:** vary connectors; strip cliché AI transitions; if two sections open the same way, rewrite one opening.
- **Preserve** all `[text](url)` URLs exactly as today.

### Bugfix in same change (small, same file)

`retrieve_evidence_for_section` query planner template currently hardcodes “Google Gemini… OpenClaw”. Replace with placeholders driven by `state.Outline.Title`, `state.Outline.Thesis`, `state.Draft`, and `sec.Title` only so the demo generalizes to any topic.

## Error handling and fallbacks

- If snapshot JSON fails to parse: **fallback** — call a simpler one-shot prompt (plain markdown list, no strict JSON) or set snapshot to a minimal placeholder instructing the final pass to synthesize a short bullet list from the assembled body before save. Demo must not panic.
- Search failures already propagate; snapshot generation should not depend on new searches (uses draft + literature review + outline only).

## Testing and acceptance

1. Run the example twice with the same embedded draft; compare first ~15 lines: both runs answer **what / for whom / first step** without contradictory fluff; **opening section hooks** should not be identical wording.
2. Manual read: snapshot readable in **under 30 seconds**; body does not repeat snapshot verbatim in every section.
3. `go build ./...` (or at least `go build` for the example package) succeeds.

## Non-goals

- New UI, CMS export, or i18n beyond what the model produces.
- Automatic evaluation metrics (BLEU, etc.).
- Splitting `main.go` into multiple files unless the file becomes unmaintainable after edits (YAGNI for this demo).

## Open decisions (locked for implementation)

- **Not_for / pitfalls:** included **optionally** when grounded; otherwise omitted.
- **Snapshot shape:** **list + one-line verdict** under a **model-chosen `h2`** from a small allowed set provided in the prompt.

## References

- Implementation plan: `docs/superpowers/plans/2026-03-25-blog-composing-reader-friendly.md`
