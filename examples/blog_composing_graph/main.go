package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	mytool "gotryai/pkg/tool"

	"github.com/invopop/jsonschema"
	"github.com/joho/godotenv"
	"github.com/smallnest/langgraphgo/graph"
	"github.com/tmc/langchaingo/llms/openai"
	"github.com/tmc/langchaingo/prompts"
)

func main() {
	godotenv.Load()

	// Sample draft input — replace or wire to stdin/flag in a real app.
	// 	draft := `
	// Title idea: Let openclaw drive the Gemini web UI in your browser

	// Rough notes: Technical walkthrough for readers who has installed openclaw on their machine
	// and wants let openclaw to take over the browser and interact with the Gemini web UI;
	// need to install skill: gemini-browser from clawhub, prepfer one-liner install, just send message "help me install the skill gemini-browser from clawhub" in the openclaw console, and install the openclaw browser replay extension in chrome https://chromewebstore.google.com/detail/openclaw-browser-relay/nglingapjinhecnfejdcpihlpneeadjp?pli=1, then configure something
	// `

	draft := `
Title idea: try langgraphgo

Rough notes: it is this pacakge smallnest/langgraphgo; basic examples of making an AI agent app, invoke agent, structured output, workflow graph, agent, etc.
`

	// Web search backend is pluggable; prompts refer to "web search" only so swapping
	// providers (e.g. Brave) does not require prompt edits.
	webSearch, err := mytool.NewDuckDuckGoSearch(
		mytool.WithDuckCount(8),
	)
	if err != nil {
		panic(err)
	}

	llm, err := openai.New(
		openai.WithBaseURL("https://api.deepseek.com"),
		openai.WithToken(os.Getenv("DEEPSEEK_API_KEY")),
		openai.WithModel("deepseek-chat"),
	)
	if err != nil {
		panic(err)
	}

	llmStructured, err := openai.New(
		openai.WithBaseURL("https://api.deepseek.com"),
		openai.WithToken(os.Getenv("DEEPSEEK_API_KEY")),
		openai.WithModel("deepseek-chat"),
		openai.WithResponseFormat(openai.ResponseFormatJSON),
	)
	if err != nil {
		panic(err)
	}

	g := graph.NewListenableStateGraph[BlogComposingGraphState]()

	g.AddNode("author_brief", "Extract binding goals, out-of-scope, and must-include from the draft only",
		func(ctx context.Context, state BlogComposingGraphState) (BlogComposingGraphState, error) {
			abSchema := jsonschema.Reflect(&AuthorBriefJSON{})
			abSchemaBytes, err := json.MarshalIndent(abSchema, "", "  ")
			if err != nil {
				return state, err
			}
			abPt := prompts.NewPromptTemplate(`
Extract a structured AUTHOR BRIEF from the ROUGH DRAFT only. Do not use outside knowledge
or assumptions beyond what is written.

This brief is binding for all later steps: they must not add popular tangents that contradict
the draft (infer tangents only from what the draft explicitly rules out or from a clear mismatch
between in_scope and typical “helpful” filler).

Fields:
- primary_goal: one sentence capturing what success means for the reader.
- in_scope: bullet strings — what the finished post IS about (be specific; use the draft’s terms).
- out_of_scope: bullet strings — main narratives, article shapes, or whole chapters the author is NOT asking for.
  Derive these from the draft: if the author centers workflow A, list “making workflow B the spine of the post”
  here when the draft did not ask for B. Quote or paraphrase the draft; do not invent product names not in the draft.
- must_include: concrete demands from the draft only — exact commands or messages to copy (quote verbatim),
  exact URLs, version pins, file paths, or UI steps the draft names.
- do_not_emphasize: topics web search often over-weights that the draft did not center on; keep each item tied
  to draft evidence or to obvious conflict with in_scope.

If the draft states a preferred command, message, or one-liner, you MUST copy that exact string into must_include.

Rough draft:
{{.draft}}

Return JSON only, matching this schema:
{{.schema}}
`, []string{"draft", "schema"})
			abPrompt, err := abPt.Format(map[string]any{
				"draft":  state.Draft,
				"schema": string(abSchemaBytes),
			})
			if err != nil {
				return state, err
			}
			abResp, err := llmStructured.Call(ctx, abPrompt)
			if err != nil {
				return state, err
			}
			var ab AuthorBriefJSON
			if err := json.Unmarshal([]byte(abResp), &ab); err != nil {
				state.AuthorBrief = AuthorBriefJSON{}
				return state, nil
			}
			state.AuthorBrief = ab
			return state, nil
		},
	)

	g.AddNode("literature_review", "Broad web search + synthesized overview and key resources",
		func(ctx context.Context, state BlogComposingGraphState) (BlogComposingGraphState, error) {
			qSchema := jsonschema.Reflect(&SearchQueriesJSON{})
			qSchemaBytes, err := json.MarshalIndent(qSchema, "", "  ")
			if err != nil {
				return state, err
			}
			qPt := prompts.NewPromptTemplate(`
The author will write a technical blog post. Plan 3-4 web search queries (for any search
provider) that give a wide overview of the topic (official docs, GitHub, announcements,
comparisons, setup guides).
Queries should complement each other, not repeat the same phrasing. Bias queries toward the
AUTHOR BRIEF — avoid centering the query plan on topics listed under out_of_scope or
do_not_emphasize unless the brief explicitly asks for them.

Rough draft:
{{.draft}}

AUTHOR BRIEF:
{{.author_brief}}

Return JSON only, matching this schema:
{{.schema}}
`, []string{"draft", "author_brief", "schema"})
			qPrompt, err := qPt.Format(map[string]any{
				"draft":        state.Draft,
				"author_brief": authorBriefForPrompt(state.AuthorBrief),
				"schema":       string(qSchemaBytes),
			})
			if err != nil {
				return state, err
			}
			qResp, err := llmStructured.Call(ctx, qPrompt)
			if err != nil {
				return state, err
			}
			var sq SearchQueriesJSON
			if err := json.Unmarshal([]byte(qResp), &sq); err != nil {
				return state, fmt.Errorf("literature search query JSON: %w", err)
			}
			queries := normalizeLiteratureQueries(sq.Queries, state.Draft)
			var bundles []string
			for _, q := range queries {
				raw, err := webSearch.Call(ctx, q)
				if err != nil {
					return state, fmt.Errorf("literature web search %q: %w", q, err)
				}
				bundles = append(bundles, fmt.Sprintf("### Query: %s\n%s", q, truncateRunes(raw, 7000)))
			}
			combined := strings.Join(bundles, "\n\n")
			if combined == "" {
				combined = "(no search results)"
			}

			litSchema := jsonschema.Reflect(&LiteratureReviewJSON{})
			litSchemaBytes, err := json.MarshalIndent(litSchema, "", "  ")
			if err != nil {
				return state, err
			}
			litPt := prompts.NewPromptTemplate(`
You are doing a short literature review for a technical blog. Using ONLY the WEB SEARCH
RESULTS below (do not invent URLs; every key_resources.url must appear verbatim in the results):

The AUTHOR BRIEF reflects the author's draft and OVERRIDES the tone of search snippets when they
stress paths the author marked out_of_scope or do_not_emphasize. Summarize the landscape in a way
that supports the brief — do not reframe the post around topics the brief excludes.

1) Write "overview": 2-4 tight paragraphs: tools/concepts a reader needs *given the author brief*,
   not a generic survey of whatever search hits emphasize.
2) Fill "key_resources" with 6-14 items: title and url copied from the results, plus an optional
   short "note" on why it matters *to this brief*.

AUTHOR BRIEF:
{{.author_brief}}

--- WEB SEARCH RESULTS ---
{{.search}}
--- END ---

Return JSON only, matching this schema:
{{.schema}}
`, []string{"author_brief", "search", "schema"})
			litPrompt, err := litPt.Format(map[string]any{
				"author_brief": authorBriefForPrompt(state.AuthorBrief),
				"search":       combined,
				"schema":       string(litSchemaBytes),
			})
			if err != nil {
				return state, err
			}
			litResp, err := llmStructured.Call(ctx, litPrompt)
			if err != nil {
				return state, err
			}
			var lit LiteratureReviewJSON
			if err := json.Unmarshal([]byte(litResp), &lit); err != nil {
				return state, fmt.Errorf("literature review JSON: %w", err)
			}
			state.LiteratureReview = literatureReviewToMarkdown(lit)
			return state, nil
		},
	)

	g.AddNode("build_outline", "Turn raw draft into a structured outline",
		func(ctx context.Context, state BlogComposingGraphState) (BlogComposingGraphState, error) {
			schema := jsonschema.Reflect(&OutlineJSON{})
			schemaBytes, err := json.MarshalIndent(schema, "", "  ")
			if err != nil {
				return state, err
			}
			pt := prompts.NewPromptTemplate(`
You are an editorial planner. Turn the author's rough draft into a concise outline.

The AUTHOR BRIEF is binding (extracted from the same draft). Use the literature review only
where it supports that brief.

Hard rules:
- Do NOT add section titles whose primary job is an out-of_scope or do_not_emphasize narrative from the brief.
- Every item under must_include in the brief must be plannable in the outline (dedicated step or clearly woven
  into a named section).
- If the author specified a preferred exact command, message, or one-liner, the outline must allow a section
  where that appears prominently — not only generic “alternative” procedures unless the draft also asks for them.

Depth and shape (unless the draft explicitly demands a very short post):
- Aim for roughly 5–9 sections so the article can introduce, explain context, walk through setup, and cover pitfalls.
- The first section must be a real introduction (problem, audience, what “success” looks like, and a roadmap of later sections),
  not only a shallow TL;DR before jump-cutting to commands.

Rough draft:
{{.draft}}

AUTHOR BRIEF:
{{.author_brief}}

Preliminary literature review (from web search):
{{.literature}}

Return JSON matching this schema:
{{.schema}}
`, []string{"draft", "author_brief", "literature", "schema"})
			prompt, err := pt.Format(map[string]any{
				"draft":        state.Draft,
				"author_brief": authorBriefForPrompt(state.AuthorBrief),
				"literature":   truncateRunes(state.LiteratureReview, 12000),
				"schema":       string(schemaBytes),
			})
			if err != nil {
				return state, err
			}
			resp, err := llmStructured.Call(ctx, prompt)
			if err != nil {
				return state, err
			}
			var oj OutlineJSON
			if err := json.Unmarshal([]byte(resp), &oj); err != nil {
				return state, err
			}
			state.Outline = Outline{
				Title:         oj.Title,
				Thesis:        oj.Thesis,
				Audience:      oj.Audience,
				SectionTitles: oj.SectionTitles,
			}
			return state, nil
		},
	)

	g.AddNode("reader_snapshot", "Skim-first snapshot from outline + literature (JSON or markdown fallback)",
		func(ctx context.Context, state BlogComposingGraphState) (BlogComposingGraphState, error) {
			rsSchema := jsonschema.Reflect(&ReaderSnapshotJSON{})
			rsSchemaBytes, err := json.MarshalIndent(rsSchema, "", "  ")
			if err != nil {
				return state, err
			}
			sectionsJoined := strings.Join(state.Outline.SectionTitles, ", ")
			rsPt := prompts.NewPromptTemplate(`
You shape the TOP of a technical blog for readers who may only skim.

Use ONLY the material below (author brief + draft + literature excerpt + outline). Do not invent facts.
The AUTHOR BRIEF wins over search-heavy tangents: must_know_bullets must reflect in_scope and must_include,
not generic checklists for topics the brief lists under out_of_scope or do_not_emphasize unless the draft explicitly needs them.

Leave not_for_or_risks empty and common_pitfalls empty unless clearly supported.

snapshot_h2 MUST be exactly one of: At a glance, Read this first, Bottom line, What matters (pick the best fit).

must_know_bullets: 3-5 items; each one line; factual or actionable; include verbatim must_include items when present in the brief.
  These bullets are skim aids only; they must NOT replace a full introductory section later (problem framing + resource digest + roadmap).
one_line_verdict: one concrete sentence — aligned with primary_goal in the brief.

AUTHOR BRIEF:
{{.author_brief}}

Rough draft:
{{.draft}}

Literature review (markdown excerpt):
{{.literature}}

Working title: {{.title}}
Thesis: {{.thesis}}
Audience: {{.audience}}
Section titles (in order): {{.section_titles}}

Return JSON only, matching this schema:
{{.schema}}
`, []string{"author_brief", "draft", "literature", "title", "thesis", "audience", "section_titles", "schema"})
			rsPrompt, err := rsPt.Format(map[string]any{
				"author_brief":   authorBriefForPrompt(state.AuthorBrief),
				"draft":          state.Draft,
				"literature":     truncateRunes(state.LiteratureReview, 10000),
				"title":          state.Outline.Title,
				"thesis":         state.Outline.Thesis,
				"audience":       state.Outline.Audience,
				"section_titles": sectionsJoined,
				"schema":         string(rsSchemaBytes),
			})
			if err != nil {
				return state, err
			}
			rsResp, err := llmStructured.Call(ctx, rsPrompt)
			if err != nil {
				return state, err
			}
			var rsj ReaderSnapshotJSON
			if jsonErr := json.Unmarshal([]byte(rsResp), &rsj); jsonErr == nil &&
				(strings.TrimSpace(rsj.OneLineVerdict) != "" || len(rsj.MustKnowBullets) > 0) {
				state.ReaderSnapshotMarkdown = snapshotToMarkdown(rsj)
				return state, nil
			}
			fbPt := prompts.NewPromptTemplate(`
Write ONLY markdown for the top of a technical blog (skim-first). No preamble.

Follow the AUTHOR BRIEF: bullets must include concrete must_include items when relevant (verbatim
strings from the draft). Do not headline topics the brief places under out_of_scope or do_not_emphasize unless
the draft explicitly requires a short aside there.

Rules:
- First line: ## then exactly one of: At a glance, Read this first, Bottom line, What matters.
- Next: one short paragraph — the single most important verdict (concrete, no cliché openers).
- Then: a bullet list of 3-5 must-know points (skim aids; the first body section must still deliver a full intro).
- Optional: one line on who this is NOT for, only if grounded in the text; otherwise omit.
Do not use filler such as "In today's rapidly changing world" or "In conclusion," as empty throat-clearing. Do not label bullets "TL;DR".

AUTHOR BRIEF:
{{.author_brief}}

Rough draft:
{{.draft}}

Outline title: {{.title}}
Thesis: {{.thesis}}
Audience: {{.audience}}
Section titles: {{.section_titles}}

Literature excerpt:
{{.literature}}
`, []string{"author_brief", "draft", "title", "thesis", "audience", "section_titles", "literature"})
			fbPrompt, err := fbPt.Format(map[string]any{
				"author_brief":   authorBriefForPrompt(state.AuthorBrief),
				"draft":          state.Draft,
				"title":          state.Outline.Title,
				"thesis":         state.Outline.Thesis,
				"audience":       state.Outline.Audience,
				"section_titles": sectionsJoined,
				"literature":     truncateRunes(state.LiteratureReview, 8000),
			})
			if err != nil {
				return state, err
			}
			fbOut, err2 := llm.Call(ctx, fbPrompt)
			if err2 != nil {
				state.ReaderSnapshotMarkdown = ""
				return state, nil
			}
			state.ReaderSnapshotMarkdown = strings.TrimSpace(fbOut)
			return state, nil
		},
	)

	g.AddNode("split_into_sections", "Materialize outline into section slots for the loop",
		func(ctx context.Context, state BlogComposingGraphState) (BlogComposingGraphState, error) {
			titles := state.Outline.SectionTitles
			if len(titles) == 0 {
				titles = []string{"Introduction", "Main argument", "Conclusion"}
			}
			secs := make([]SectionWork, 0, len(titles))
			for i, t := range titles {
				secs = append(secs, SectionWork{
					ID:    fmt.Sprintf("sec-%d", i),
					Title: t,
				})
			}
			state.Sections = secs
			state.CurrentIndex = 0
			return state, nil
		},
	)

	g.AddNode("retrieve_evidence_for_section", "Plan web searches, run them, synthesize evidence bullets",
		func(ctx context.Context, state BlogComposingGraphState) (BlogComposingGraphState, error) {
			if state.CurrentIndex < 0 || state.CurrentIndex >= len(state.Sections) {
				return state, fmt.Errorf("invalid CurrentIndex %d for %d sections", state.CurrentIndex, len(state.Sections))
			}
			sec := &state.Sections[state.CurrentIndex]

			schema := jsonschema.Reflect(&SearchQueriesJSON{})
			schemaBytes, err := json.MarshalIndent(schema, "", "  ")
			if err != nil {
				return state, err
			}
			qPt := prompts.NewPromptTemplate(`
You plan web searches for ONE section of a technical blog post.

Use the article title, thesis, draft notes, section heading, and AUTHOR BRIEF. Produce 2-3 distinct
queries for THIS section only. Do not craft queries whose main payoff is material the AUTHOR BRIEF lists under
out_of_scope or do_not_emphasize unless this section’s heading clearly requires it per the draft.

Blog working title: {{.title}}
Thesis: {{.thesis}}
Full draft notes: {{.draft}}
Section heading: {{.section_title}}

AUTHOR BRIEF:
{{.author_brief}}

Literature review (context; do not duplicate whole-article searches unless this section needs it):
{{.literature_review}}

Return JSON only, matching this schema:
{{.schema}}
`, []string{"title", "thesis", "draft", "section_title", "author_brief", "literature_review", "schema"})
			qPrompt, err := qPt.Format(map[string]any{
				"title":             state.Outline.Title,
				"thesis":            state.Outline.Thesis,
				"draft":             state.Draft,
				"section_title":     sec.Title,
				"author_brief":      authorBriefForPrompt(state.AuthorBrief),
				"literature_review": truncateRunes(state.LiteratureReview, 4000),
				"schema":            string(schemaBytes),
			})
			if err != nil {
				return state, err
			}
			qResp, err := llmStructured.Call(ctx, qPrompt)
			if err != nil {
				return state, err
			}
			var sq SearchQueriesJSON
			if err := json.Unmarshal([]byte(qResp), &sq); err != nil {
				return state, fmt.Errorf("search query JSON: %w", err)
			}
			queries := normalizeSearchQueries(sq.Queries, state.Outline.Title, sec.Title)
			var searchBundles []string
			for _, q := range queries {
				raw, err := webSearch.Call(ctx, q)
				if err != nil {
					return state, fmt.Errorf("web search %q: %w", q, err)
				}
				searchBundles = append(searchBundles,
					fmt.Sprintf("### Query: %s\n%s", q, truncateRunes(raw, 9000)))
			}
			combined := strings.Join(searchBundles, "\n\n")
			if combined == "" {
				combined = "(no search results; rely on draft only)"
			}

			synthPt := prompts.NewPromptTemplate(`
You are a research assistant. From the WEB SEARCH RESULTS below, produce 4–10 evidence bullets for THIS section.

Each bullet must be 2–4 sentences (not one-liners): (1) the fact or takeaway grounded in the snippets,
(2) a short "digest" of what that source page is useful for — what a reader learns, which steps or concepts it covers,
(3) when a relevant URL exists in the results, end with ONE markdown link using that exact URL:
"... [short label](https://exact/url/from/results)."
Use a short, human anchor for the link (not the raw URL as link text). At most one markdown link per bullet.
If something is uncertain in the results, say so briefly. Do not invent URLs. Ignore irrelevant hits.

AUTHOR BRIEF (binding): Deprioritize or omit bullets that mainly support do_not_emphasize / out_of_scope
topics when they would steer the section away from the author's draft. Prefer evidence aligned with
in_scope and must_include.

AUTHOR BRIEF:
{{.author_brief}}

Section: {{.section_title}}
Article thesis: {{.thesis}}

--- WEB SEARCH RESULTS ---
{{.search}}
--- END ---

Output plain text only: one bullet per line starting with "- ".
`, []string{"author_brief", "section_title", "thesis", "search"})
			synthPrompt, err := synthPt.Format(map[string]any{
				"author_brief":  authorBriefForPrompt(state.AuthorBrief),
				"section_title": sec.Title,
				"thesis":        state.Outline.Thesis,
				"search":        combined,
			})
			if err != nil {
				return state, err
			}
			resp, err := llm.Call(ctx, synthPrompt)
			if err != nil {
				return state, err
			}
			sec.Evidence = parseBulletLines(resp)
			return state, nil
		},
	)

	g.AddNode("draft_section", "Write markdown for the current section, then advance index",
		func(ctx context.Context, state BlogComposingGraphState) (BlogComposingGraphState, error) {
			if state.CurrentIndex < 0 || state.CurrentIndex >= len(state.Sections) {
				return state, fmt.Errorf("invalid CurrentIndex %d for %d sections", state.CurrentIndex, len(state.Sections))
			}
			sec := &state.Sections[state.CurrentIndex]
			evidenceBlock := strings.Join(sec.Evidence, "\n- ")
			if evidenceBlock != "" {
				evidenceBlock = "- " + evidenceBlock
			}

			sectionRole := sectionRoleInstructions(state.CurrentIndex == 0)

			pt := prompts.NewPromptTemplate(`
Write ONE section of a blog post in Markdown.

{{.section_role}}

AUTHOR BRIEF (binding — wins over evidence when they conflict):
- Honor in_scope; do not build the section around out_of_scope or do_not_emphasize topics.
- Every string listed under must_include must appear verbatim somewhere in the FINAL whole article.
  For THIS section: when the section title and draft imply setup, installation, first-run, or copy-paste steps,
  weave the relevant must_include items here (exact commands, URLs, or messages from the draft).
- If the draft states a preferred procedure, present it first in the matching section before optional alternatives
  the draft did not request.

AUTHOR BRIEF:
{{.author_brief}}

Original rough draft (full context; do not contradict):
{{.draft}}

Article title: {{.title}}
Thesis: {{.thesis}}
Section heading to use as ## {{.section_title}}

Background from literature review (optional context):
{{.literature_review}}

Evidence bullets from web research (each may summarize a source + link). Reuse exact URLs in prose as
inline [text](url); never invent links. When you cite a resource, include a brief knowledge digest:
what the reader gets from that page (steps, definitions, or perspective) — grounded only in the evidence/literature.
{{.evidence}}

Structure and voice:
- After the ## heading, write a substantive body. Do not compress into a single short paragraph per section.
- Start the body with EXACTLY ONE opening sentence (no label prefix like "Takeaway:", "Summary:", "Conclusion:").
  That sentence must stand alone as this section's verdict if the reader reads nothing else.
  Across sections in one article, VARY how you open: sometimes a blunt claim, sometimes a short contrast,
  sometimes a concrete scenario — do not reuse the same rhythm every time.
- Follow with enough paragraphs to meet the length target in SECTION ROLE; weave steps, tradeoffs, and digests
  of linked material — not bare link lists.
- Prefer short sentences, concrete nouns, and light lists where they reduce cognitive load.
- Avoid filler and stock phrases ("in today's world", "it is worth noting", "in conclusion," "indispensable", or vague hype).
- Stay appropriate for audience: {{.audience}}.

Output only the section (heading + body), no preamble.
`, []string{"section_role", "author_brief", "draft", "title", "thesis", "section_title", "literature_review", "evidence", "audience"})
			prompt, err := pt.Format(map[string]any{
				"section_role":      sectionRole,
				"author_brief":      authorBriefForPrompt(state.AuthorBrief),
				"draft":             state.Draft,
				"title":             state.Outline.Title,
				"thesis":            state.Outline.Thesis,
				"section_title":     sec.Title,
				"literature_review": truncateRunes(state.LiteratureReview, 8000),
				"evidence":          evidenceBlock,
				"audience":          state.Outline.Audience,
			})
			if err != nil {
				return state, err
			}
			resp, err := llm.Call(ctx, prompt)
			if err != nil {
				return state, err
			}
			sec.Draft = strings.TrimSpace(resp)
			state.CurrentIndex++
			return state, nil
		},
	)

	g.AddConditionalEdge("draft_section", func(ctx context.Context, state BlogComposingGraphState) string {
		if state.CurrentIndex < len(state.Sections) {
			return "retrieve_evidence_for_section"
		}
		return "assemble_document"
	})

	g.AddNode("assemble_document", "Concatenate section drafts into one markdown document",
		func(ctx context.Context, state BlogComposingGraphState) (BlogComposingGraphState, error) {
			var b strings.Builder
			b.WriteString("# ")
			b.WriteString(state.Outline.Title)
			b.WriteString("\n\n")
			if snap := strings.TrimSpace(state.ReaderSnapshotMarkdown); snap != "" {
				b.WriteString(snap)
				b.WriteString("\n\n")
			}
			thesis := strings.TrimSpace(state.Outline.Thesis)
			if thesis != "" {
				snapLower := strings.ToLower(state.ReaderSnapshotMarkdown)
				if !strings.Contains(snapLower, strings.ToLower(thesis)) {
					b.WriteString("*")
					b.WriteString(thesis)
					b.WriteString("*\n\n")
				}
			}
			for _, sec := range state.Sections {
				b.WriteString(sec.Draft)
				b.WriteString("\n\n")
			}
			state.AssembledMarkdown = strings.TrimSpace(b.String())
			return state, nil
		},
	)

	g.AddNode("final_editorial_pass", "Polish the full markdown for publication",
		func(ctx context.Context, state BlogComposingGraphState) (BlogComposingGraphState, error) {
			pt := prompts.NewPromptTemplate(`
You are a senior editor. Polish the following blog post for publication.

The ORIGINAL DRAFT and AUTHOR BRIEF are the source of truth for scope.

Goals:
- Remove or demote dominant sections that violate the AUTHOR BRIEF (out_of_scope / do_not_emphasize narratives).
  If stray web-search material conflicts with in_scope, cut or demote it unless the original draft explicitly
  needed a brief mention; at most one short aside — otherwise delete.
- Ensure every item under must_include in the brief appears verbatim in the final article. If any are missing,
  add a tight paragraph in the correct section without inventing new requirements beyond the draft.
- Length and depth: for a tutorial-style post like this, target roughly **1,800–3,500 words** total unless the
  draft explicitly requires brevity. If the piece is thin mostly due to terseness (not lack of sources), **expand**
  using only facts and digests already implied by the linked material and body — do not add new URLs or new products.
- The **introduction** must read as a full section: problem, audience, conceptual framing, digest of what key linked
  resources contain, and a roadmap. If the intro is only quick setup bullets, rewrite it into expository paragraphs
  while keeping must_include verbatim.
- Each substantive section should teach: when links appear, the reader should see what those pages are *for*, not only hrefs.
- Remove fluff, throat-clearing, and redundant restatements (especially repeated thesis lines).
- Keep skim-value: the opening snapshot block (if present) must NOT contradict the body; tighten snapshot
  bullets if the body refined the claim.
- If two section openings use the same rhetorical pattern, rewrite one so the article feels less templated.
- Prefer short sentences where it helps; keep concrete nouns and steps.

AUTHOR BRIEF:
{{.author_brief}}

Original rough draft (for alignment):
{{.draft}}

CRITICAL: Preserve every markdown link [text](url) exactly (same URLs). You may reword
link text slightly if grammar requires it, but do not drop sources or fabricate URLs.

If several sections cite the same important pages, you may append a concise "## References"
section at the end with distinct links not already redundant in the body — otherwise omit it.

Output only the final markdown.

--- BEGIN ---
{{.md}}
--- END ---
`, []string{"author_brief", "draft", "md"})
			prompt, err := pt.Format(map[string]any{
				"author_brief": authorBriefForPrompt(state.AuthorBrief),
				"draft":        state.Draft,
				"md":           state.AssembledMarkdown,
			})
			if err != nil {
				return state, err
			}
			resp, err := llm.Call(ctx, prompt)
			if err != nil {
				return state, err
			}
			state.FinalPost = strings.TrimSpace(resp)
			return state, nil
		},
	)

	saveNode := g.AddNode("save_document", "Persist final post to disk",
		func(ctx context.Context, state BlogComposingGraphState) (BlogComposingGraphState, error) {
			outDir := "out"
			if d := os.Getenv("BLOG_COMPOSER_OUT_DIR"); d != "" {
				outDir = d
			}
			if err := os.MkdirAll(outDir, 0o755); err != nil {
				return state, err
			}
			name := fmt.Sprintf("blog_post_%s.md", time.Now().Format("20060102_150405"))
			path := filepath.Join(outDir, name)
			if err := os.WriteFile(path, []byte(state.FinalPost), 0o644); err != nil {
				return state, err
			}
			state.SavedPath = path
			return state, nil
		},
	)

	g.AddEdge("author_brief", "literature_review")
	g.AddEdge("literature_review", "build_outline")
	g.AddEdge("build_outline", "reader_snapshot")
	g.AddEdge("reader_snapshot", "split_into_sections")
	g.AddEdge("split_into_sections", "retrieve_evidence_for_section")
	g.AddEdge("retrieve_evidence_for_section", "draft_section")
	g.AddEdge("assemble_document", "final_editorial_pass")
	g.AddEdge("final_editorial_pass", "save_document")
	g.AddEdge("save_document", graph.END)

	g.SetEntryPoint("author_brief")

	g.AddGlobalListener(&EventLogger{})
	saveNode.AddListener(&SavedPathReporter{})

	runnable, err := g.CompileListenable()
	if err != nil {
		panic(err)
	}

	ctx := context.Background()
	initial := BlogComposingGraphState{Draft: draft}

	fmt.Println()
	fmt.Printf("[%s] chain start\n", time.Now().Format("15:04:05.000"))
	final, err := runnable.Invoke(ctx, initial)
	fmt.Printf("[%s] chain end\n", time.Now().Format("15:04:05.000"))
	if err != nil {
		panic(err)
	}

	out, _ := json.MarshalIndent(final, "", "  ")
	fmt.Println()
	fmt.Println("final state:")
	fmt.Println(string(out))
	fmt.Println()
}

