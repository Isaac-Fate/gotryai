package blogcomposer

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/invopop/jsonschema"
	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/prompts"
	"github.com/tmc/langchaingo/tools"
)

func analyzeDraftNode(llmStructured llms.Model) func(context.Context, State) (State, error) {
	return func(ctx context.Context, state State) (State, error) {
		state.DraftMeta = parseDraftMeta(state.Draft)

		schema := jsonschema.Reflect(&draftAnalysisJSON{})
		schemaBytes, _ := json.MarshalIndent(schema, "", "  ")

		body := draftBody(state)
		pt := prompts.NewPromptTemplate(promptAnalyzeDraft, []string{"draft", "schema"})
		prompt, err := pt.Format(map[string]any{
			"draft":  body,
			"schema": string(schemaBytes),
		})
		if err != nil {
			return state, err
		}
		resp, err := llms.GenerateFromSinglePrompt(ctx, llmStructured, prompt)
		if err != nil {
			return state, err
		}
		var analysis draftAnalysisJSON
		if err := json.Unmarshal([]byte(resp), &analysis); err != nil {
			return state, fmt.Errorf("analyze_draft JSON: %w", err)
		}
		state.DraftMeta.PostType = analysis.PostType
		state.DraftMeta.Audience = analysis.Audience
		state.DraftMeta.CoreClaim = analysis.CoreClaim
		return state, nil
	}
}

func researchNode(
	llm, llmStructured llms.Model,
	webSearch tools.Tool,
) func(context.Context, State) (State, error) {
	return func(ctx context.Context, state State) (State, error) {
		qSchema := jsonschema.Reflect(&searchQueriesJSON{})
		qSchemaBytes, _ := json.MarshalIndent(qSchema, "", "  ")

		body := draftBody(state)
		qPt := prompts.NewPromptTemplate(promptResearchQueries,
			[]string{"draft", "post_type", "core_claim", "schema"})
		qPrompt, err := qPt.Format(map[string]any{
			"draft":      body,
			"post_type":  state.DraftMeta.PostType,
			"core_claim": state.DraftMeta.CoreClaim,
			"schema":     string(qSchemaBytes),
		})
		if err != nil {
			return state, err
		}
		qResp, err := llms.GenerateFromSinglePrompt(ctx, llmStructured, qPrompt)
		if err != nil {
			return state, fmt.Errorf("research queries: %w", err)
		}
		var sq searchQueriesJSON
		if err := json.Unmarshal([]byte(qResp), &sq); err != nil {
			return state, fmt.Errorf("research query JSON: %w", err)
		}
		queries := normalizeQueries(sq.Queries, 6)
		if len(queries) == 0 {
			queries = []string{body}
		}

		var bundles []string
		for _, q := range queries {
			raw, err := webSearch.Call(ctx, q)
			if err != nil {
				fmt.Fprintf(os.Stderr, "warning: search %q failed: %v\n", q, err)
				continue
			}
			bundles = append(bundles, fmt.Sprintf("### Query: %s\n%s", q, truncateRunes(raw, 8000)))
		}
		combined := strings.Join(bundles, "\n\n")
		if combined == "" {
			state.KnowledgeBase = "(no search results available)"
			return state, nil
		}

		kbSchema := jsonschema.Reflect(&knowledgeBaseJSON{})
		kbSchemaBytes, _ := json.MarshalIndent(kbSchema, "", "  ")

		synthPt := prompts.NewPromptTemplate(promptResearchSynthesize,
			[]string{"draft", "search", "schema"})
		synthPrompt, err := synthPt.Format(map[string]any{
			"draft":  body,
			"search": combined,
			"schema": string(kbSchemaBytes),
		})
		if err != nil {
			return state, err
		}
		synthResp, err := llmStructured.Call(ctx, synthPrompt)
		if err != nil {
			return state, err
		}
		var kb knowledgeBaseJSON
		if err := json.Unmarshal([]byte(synthResp), &kb); err != nil {
			return state, fmt.Errorf("research synthesis JSON: %w", err)
		}
		state.KnowledgeBase = knowledgeBaseToMarkdown(kb)
		return state, nil
	}
}

