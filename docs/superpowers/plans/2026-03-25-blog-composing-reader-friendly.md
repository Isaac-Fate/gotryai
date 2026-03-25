# Blog composing graph: reader-friendly output — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Update `examples/blog_composing_graph/main.go` so posts include a **skim-first snapshot**, **anti-formula section openings**, and a stronger **de-fluff final pass**, plus fix the **hardcoded search-query topic** in `retrieve_evidence_for_section`.

**Architecture:** Add a **`reader_snapshot`** node after `build_outline` that uses the structured LLM to emit JSON, then render markdown into state. Adjust **`assemble_document`**, **`draft_section`**, and **`final_editorial_pass`** prompts. Add **parse fallback** for snapshot JSON failures. Generalize **section search query** prompt inputs.

**Tech stack:** Go, existing `langgraphgo` graph, `langchaingo` OpenAI-compat client (`deepseek-chat`), `invopop/jsonschema`, `joho/godotenv`.

**Spec:** `docs/superpowers/specs/2026-03-25-blog-composing-reader-friendly-design.md`

---

## File map

| File | Responsibility |
|------|------------------|
| `examples/blog_composing_graph/main.go` | All graph nodes, state, structs, prompts, assemble order, query template fix |

No new files required for the demo unless the implementer splits helpers (optional, YAGNI).

---

### Task 1: Snapshot JSON types and state

**Files:**
- Modify: `examples/blog_composing_graph/main.go`

- [ ] **Step 1:** Add `ReaderSnapshotJSON` struct matching spec fields (`one_line_verdict`, `must_know_bullets`, `who_its_for`, `not_for_or_risks`, `common_pitfalls`, `snapshot_h2` or equivalent). Use `json` tags consistent with existing `OutlineJSON` style.

- [ ] **Step 2:** Add `ReaderSnapshotMarkdown string` (or render inline without storing raw JSON) to `BlogComposingGraphState`.

- [ ] **Step 3:** Add `snapshotToMarkdown(ReaderSnapshotJSON) string` helper that renders title-adjacent `h2` + verdict + bullets + optional lines; skip empty optional fields.

- [ ] **Step 4:** Commit  
```bash
git add examples/blog_composing_graph/main.go
git commit -m "feat(blog_composing_graph): add reader snapshot types and markdown renderer"
```

---

### Task 2: `reader_snapshot` node and graph wiring

**Files:**
- Modify: `examples/blog_composing_graph/main.go`

- [ ] **Step 1:** Implement node using `llmStructured` + `jsonschema.Reflect(&ReaderSnapshotJSON{})`. Prompt inputs: `draft`, truncated `literature_review`, `outline` (title, thesis, audience, section titles). Instruct: choose `snapshot_h2` from allowed labels only; 3–5 bullets; optional `not_for` / pitfalls only if grounded.

- [ ] **Step 2:** On successful parse, set `state.ReaderSnapshotMarkdown = snapshotToMarkdown(...)`.

- [ ] **Step 3:** On JSON error: set snapshot from **fallback** — e.g. second prompt with plain markdown “3–5 bullets + one verdict line” via `llm.Call`, or set empty string and let final pass handle (spec prefers non-panic minimal list).

- [ ] **Step 4:** `g.AddNode("reader_snapshot", ...)`, `g.AddEdge("build_outline", "reader_snapshot")`, change next edge from `build_outline` → `split_into_sections` to `reader_snapshot` → `split_into_sections`. Update state comment flow diagram.

- [ ] **Step 5:** Commit  
```bash
git commit -am "feat(blog_composing_graph): add reader_snapshot node and edges"
```

---

### Task 3: `assemble_document` and search-query generalization

**Files:**
- Modify: `examples/blog_composing_graph/main.go`

- [ ] **Step 1:** Modify `assemble_document` to write: `# Title`, newline, `ReaderSnapshotMarkdown` (if non-empty), newline, optional thesis line per spec (omit if redundant—heuristic: if thesis empty skip; else include italic line only when not substring of snapshot; simplest demo: keep italic thesis after snapshot if non-empty).

- [ ] **Step 2:** Replace hardcoded Gemini/OpenClaw text in `retrieve_evidence_for_section` query `PromptTemplate` with topic drawn from `state.Outline.Title`, `state.Outline.Thesis`, `state.Draft`, `sec.Title`.

- [ ] **Step 3:** Commit  
```bash
git commit -am "feat(blog_composing_graph): assemble snapshot block; generalize section search queries"
```

---

### Task 4: `draft_section` and `final_editorial_pass` prompts

**Files:**
- Modify: `examples/blog_composing_graph/main.go`

- [ ] **Step 1:** Update `draft_section` template: first paragraph = one standalone sentence, varied openings, no repeated “Takeaway” labels; ban list of filler phrases; keep link rules.

- [ ] **Step 2:** Update `final_editorial_pass`: de-fluff, contradict snapshot vs body check, rewrite duplicate section openings; preserve URLs.

- [ ] **Step 3:** Commit  
```bash
git commit -am "feat(blog_composing_graph): anti-formula drafting and editorial pass"
```

---

### Task 5: Verify build and manual smoke

**Files:**
- None required

- [ ] **Step 1:** Run  
```bash
cd /Users/feiyiheng/projects/gotryai && go build -o /dev/null ./examples/blog_composing_graph/...
```
Expected: exit code 0.

- [ ] **Step 2:** Optional (needs API keys):  
```bash
go run ./examples/blog_composing_graph
```
Inspect `out/blog_post_*.md` first screenful for snapshot + non-formulaic openings.

- [ ] **Step 3:** Commit only if fixes needed from build.

---

## Plan review checklist

- [ ] All tasks reference exact file path  
- [ ] Fallback behavior for snapshot JSON documented and implemented  
- [ ] Graph edges and flow comment updated  
- [ ] No scope creep (single-file demo unless split necessary)