// --- Graph state (typed, documents the full pipeline) ---

// BlogComposingGraphState is the single source of truth for the composer graph.
// Flow: author_brief → literature_review → build_outline → reader_snapshot → split_into_sections →
//
//	[retrieve_evidence_for_section → draft_section]* → assemble_document →
//	final_editorial_pass → save_document
type BlogComposingGraphState struct {
	// Draft is the raw input from the author.
	Draft string `json:"draft"`

	// AuthorBrief is extracted from the draft only (binding constraints for all later nodes).
	AuthorBrief AuthorBriefJSON `json:"author_brief,omitempty"`

	// LiteratureReview is overview + key resources (markdown) from literature_review.
	LiteratureReview string `json:"literature_review,omitempty"`

	// Outline is filled by build_outline.
	Outline Outline `json:"outline"`

	// ReaderSnapshotMarkdown is the skim-first block placed under the title (from reader_snapshot).
	ReaderSnapshotMarkdown string `json:"reader_snapshot_markdown,omitempty"`

	// Sections is populated by split_into_sections; each item gains Evidence then Draft.
	Sections []SectionWork `json:"sections"`

	// CurrentIndex is the loop cursor: retrieve/draft read Sections[CurrentIndex];
	// draft_section increments it after writing. When CurrentIndex == len(Sections), the loop ends.
	CurrentIndex int `json:"current_index"`

	// AssembledMarkdown is the stitched article before editing.
	AssembledMarkdown string `json:"assembled_markdown,omitempty"`

	// FinalPost is the editor-polished markdown.
	FinalPost string `json:"final_post,omitempty"`

	// SavedPath is set by save_document.
	SavedPath string `json:"saved_path,omitempty"`
}

