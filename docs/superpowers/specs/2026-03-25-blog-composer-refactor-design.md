# Blog Composing Graph Refactor — Adaptive Blueprint

**Date:** 2026-03-25
**Location:** `examples/blog_composing_graph/`
**Approach:** Adaptive Blueprint (Approach A)

## Problem

The current blog composing graph produces short, template-looking posts. Every section goes through the same prompt and gets the same "opening sentence + 4 paragraphs" shape. There are no mermaid diagrams, no code listings, no personality. The outline is a flat list of titles, so every post reads like "Introduction, Step 1, Step 2, Conclusion."

## Goals

- Produce 2,000–3,500 word technical blog posts worth bookmarking.
- Always include mermaid diagrams (at least 2 per post) and code listings (at least 2 per post).
- Support any technical post type: tutorial, exploration, deep-dive, opinion, hybrid — detected from draft.
- Non-template structure: varied section types, creative headings, narrative arc.
- Personal voice and opinions woven throughout.
- Input remains freeform rough notes with optional structured hints.

## Non-Goals

- Generating non-technical blog posts (marketing, listicles, etc.).
- Interactive editing or human-in-the-loop mid-pipeline.
- Multi-language output (English only for now).

---

## Input Format

The graph input is a single `draft` string. It supports an optional YAML frontmatter block:

```
voice: skeptical systems engineer who's seen too many ORMs
style: opinionated, code-heavy
must_include:
  - https://github.com/smallnest/langgraphgo
  - "go get github.com/smallnest/langgraphgo"
---
Title idea: try langgraphgo

Rough notes: basic examples of making an AI agent app...
```

If no `---` separator exists, the entire string is treated as freeform notes and everything is inferred.

---

## Graph State

```go
type State struct {
    Draft          string         // raw author input
    DraftMeta      DraftMeta      // parsed hints + LLM-detected analysis; populated by analyze_draft
    KnowledgeBase  string         // markdown: overview + evidence bullets with URLs
    Blueprint      Blueprint      // creative structure with section types + artifact specs
    Sections       []SectionDraft // rendered markdown per section; appended by write_rich_section
    CurrentIndex   int            // loop cursor: indexes Blueprint.Sections; incremented after each write
    FinalPost      string         // reused across assemble → voice_pass → final_edit (each overwrites)
    SavedPath      string         // output file path
}

type DraftMeta struct {
    // Parsed from YAML frontmatter (empty if no frontmatter)
    Voice       string   // e.g. "skeptical systems engineer"
    Style       string   // e.g. "opinionated, code-heavy"
    MustInclude []string // exact URLs, commands, strings to include verbatim
    FreeformBody string  // the draft text after frontmatter extraction

    // Detected by LLM in analyze_draft
    PostType  string // "tutorial", "exploration", "deep-dive", "opinion", "hybrid"
    Audience  string // one sentence
    CoreClaim string // one sentence — what the post argues or demonstrates
}

type Blueprint struct {
    Title        string
    Thesis       string
    Audience     string
    PostType     string        // "tutorial", "exploration", "deep-dive", "opinion", "hybrid"
    NarrativeArc string        // 1-2 sentences: the emotional/intellectual journey
    Sections     []SectionSpec
}

type SectionSpec struct {
    ID          string   // "sec-0", "sec-1", ...
    Title       string   // creative heading
    Type        string   // "narrative", "how-to", "comparison", "deep-dive", "aside", "conclusion"
    Purpose     string   // 1 sentence: what this section achieves
    Artifacts   []string // "mermaid-architecture", "mermaid-flow", "code-go", "code-bash", etc.
    SearchHints []string // optional: extra queries for this section
}

type SectionDraft struct {
    ID       string
    Title    string
    Markdown string // fully rendered section with diagrams and code
}
```

---

## Pipeline: 8 Nodes

```
analyze_draft → research → design_blueprint → write_rich_section (loop) → assemble → voice_pass → final_edit → save
```

### Graph Edges

```
g.AddEdge("analyze_draft", "research")
g.AddEdge("research", "design_blueprint")
g.AddEdge("design_blueprint", "write_rich_section")
g.AddConditionalEdge("write_rich_section", func(...) string {
    if state.CurrentIndex < len(state.Blueprint.Sections) {
        return "write_rich_section"
    }
    return "assemble"
})
g.AddEdge("assemble", "voice_pass")
g.AddEdge("voice_pass", "final_edit")
g.AddEdge("final_edit", "save")
g.AddEdge("save", graph.END)
g.SetEntryPoint("analyze_draft")
```

