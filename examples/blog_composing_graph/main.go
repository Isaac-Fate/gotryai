package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/invopop/jsonschema"
	"github.com/joho/godotenv"
	"github.com/smallnest/langgraphgo/graph"
	"github.com/smallnest/langgraphgo/tool"
	"github.com/tmc/langchaingo/llms/openai"
	"github.com/tmc/langchaingo/prompts"
)

func main() {
	godotenv.Load()

	// Sample draft input — replace or wire to stdin/flag in a real app.
	draft := `
Title idea: Using Gemini on the web in Chrome with OpenClaw

Rough notes: Technical walkthrough for readers who run OpenClaw (or are
evaluating it) and want the model/browser path to be Google Gemini inside Chrome
rather than a headless or alternate stack. Cover why you might want in-browser
Gemini (accounts, extensions, live web UI), how configuration or tool routing
typically works at a high level, and practical pitfalls (auth, rate limits,
keeping Chrome in the loop). Audience: developers automating browsers or agent
runtimes. Tone: hands-on engineering, accurate names and steps — use web search
when specifics or versions are uncertain.
`

	bochaSearch, err := tool.NewBochaSearch(
		"",
		tool.WithBochaCount(8),
		tool.WithBochaSummary(true),
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

	g.AddNode("literature_review", "Broad Bocha search + synthesized overview and key resources",
		func(ctx context.Context, state BlogComposingGraphState) (BlogComposingGraphState, error) {
			qSchema := jsonschema.Reflect(&SearchQueriesJSON{})
			qSchemaBytes, err := json.MarshalIndent(qSchema, "", "  ")
			if err != nil {
				return state, err
			}
			qPt := prompts.NewPromptTemplate(`
The author will write a technical blog post. Plan 3–4 web search queries that give a
wide overview of the topic (official docs, GitHub, announcements, comparisons, setup guides).
Queries should complement each other, not repeat the same phrasing. Base this only on the
rough draft below.

Rough draft:
{{.draft}}

Return JSON only, matching this schema:
{{.schema}}
`, []string{"draft", "schema"})
			qPrompt, err := qPt.Format(map[string]any{
				"draft":  state.Draft,
				"schema": string(qSchemaBytes),
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
				raw, err := bochaSearch.Call(ctx, q)
				if err != nil {
					return state, fmt.Errorf("literature bocha search %q: %w", q, err)
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

1) Write "overview": 2–4 tight paragraphs on what the landscape looks like, main tools/concepts,
   and what a reader would need to know before a how-to article.
2) Fill "key_resources" with 6–14 items: title and url copied from the results, plus an optional
   short "note" on why it matters.

--- WEB SEARCH RESULTS ---
{{.search}}
--- END ---

Return JSON only, matching this schema:
{{.schema}}
`, []string{"search", "schema"})
			litPrompt, err := litPt.Format(map[string]any{
				"search": combined,
				"schema": string(litSchemaBytes),
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
Use the preliminary literature review to strengthen structure (fill gaps, order sections
logically, reflect what primary sources emphasize). If the review conflicts with the draft,
prefer being faithful to the draft's intent while aligning section titles with real-world terminology.

Rough draft:
{{.draft}}

Preliminary literature review (from web search):
{{.literature}}

Return JSON matching this schema:
{{.schema}}
`, []string{"draft", "literature", "schema"})
			prompt, err := pt.Format(map[string]any{
				"draft":     state.Draft,
				"literature": truncateRunes(state.LiteratureReview, 12000),
				"schema":    string(schemaBytes),
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

Use ONLY the material below (draft + literature excerpt + outline). Do not invent facts.
Leave not_for_or_risks empty and common_pitfalls empty unless clearly supported.

snapshot_h2 MUST be exactly one of: 要点速览, 读前快照, 核心结论, 先说重点 (pick the best fit).

must_know_bullets: 3–5 items; each one line; factual or actionable; no rhetorical questions.
one_line_verdict: one concrete sentence — the article's main claim or recommendation.

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
`, []string{"draft", "literature", "title", "thesis", "audience", "section_titles", "schema"})
			rsPrompt, err := rsPt.Format(map[string]any{
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

Rules:
- First line: ## then exactly one of 要点速览, 读前快照, 核心结论, 先说重点.
- Next: one short paragraph — the single most important verdict (concrete, no cliché openers).
- Then: a bullet list of 3–5 must-know points.
- Optional: one line on who this is NOT for, only if grounded in the text; otherwise omit.
Do not use filler such as "在当今时代" or "综上所述". Do not label bullets "TL;DR".

Rough draft:
{{.draft}}

Outline title: {{.title}}
Thesis: {{.thesis}}
Audience: {{.audience}}
Section titles: {{.section_titles}}

Literature excerpt:
{{.literature}}
`, []string{"draft", "title", "thesis", "audience", "section_titles", "literature"})
			fbPrompt, err := fbPt.Format(map[string]any{
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

	g.AddNode("retrieve_evidence_for_section", "Plan Bocha web searches, run them, synthesize evidence bullets",
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

Use the article title, thesis, draft notes, and section heading to infer the real topic,
products, and jargon. Produce 2–3 distinct queries that surface current, factual material
(docs, GitHub, release notes, setup guides) for THIS section only. Prefer specific names
from the draft/title. Do not invent URLs.

Blog working title: {{.title}}
Thesis: {{.thesis}}
Full draft notes: {{.draft}}
Section heading: {{.section_title}}

Literature review (context; do not duplicate whole-article searches unless this section needs it):
{{.literature_review}}

Return JSON only, matching this schema:
{{.schema}}
`, []string{"title", "thesis", "draft", "section_title", "literature_review", "schema"})
			qPrompt, err := qPt.Format(map[string]any{
				"title":             state.Outline.Title,
				"thesis":            state.Outline.Thesis,
				"draft":             state.Draft,
				"section_title":     sec.Title,
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
				raw, err := bochaSearch.Call(ctx, q)
				if err != nil {
					return state, fmt.Errorf("bocha search %q: %w", q, err)
				}
				searchBundles = append(searchBundles,
					fmt.Sprintf("### Query: %s\n%s", q, truncateRunes(raw, 6000)))
			}
			combined := strings.Join(searchBundles, "\n\n")
			if combined == "" {
				combined = "(no search results; rely on draft only)"
			}

			synthPt := prompts.NewPromptTemplate(`
You are a research assistant. From the WEB SEARCH RESULTS below, extract 3–8 concise
evidence bullets for the blog section. Each bullet must be a paraphrased fact or
actionable point grounded in the snippets. When a relevant page URL appears in the results,
end the bullet with a markdown link using that exact URL, e.g. "…see [docs](https://exact/url/from/results)."
Use a short, human anchor (not raw URLs as link text). At most one markdown link per bullet.
If something is uncertain in the results, say so briefly. Do not invent URLs. Ignore irrelevant hits.

Section: {{.section_title}}
Article thesis: {{.thesis}}

--- WEB SEARCH RESULTS ---
{{.search}}
--- END ---

Output plain text only: one bullet per line starting with "- ".
`, []string{"section_title", "thesis", "search"})
			synthPrompt, err := synthPt.Format(map[string]any{
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
			pt := prompts.NewPromptTemplate(`
Write ONE section of a blog post in Markdown.

Article title: {{.title}}
Thesis: {{.thesis}}
Section heading to use as ## {{.section_title}}

Background from literature review (optional context):
{{.literature_review}}

Evidence bullets from web research; each may contain markdown links — reuse those exact
URLs in prose where relevant (inline [text](url)); never invent links:
{{.evidence}}

Structure and voice:
- Start the body with EXACTLY ONE opening sentence (no label prefix like "Takeaway:", "小结：", "结论：").
  That sentence must stand alone as this section's verdict if the reader reads nothing else.
  Across sections in one article, VARY how you open: sometimes a blunt claim, sometimes a short contrast,
  sometimes a concrete scenario — do not reuse the same rhythm every time.
- Then 1–3 more short paragraphs: support, steps, nuance. Prefer short sentences, concrete nouns, and
  light lists where they reduce cognitive load.
- Avoid filler and stock phrases ("在当今时代", "值得注意的是", "综上所述", "不可或缺", or vague hype).
- Use the heading once as ## line; stay appropriate for audience: {{.audience}}.
- Cite helpful resources in the prose (not only at the end) when evidence bullets provide URLs.

Output only the section (heading + body), no preamble.
`, []string{"title", "thesis", "section_title", "literature_review", "evidence", "audience"})
			prompt, err := pt.Format(map[string]any{
				"title":             state.Outline.Title,
				"thesis":            state.Outline.Thesis,
				"section_title":     sec.Title,
				"literature_review": truncateRunes(state.LiteratureReview, 3500),
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

Goals:
- Remove fluff, throat-clearing, and redundant restatements (especially repeated thesis lines).
- Keep skim-value: the opening snapshot block (if present) must NOT contradict the body; tighten snapshot
  bullets if the body refined the claim.
- If two section openings use the same rhetorical pattern, rewrite one so the article feels less templated.
- Prefer short sentences where it helps; keep concrete nouns and steps.

CRITICAL: Preserve every markdown link [text](url) exactly (same URLs). You may reword
link text slightly if grammar requires it, but do not drop sources or fabricate URLs.

If several sections cite the same important pages, you may append a concise "## References"
section at the end with distinct links not already redundant in the body — otherwise omit it.

Output only the final markdown.

--- BEGIN ---
{{.md}}
--- END ---
`, []string{"md"})
			prompt, err := pt.Format(map[string]any{"md": state.AssembledMarkdown})
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

	g.AddEdge("literature_review", "build_outline")
	g.AddEdge("build_outline", "reader_snapshot")
	g.AddEdge("reader_snapshot", "split_into_sections")
	g.AddEdge("split_into_sections", "retrieve_evidence_for_section")
	g.AddEdge("retrieve_evidence_for_section", "draft_section")
	g.AddEdge("assemble_document", "final_editorial_pass")
	g.AddEdge("final_editorial_pass", "save_document")
	g.AddEdge("save_document", graph.END)

	g.SetEntryPoint("literature_review")

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
// Flow: literature_review → build_outline → reader_snapshot → split_into_sections →
//
//	[retrieve_evidence_for_section → draft_section]* → assemble_document →
//	final_editorial_pass → save_document
type BlogComposingGraphState struct {
	// Draft is the raw input from the author.
	Draft string `json:"draft"`

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

// OutlineJSON is the LLM JSON shape for build_outline.
type OutlineJSON struct {
	Title         string   `json:"title"`
	Thesis        string   `json:"thesis"`
	Audience      string   `json:"audience,omitempty"`
	SectionTitles []string `json:"section_titles"`
}

// SearchQueriesJSON is the LLM JSON shape for Bocha query planning.
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
	allowed := []string{"要点速览", "读前快照", "核心结论", "先说重点"}
	s = strings.TrimSpace(s)
	for _, a := range allowed {
		if s == a {
			return s
		}
	}
	return "要点速览"
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