// Outline is structured planning output from the first node.
type Outline struct {
	Title         string   `json:"title"`
	Thesis        string   `json:"thesis"`
	Audience      string   `json:"audience,omitempty"`
	SectionTitles []string `json:"section_titles"`
}

// SectionWork is one outline section flowing through evidence + drafting.
type SectionWork struct {
	ID       string   `json:"id"`
	Title    string   `json:"title"`
	Evidence []string `json:"evidence,omitempty"`
	Draft    string   `json:"draft,omitempty"`
}

// AuthorBriefJSON is the LLM JSON shape for author_brief (draft-only extraction).
type AuthorBriefJSON struct {
	PrimaryGoal    string   `json:"primary_goal"`
	InScope        []string `json:"in_scope"`
	OutOfScope     []string `json:"out_of_scope"`
	MustInclude    []string `json:"must_include"`
	DoNotEmphasize []string `json:"do_not_emphasize"`
}

func authorBriefForPrompt(b AuthorBriefJSON) string {
	if strings.TrimSpace(b.PrimaryGoal) == "" && len(b.InScope) == 0 && len(b.MustInclude) == 0 &&
		len(b.OutOfScope) == 0 && len(b.DoNotEmphasize) == 0 {
		return "(No author brief extracted — follow the rough draft literally.)"
	}
	var w strings.Builder
	w.WriteString("primary_goal: ")
	w.WriteString(strings.TrimSpace(b.PrimaryGoal))
	w.WriteString("\n")
	writeBullets := func(label string, xs []string) {
		if len(xs) == 0 {
			return
		}
		w.WriteString(label)
		w.WriteString(":\n")
		for _, s := range xs {
			s = strings.TrimSpace(s)
			if s == "" {
				continue
			}
			w.WriteString("- ")
			w.WriteString(s)
			w.WriteString("\n")
		}
	}
	writeBullets("in_scope", b.InScope)
	writeBullets("out_of_scope", b.OutOfScope)
	writeBullets("must_include (verbatim where quoted in draft)", b.MustInclude)
	writeBullets("do_not_emphasize", b.DoNotEmphasize)
	return strings.TrimSpace(w.String())
}