### Section Loop Mechanics

`CurrentIndex` starts at 0. Each `write_rich_section` invocation reads `Blueprint.Sections[CurrentIndex]`, produces one `SectionDraft`, appends it to `State.Sections`, and increments `CurrentIndex`. The conditional edge loops back to `write_rich_section` until `CurrentIndex == len(Blueprint.Sections)`.

### Intermediate State Reuse

`FinalPost` is written by `assemble`, then overwritten in-place by `voice_pass`, then overwritten again by `final_edit`. No separate `AssembledMarkdown` or `VoicePassed` fields — each stage replaces `FinalPost`.

### Node 1: `analyze_draft`

**Replaces:** `author_brief`

Parse YAML frontmatter if present → populate `DraftMeta.Voice`, `.Style`, `.MustInclude`, `.FreeformBody`. Then one LLM call (structured JSON) to detect and store on `DraftMeta`:
- `PostType` (tutorial / exploration / deep-dive / opinion / hybrid)
- `Audience` (one sentence)
- `CoreClaim` (one sentence — what the post argues or demonstrates)

`design_blueprint` uses `DraftMeta.PostType` as the seed for `Blueprint.PostType` (may refine it), `DraftMeta.Audience` for `Blueprint.Audience`, and `DraftMeta.CoreClaim` as the basis for `Blueprint.Thesis` (may sharpen it based on research). After `design_blueprint`, all downstream nodes (`write_rich_section`, `voice_pass`, etc.) read from `Blueprint.*` as the canonical source — `DraftMeta.*` is only used as fallback if `Blueprint` fields are empty.

This replaces the over-engineered author brief with its out_of_scope/do_not_emphasize machinery.

### Node 2: `research`

**Replaces:** `literature_review` + per-section `retrieve_evidence_for_section`

One batch research pass:
1. LLM plans 4–6 search queries covering the whole topic (structured JSON).
2. Execute all queries via `webSearch.Call`.
3. LLM synthesizes combined results into a knowledge base document:
   - 2–3 overview paragraphs
   - 8–15 evidence bullets, each 2–3 sentences with one markdown link from search results
   - Key resources list with URLs

This knowledge base feeds into all later nodes. No more per-section search.

### Node 3: `design_blueprint`

**Replaces:** `build_outline` + `reader_snapshot` + `split_into_sections`

Given draft + DraftMeta + knowledge base, produce the full `Blueprint` JSON.

Prompt constraints:
- **Vary section types.** A good post mixes types: narrative → how-to → aside → deep-dive → comparison → conclusion. Never produce 5 consecutive same-type sections.
- **Plan artifacts explicitly.** For each section, list required mermaid diagrams and code blocks. Minimum across the whole post: 2 mermaid diagrams, 2 code blocks.
- **Creative headings.** Not "Step 1: Install X" — use headings like "Getting the pieces on the board" or "Where things go sideways."
- **Narrative arc.** Describe the post's journey: "Start with the frustration of manual config, build toward the 'aha' of automated flows, end with what's still rough."
- **5–8 sections** for a 2,000–3,500 word target.

### Node 4: `write_rich_section` (looped)

**Replaces:** `draft_section`

For each `SectionSpec`, dynamically build a prompt based on `SectionSpec.Type`:

| Type | Prompt emphasis | Target words |
|---|---|---|
| `narrative` | Story-driven, set context, analogies, no bullet lists | 400–700 |
| `how-to` | Steps with code blocks, brief explanations, gotchas | 500–800 |
| `comparison` | Table or side-by-side, then trade-off analysis | 400–600 |
| `deep-dive` | Explain internals, mermaid diagram for architecture/flow | 500–900 |
| `aside` | Short personal note, opinion, anecdote. Conversational | 100–250 |
| `conclusion` | Honest verdict, what's good, what's rough, next steps. No "In conclusion" | 200–400 |