func designBlueprintNode(llmStructured llms.Model) func(context.Context, State) (State, error) {
	return func(ctx context.Context, state State) (State, error) {
		bpSchema := jsonschema.Reflect(&blueprintJSON{})
		bpSchemaBytes, _ := json.MarshalIndent(bpSchema, "", "  ")

		body := draftBody(state)
		mustIncludeBlock := ""
		if len(state.DraftMeta.MustInclude) > 0 {
			mustIncludeBlock = "MUST INCLUDE (verbatim in the final post):\n"
			for _, item := range state.DraftMeta.MustInclude {
				mustIncludeBlock += "- " + item + "\n"
			}
		}

		pt := prompts.NewPromptTemplate(
			promptDesignBlueprint,
			[]string{
				"draft",
				"post_type",
				"audience",
				"core_claim",
				"knowledge_base",
				"must_include_block",
				"schema",
			},
		)
		prompt, err := pt.Format(map[string]any{
			"draft":              body,
			"post_type":          state.DraftMeta.PostType,
			"audience":           state.DraftMeta.Audience,
			"core_claim":         state.DraftMeta.CoreClaim,
			"knowledge_base":     truncateRunes(state.KnowledgeBase, 12000),
			"must_include_block": mustIncludeBlock,
			"schema":             string(bpSchemaBytes),
		})
		if err != nil {
			return state, err
		}
		resp, err := llms.GenerateFromSinglePrompt(ctx, llmStructured, prompt)
		if err != nil {
			return state, err
		}
		var bpj blueprintJSON
		if err := json.Unmarshal([]byte(resp), &bpj); err != nil {
			return state, fmt.Errorf("blueprint JSON: %w", err)
		}
		if len(bpj.Sections) == 0 {
			return state, fmt.Errorf("blueprint produced no sections")
		}
		for i := range bpj.Sections {
			if bpj.Sections[i].ID == "" {
				bpj.Sections[i].ID = fmt.Sprintf("sec-%d", i)
			}
		}
		state.Blueprint = Blueprint{
			Title:        bpj.Title,
			Thesis:       bpj.Thesis,
			Audience:     bpj.Audience,
			PostType:     bpj.PostType,
			NarrativeArc: bpj.NarrativeArc,
			Sections:     bpj.Sections,
		}
		state.CurrentIndex = 0
		state.Sections = nil
		return state, nil
	}
}

func writeRichSectionNode(llm llms.Model) func(context.Context, State) (State, error) {
	return func(ctx context.Context, state State) (State, error) {
		if state.CurrentIndex < 0 || state.CurrentIndex >= len(state.Blueprint.Sections) {
			return state, fmt.Errorf("invalid CurrentIndex %d for %d sections",
				state.CurrentIndex, len(state.Blueprint.Sections))
		}
		spec := state.Blueprint.Sections[state.CurrentIndex]

		kb := relevantKB(state.KnowledgeBase, spec.SearchHints, spec.Title, 10000)
		prior := priorContext(state.Sections)
		voice := voiceOrDefault(state.DraftMeta)
		body := draftBody(state)

		prompt := buildSectionPrompt(spec, state.Blueprint, kb, prior, voice, body)

		resp, err := llms.GenerateFromSinglePrompt(ctx, llm, prompt)
		if err != nil {
			return state, err
		}

		md := strings.TrimSpace(resp)
		if md == "" {
			fmt.Fprintf(
				os.Stderr,
				"warning: write_rich_section produced empty markdown for %q\n",
				spec.Title,
			)
		}

		state.Sections = append(state.Sections, SectionDraft{
			ID:       spec.ID,
			Title:    spec.Title,
			Markdown: md,
		})
		state.CurrentIndex++
		return state, nil
	}
}

func sectionRouter(_ context.Context, state State) string {
	if state.CurrentIndex < len(state.Blueprint.Sections) {
		return "write_rich_section"
	}
	return "assemble"
}

func assembleNode() func(context.Context, State) (State, error) {
	return func(_ context.Context, state State) (State, error) {
		var b strings.Builder
		b.WriteString("# ")
		b.WriteString(state.Blueprint.Title)
		b.WriteString("\n\n")
		for _, sec := range state.Sections {
			b.WriteString(sec.Markdown)
			b.WriteString("\n\n")
		}
		state.FinalPost = strings.TrimSpace(b.String())
		return state, nil
	}
}

func voicePassNode(llm llms.Model) func(context.Context, State) (State, error) {
	return func(ctx context.Context, state State) (State, error) {
		voice := voiceOrDefault(state.DraftMeta)

		pt := prompts.NewPromptTemplate(promptVoicePass, []string{"voice", "post"})
		prompt, err := pt.Format(map[string]any{
			"voice": voice,
			"post":  state.FinalPost,
		})
		if err != nil {
			return state, err
		}
		resp, err := llms.GenerateFromSinglePrompt(ctx, llm, prompt)
		if err != nil {
			return state, err
		}
		state.FinalPost = strings.TrimSpace(resp)
		return state, nil
	}
}

func finalEditNode(llm llms.Model) func(context.Context, State) (State, error) {
	return func(ctx context.Context, state State) (State, error) {
		pt := prompts.NewPromptTemplate(promptFinalEdit, []string{"post", "knowledge_base"})
		kb := strings.TrimSpace(state.KnowledgeBase)
		if kb == "" {
			kb = "(no knowledge base — preserve existing links only; do not invent URLs)"
		} else {
			kb = truncateRunes(kb, 12000)
		}
		prompt, err := pt.Format(map[string]any{
			"post":           state.FinalPost,
			"knowledge_base": kb,
		})
		if err != nil {
			return state, err
		}
		resp, err := llms.GenerateFromSinglePrompt(ctx, llm, prompt)
		if err != nil {
			return state, err
		}
		state.FinalPost = strings.TrimSpace(resp)

		wc := CountWords(state.FinalPost)
		if wc < 1800 {
			fmt.Fprintf(os.Stderr, "warning: final post is %d words (target: 2000-3500)\n", wc)
		}
		return state, nil
	}
}

func saveNode() func(context.Context, State) (State, error) {
	return func(_ context.Context, state State) (State, error) {
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
		fmt.Printf("\n  saved: %s (%d words)\n", path, CountWords(state.FinalPost))
		return state, nil
	}
}

func draftBody(state State) string {
	if body := state.DraftMeta.FreeformBody; body != "" {
		return body
	}
	return state.Draft
}