// OutlineJSON is the LLM JSON shape for build_outline.
type OutlineJSON struct {
	Title         string   `json:"title"`
	Thesis        string   `json:"thesis"`
	Audience      string   `json:"audience,omitempty"`
	SectionTitles []string `json:"section_titles"`
}

// SearchQueriesJSON is the LLM JSON shape for web search query planning.
type SearchQueriesJSON struct {
	Queries []string `json:"queries"`
}

// LiteratureReviewJSON is the structured literature pass for build_outline.
type LiteratureReviewJSON struct {
	Overview     string                   `json:"overview"`
	KeyResources []LiteratureResourceJSON `json:"key_resources"`
}

// LiteratureResourceJSON is one cited source from the literature review.
type LiteratureResourceJSON struct {
	Title string `json:"title"`
	URL   string `json:"url"`
	Note  string `json:"note,omitempty"`
}

// ReaderSnapshotJSON is the LLM JSON shape for reader_snapshot.
type ReaderSnapshotJSON struct {
	OneLineVerdict  string   `json:"one_line_verdict"`
	MustKnowBullets []string `json:"must_know_bullets"`
	WhoItsFor       string   `json:"who_its_for,omitempty"`
	NotForOrRisks   string   `json:"not_for_or_risks,omitempty"`
	CommonPitfalls  []string `json:"common_pitfalls,omitempty"`
	SnapshotH2      string   `json:"snapshot_h2"`
}