Each prompt receives:
- The section's `Artifacts` list (which mermaid/code to include)
- The knowledge base, capped to 10,000 runes; if over, prefer bullets containing any case-insensitive substring from `SearchHints` or whitespace-split section title words
- The `NarrativeArc` so the section knows its place in the story
- Prior section titles + last 400 runes of the previous section body (for continuity; no separate summary generation). For the first section (`CurrentIndex == 0`), these are empty.
- `DraftMeta.Voice` for tone consistency

Mermaid diagrams and code listings are generated inline within each section — not bolted on as an afterthought.

### Node 5: `assemble`

Concatenate: `# {Title}\n\n` + all section markdowns joined by `\n\n`.

No more reader_snapshot block at the top. The first section (narrative type) serves as the hook.

### Node 6: `voice_pass` (NEW)

Operates on the full assembled post. Prompt:
- Add 2–4 personal asides or parenthetical opinions.
- Add human transitions between sections.
- If `DraftMeta.Voice` is set, lean into that persona.
- Default voice: "experienced engineer on personal blog — direct, occasionally wry, doesn't oversell."
- **Do not change** code blocks, mermaid diagrams, URLs, or factual claims.

### Node 7: `final_edit`

Light polish:
- Fix heading hierarchy.
- Remove accidental duplicates or contradictions.
- Ensure mermaid blocks use correct fencing.
- Preserve all URLs.
- If the post is under 1,800 words (whitespace-split token count), log a warning to stdout (no state field needed; informational only).
- Mermaid/code minimums (2 each) are best-effort via prompt instructions; no repair node. If the model under-delivers, the post ships as-is.

### Node 8: `save`

Write to `out/` directory (or `BLOG_COMPOSER_OUT_DIR` env var). Create directory if needed.

---

## File Structure

```
examples/blog_composing_graph/
├── main.go      ~60 lines   graph wiring, draft input, LLM/search init, run
├── state.go     ~80 lines   State, Blueprint, SectionSpec, SectionDraft, DraftMeta types
├── nodes.go     ~350 lines  8 node functions as top-level funcs returning closures
├── prompts.go   ~250 lines  prompt constants + buildSectionPrompt(SectionSpec) dynamic builder
└── helpers.go   ~120 lines  parseDraftMeta, truncateRunes, parseBulletLines, normalizeQueries
```

Total: ~860 lines (down from ~1,149).

Each node function in `nodes.go` is structured as:
```go
func researchNode(llm, llmStructured llms.Model, search tools.Tool) graph.NodeFunc[State] {
    return func(ctx context.Context, state State) (State, error) {
        // ...
    }
}
```

This keeps `main.go` clean — it only wires nodes and edges.

---

## Key Prompt Principles (across all nodes)

1. **No filler phrases.** Explicitly ban: "In today's world", "It is worth noting", "In conclusion", "indispensable", "game-changer."
2. **Mermaid diagrams use ` ```mermaid` fencing.** Always. Types: flowchart, sequence, class, state.
3. **Code listings specify language.** Always ` ```go`, ` ```bash`, ` ```yaml`, etc.
4. **Links are inline.** `[label](url)` in prose, not footnotes. Only use URLs from the knowledge base — never invent.
5. **Varied rhythm.** Each section type has different structural rules. No two consecutive sections should open the same way.
6. **Personal > corporate.** Prefer "I found that..." over "It has been observed that..."

---

## What's Removed

- `author_brief` node and all AuthorBriefJSON machinery (out_of_scope, do_not_emphasize, etc.)
- `reader_snapshot` node and ReaderSnapshotJSON / snapshotToMarkdown
- `sectionRoleInstructions()` function
- Per-section search (retrieve_evidence_for_section) — replaced by batch research
- Formulaic snapshot headings ("At a glance", "Read this first", etc.)
- The `authorBriefForPrompt()` formatting function

---

## Error Handling

- **Invalid YAML frontmatter:** Treat entire draft as freeform (no DraftMeta hints); log a warning.
- **Failed JSON parse from LLM:** Return error, fail the graph. Do not silently continue with empty state.
- **Empty `Blueprint.Sections`:** Return error ("blueprint produced no sections").
- **Web search failure:** Return error. If partial results are available, use them (skip failed queries, proceed with what succeeded). If all queries fail, set `KnowledgeBase` to "(no search results available)" and continue — the LLM can still write from the draft alone.
- **`write_rich_section` produces empty markdown:** Log a warning, store an empty `SectionDraft`, continue to next section.
