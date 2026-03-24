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
	"github.com/tmc/langchaingo/llms/openai"
	"github.com/tmc/langchaingo/prompts"
)

func main() {
	godotenv.Load()

	// Sample draft input — replace or wire to stdin/flag in a real app.
	draft := `
Title idea: Why small language-model graphs beat one-shot prompts

Rough notes: Want to argue that breaking generation into outline, evidence, and
section drafting improves coherence vs asking for a full article at once.
Mention LangGraph-style state and listeners for observability. Audience: backend
engineers evaluating agentic workflows. Tone: practical, not hype.
`

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

	g := graph.NewListenableStateGraph[BlogComposerState]()

	g.AddNode("build_outline", "Turn raw draft into a structured outline",
		func(ctx context.Context, state BlogComposerState) (BlogComposerState, error) {
			schema := jsonschema.Reflect(&OutlineJSON{})
			schemaBytes, err := json.MarshalIndent(schema, "", "  ")
			if err != nil {
				return state, err
			}
			pt := prompts.NewPromptTemplate(`
You are an editorial planner. Turn the author's rough draft into a concise outline.

Rough draft:
{{.draft}}

Return JSON matching this schema:
{{.schema}}
`, []string{"draft", "schema"})
			prompt, err := pt.Format(map[string]any{
				"draft":  state.Draft,
				"schema": string(schemaBytes),
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

	g.AddNode("split_into_sections", "Materialize outline into section slots for the loop",
		func(ctx context.Context, state BlogComposerState) (BlogComposerState, error) {
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

	g.AddNode("retrieve_evidence_for_section", "Gather supporting points for the current section (LLM-simulated retrieval)",
		func(ctx context.Context, state BlogComposerState) (BlogComposerState, error) {
			if state.CurrentIndex < 0 || state.CurrentIndex >= len(state.Sections) {
				return state, fmt.Errorf("invalid CurrentIndex %d for %d sections", state.CurrentIndex, len(state.Sections))
			}
			sec := &state.Sections[state.CurrentIndex]
			pt := prompts.NewPromptTemplate(`
You simulate "evidence retrieval" for one section of a blog post. Propose 3–6 concrete
bullet points (facts, definitions, examples, or citations the author could rely on).
Use only the draft and outline context — no real web access. Be specific.

Blog working title: {{.title}}
Thesis: {{.thesis}}
Full draft notes: {{.draft}}
Section title: {{.section_title}}

Reply with plain text: one bullet per line starting with "- ".
`, []string{"title", "thesis", "draft", "section_title"})
			prompt, err := pt.Format(map[string]any{
				"title":         state.Outline.Title,
				"thesis":        state.Outline.Thesis,
				"draft":         state.Draft,
				"section_title": sec.Title,
			})
			if err != nil {
				return state, err
			}
			resp, err := llm.Call(ctx, prompt)
			if err != nil {
				return state, err
			}
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
			sec.Evidence = bullets
			return state, nil
		},
	)

	g.AddNode("draft_section", "Write markdown for the current section, then advance index",
		func(ctx context.Context, state BlogComposerState) (BlogComposerState, error) {
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

Evidence bullets to weave in (optional):
{{.evidence}}

Requirements: 2–4 short paragraphs; use the heading once as ## line; stay on voice
for audience: {{.audience}}.
Output only the section (heading + body), no preamble.
`, []string{"title", "thesis", "section_title", "evidence", "audience"})
			prompt, err := pt.Format(map[string]any{
				"title":         state.Outline.Title,
				"thesis":        state.Outline.Thesis,
				"section_title": sec.Title,
				"evidence":      evidenceBlock,
				"audience":      state.Outline.Audience,
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

	g.AddConditionalEdge("draft_section", func(ctx context.Context, state BlogComposerState) string {
		if state.CurrentIndex < len(state.Sections) {
			return "retrieve_evidence_for_section"
		}
		return "assemble_document"
	})

	g.AddNode("assemble_document", "Concatenate section drafts into one markdown document",
		func(ctx context.Context, state BlogComposerState) (BlogComposerState, error) {
			var b strings.Builder
			b.WriteString("# ")
			b.WriteString(state.Outline.Title)
			b.WriteString("\n\n")
			if state.Outline.Thesis != "" {
				b.WriteString("*")
				b.WriteString(state.Outline.Thesis)
				b.WriteString("*\n\n")
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
		func(ctx context.Context, state BlogComposerState) (BlogComposerState, error) {
			pt := prompts.NewPromptTemplate(`
You are a senior editor. Polish the following blog post: fix flow, tighten wording,
fix markdown headings hierarchy if needed, keep the author's voice. Output only the
final markdown.

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
		func(ctx context.Context, state BlogComposerState) (BlogComposerState, error) {
			outDir := "."
			if d := os.Getenv("BLOG_COMPOSER_OUT_DIR"); d != "" {
				outDir = d
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

	g.AddEdge("build_outline", "split_into_sections")
	g.AddEdge("split_into_sections", "retrieve_evidence_for_section")
	g.AddEdge("retrieve_evidence_for_section", "draft_section")
	g.AddEdge("assemble_document", "final_editorial_pass")
	g.AddEdge("final_editorial_pass", "save_document")
	g.AddEdge("save_document", graph.END)

	g.SetEntryPoint("build_outline")

	g.AddGlobalListener(&EventLogger{})
	saveNode.AddListener(&SavedPathReporter{})

	runnable, err := g.CompileListenable()
	if err != nil {
		panic(err)
	}

	ctx := context.Background()
	initial := BlogComposerState{Draft: draft}

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

// BlogComposerState is the single source of truth for the composer graph.
// Flow: build_outline → split_into_sections → [retrieve_evidence_for_section ↔ draft_section]* →
//
//	assemble_document → final_editorial_pass → save_document
type BlogComposerState struct {
	// Draft is the raw input from the author.
	Draft string `json:"draft"`

	// Outline is filled by build_outline.
	Outline Outline `json:"outline"`

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

// --- Listeners ---

type EventLogger struct{}

func (l *EventLogger) OnNodeEvent(
	ctx context.Context, event graph.NodeEvent, nodeName string, state BlogComposerState, err error,
) {
	ts := time.Now().Format("15:04:05.000")
	printState := func() {
		redacted := state
		if len(redacted.Draft) > 400 {
			redacted.Draft = redacted.Draft[:400] + "…"
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
	ctx context.Context, event graph.NodeEvent, nodeName string, state BlogComposerState, err error,
) {
	if event != graph.NodeEventComplete || state.SavedPath == "" {
		return
	}
	fmt.Printf("\n  saved: %s\n", state.SavedPath)
}