func normalizeSnapshotH2(s string) string {
	allowed := []string{"At a glance", "Read this first", "Bottom line", "What matters"}
	s = strings.TrimSpace(s)
	for _, a := range allowed {
		if s == a {
			return s
		}
	}
	return "At a glance"
}

func snapshotToMarkdown(j ReaderSnapshotJSON) string {
	h2 := normalizeSnapshotH2(j.SnapshotH2)
	var b strings.Builder
	b.WriteString("## ")
	b.WriteString(h2)
	b.WriteString("\n\n")
	if v := strings.TrimSpace(j.OneLineVerdict); v != "" {
		b.WriteString(v)
		b.WriteString("\n\n")
	}
	for _, item := range j.MustKnowBullets {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		b.WriteString("- ")
		b.WriteString(item)
		b.WriteString("\n")
	}
	if who := strings.TrimSpace(j.WhoItsFor); who != "" {
		b.WriteString("\n")
		b.WriteString(who)
		b.WriteString("\n")
	}
	if notFor := strings.TrimSpace(j.NotForOrRisks); notFor != "" {
		b.WriteString("\n")
		b.WriteString(notFor)
		b.WriteString("\n")
	}
	if len(j.CommonPitfalls) > 0 {
		b.WriteString("\n")
		for _, p := range j.CommonPitfalls {
			p = strings.TrimSpace(p)
			if p == "" {
				continue
			}
			b.WriteString("- ")
			b.WriteString(p)
			b.WriteString("\n")
		}
	}
	return strings.TrimSpace(b.String())
}

func literatureReviewToMarkdown(j LiteratureReviewJSON) string {
	var b strings.Builder
	overview := strings.TrimSpace(j.Overview)
	if overview != "" {
		b.WriteString(overview)
	}
	if len(j.KeyResources) > 0 {
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString("### Key resources\n\n")
		for _, r := range j.KeyResources {
			url := strings.TrimSpace(r.URL)
			if url == "" {
				continue
			}
			title := strings.TrimSpace(r.Title)
			if title == "" {
				title = url
			}
			line := fmt.Sprintf("- [%s](%s)", title, url)
			if note := strings.TrimSpace(r.Note); note != "" {
				line += " — " + note
			}
			b.WriteString(line)
			b.WriteString("\n")
		}
	}
	return strings.TrimSpace(b.String())
}

func normalizeLiteratureQueries(qs []string, draft string) []string {
	var out []string
	seen := make(map[string]struct{})
	for _, q := range qs {
		q = strings.TrimSpace(q)
		if q == "" {
			continue
		}
		key := strings.ToLower(q)
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, q)
		if len(out) >= 4 {
			break
		}
	}
	if len(out) > 0 {
		return out
	}
	d := strings.TrimSpace(draft)
	if len(d) > 200 {
		d = d[:200]
	}
	if d != "" {
		return []string{
			d + " documentation overview",
			d + " official guide OR GitHub",
		}
	}
	return []string{
		"technical topic overview documentation",
		"getting started guide tutorial",
	}
}

func normalizeSearchQueries(qs []string, articleTitle, sectionTitle string) []string {
	var out []string
	seen := make(map[string]struct{})
	for _, q := range qs {
		q = strings.TrimSpace(q)
		if q == "" {
			continue
		}
		key := strings.ToLower(q)
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, q)
		if len(out) >= 3 {
			break
		}
	}
	if len(out) > 0 {
		return out
	}
	at := strings.TrimSpace(articleTitle)
	st := strings.TrimSpace(sectionTitle)
	return []string{
		fmt.Sprintf("%s %s documentation", at, st),
		fmt.Sprintf("%s %s tutorial setup", at, st),
	}
}

func truncateRunes(s string, max int) string {
	if max <= 0 {
		return s
	}
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "…"
}

func parseBulletLines(resp string) []string {
	var bullets []string
	for _, line := range strings.Split(resp, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		line = strings.TrimPrefix(line, "-")
		line = strings.TrimPrefix(line, "•")
		line = strings.TrimSpace(line)
		if line != "" {
			bullets = append(bullets, line)
		}
	}
	return bullets
}

func sectionRoleInstructions(isIntroduction bool) string {
	if isIntroduction {
		return `SECTION ROLE — OPENING / INTRODUCTION (first section of the article):
- Target length: about 700–1,400 words in the body (after the ## heading) when the evidence and literature support it.
- Write at least 5 meaty paragraphs (most paragraphs 4–7 sentences). Do not stop at a thin hook + roadmap.
- Paragraphs 1–2: who this is for, the problem, and why driving Gemini in Chrome via OpenClaw matters (vs API-only or headless-only stories).
- Next paragraphs: a **knowledge digest** of the most important external resources — for each major link you rely on from the evidence/literature,
  spend a few sentences explaining what that page/documentation covers and what the reader gains (not just naming the link).
- Include a clear **roadmap**: name the upcoming sections and what each will deliver (still prose, not a bare bullet list unless helpful).
- You may use a short bullet list only where it truly aids skimming; the bulk must be expository paragraphs.
- The skim block under the title (if any) is not a substitute for this full introduction.`
	}
	return `SECTION ROLE — BODY SECTION (not the article introduction):
- Target length: about 450–900 words in the body after the ## heading for core/tutorial sections; 350–600 for short wrap-ups.
- Write at least 4 substantive paragraphs unless this section is intentionally a brief checklist the outline marks as such.
- Whenever you reference a URL from the evidence, add 1–3 sentences digesting what that resource explains or how it fits the workflow.
- Prefer teaching and context over telegraphic step-only blurbs; ground claims in the evidence bullets.`
}

// --- Listeners ---

type EventLogger struct{}

func (l *EventLogger) OnNodeEvent(
	ctx context.Context, event graph.NodeEvent, nodeName string, state BlogComposingGraphState, err error,
) {
	ts := time.Now().Format("15:04:05.000")
	printState := func() {
		redacted := state
		if len(redacted.Draft) > 400 {
			redacted.Draft = redacted.Draft[:400] + "…"
		}
		if len(redacted.LiteratureReview) > 600 {
			redacted.LiteratureReview = redacted.LiteratureReview[:600] + "…"
		}
		if len(redacted.ReaderSnapshotMarkdown) > 500 {
			redacted.ReaderSnapshotMarkdown = redacted.ReaderSnapshotMarkdown[:500] + "…"
		}
		j, _ := json.MarshalIndent(redacted, "    ", "  ")
		fmt.Printf("    state:\n%s\n", j)
	}
	switch event {
	case graph.NodeEventStart:
		fmt.Printf("[%s] ▶ %q\n", ts, nodeName)
		printState()
	case graph.NodeEventComplete:
		fmt.Printf("[%s] ✓ %q\n", ts, nodeName)
		printState()
	case graph.NodeEventError:
		fmt.Printf("[%s] ✗ %q: %v\n", ts, nodeName, err)
		printState()
	default:
		fmt.Printf("[%s] %s %q\n", ts, event, nodeName)
	}
}

type SavedPathReporter struct{}

func (r *SavedPathReporter) OnNodeEvent(
	ctx context.Context, event graph.NodeEvent, nodeName string, state BlogComposingGraphState, err error,
) {
	if event != graph.NodeEventComplete || state.SavedPath == "" {
		return
	}
	fmt.Printf("\n  saved: %s\n", state.SavedPath)
}
