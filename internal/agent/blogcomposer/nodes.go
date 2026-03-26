package blogcomposer

import (
	"context"
	"encoding/json"
	"fmt"
	"gotryai/pkg/structured"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/prompts"
	"github.com/tmc/langchaingo/tools"
)

func analyzeDraftNode(llmStructured llms.Model) func(context.Context, State) (State, error) {
	return func(ctx context.Context, state State) (State, error) {
		if strings.TrimSpace(state.RunID) == "" {
			state.RunID = fmt.Sprintf("run_%d", time.Now().UnixNano())
		}
		state.DraftMeta = parseDraftMeta(state.Draft)

		schema := structured.GetSchemaString(&draftAnalysisJSON{})

		body := draftBody(state)
		pt := prompts.NewPromptTemplate(promptAnalyzeDraft, []string{"draft", "schema"})
		prompt, err := pt.Format(map[string]any{
			"draft":  body,
			"schema": schema,
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
	o Options,
) func(context.Context, State) (State, error) {
	return func(ctx context.Context, state State) (State, error) {
		qSchema := structured.GetSchemaString(&searchQueriesJSON{})

		body := draftBody(state)
		qPt := prompts.NewPromptTemplate(promptResearchQueries,
			[]string{"draft", "post_type", "core_claim", "schema"})
		qPrompt, err := qPt.Format(map[string]any{
			"draft":      body,
			"post_type":  state.DraftMeta.PostType,
			"core_claim": state.DraftMeta.CoreClaim,
			"schema":     qSchema,
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
		queries := normalizeQueries(sq.Queries, 7)
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
			bundles = append(bundles, fmt.Sprintf("### Query: %s\n%s", q, truncateRunes(raw, o.ResearchHitMaxRunes)))
		}
		combined := strings.Join(bundles, "\n\n")
		if combined == "" {
			state.KnowledgeBase = "(no search results available)"
			return state, nil
		}

		kbSchema := structured.GetSchemaString(&knowledgeBaseJSON{})

		synthPt := prompts.NewPromptTemplate(promptResearchSynthesize,
			[]string{"draft", "search", "schema"})
		synthSearch := truncateRunes(combined, o.ResearchSynthMaxRunes)
		synthPrompt, err := synthPt.Format(map[string]any{
			"draft":  body,
			"search": synthSearch,
			"schema": kbSchema,
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

		state.KnowledgeVerbatim = verbatimSnippetsToMarkdown(kb.VerbatimSnippets)
		kbMD := knowledgeBaseToMarkdown(kb)
		if strings.TrimSpace(combined) != "" {
			// Front-load raw tool output so section-scoped KB truncation still often retains real ``` fences
			// (e.g. DuckDuckGo WithDuckMarkdown). Writers copy from here; no separate fence parser.
			kbMD = "### Raw web search (verbatim markdown from the search tool)\n\n" +
				truncateRunes(combined, o.RawSearchCapRunes) + "\n\n---\n\n" + kbMD
		}
		state.KnowledgeBase = kbMD
		return state, nil
	}
}

func designBlueprintNode(llmStructured llms.Model, o Options) func(context.Context, State) (State, error) {
	return func(ctx context.Context, state State) (State, error) {
		bpSchema := structured.GetSchemaString(&blueprintJSON{})

		body := draftBody(state)
		mustIncludeBlock := ""
		if len(state.DraftMeta.MustInclude) > 0 {
			mustIncludeBlock = "MUST INCLUDE (verbatim in the final post):\n"
			for _, item := range state.DraftMeta.MustInclude {
				mustIncludeBlock += "- " + item + "\n"
			}
		}

		verbatimBlock := strings.TrimSpace(state.KnowledgeVerbatim)
		if verbatimBlock == "" {
			verbatimBlock = "(none — if this stays empty, section agents pull verbatim listings only from live search + Fetch_Page_Text.)"
		} else {
			verbatimBlock = truncateRunes(verbatimBlock, o.DesignVerbatimMaxRunes)
		}

		kbForBlueprint := strings.TrimSpace(state.KnowledgeBase)
		if kbForBlueprint == "" {
			kbForBlueprint = "(none — OUTLINE-ONLY MODE: see LINK CITATION PLAN in the prompt. No batch web research ran; each section will search while drafting.)"
		} else {
			kbForBlueprint = truncateRunes(kbForBlueprint, o.BlueprintKBMaxRunes)
		}

		pt := prompts.NewPromptTemplate(
			promptDesignBlueprint,
			[]string{
				"draft",
				"post_type",
				"audience",
				"core_claim",
				"knowledge_base",
				"verbatim_snippets",
				"must_include_block",
				"schema",
			},
		)
		prompt, err := pt.Format(map[string]any{
			"draft":              body,
			"post_type":          state.DraftMeta.PostType,
			"audience":           state.DraftMeta.Audience,
			"core_claim":         state.DraftMeta.CoreClaim,
			"knowledge_base":     kbForBlueprint,
			"verbatim_snippets":  verbatimBlock,
			"must_include_block": mustIncludeBlock,
			"schema":             bpSchema,
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
		state.SessionToolCorpus = ""
		state.PrefetchedSources = nil
		return state, nil
	}
}

func prefetchBlueprintSourcesNode(fetch tools.Tool) func(context.Context, State) (State, error) {
	return func(ctx context.Context, state State) (State, error) {
		state.PrefetchedSources = nil
		if fetch == nil || len(state.Blueprint.Sections) == 0 {
			return state, nil
		}
		seen := make(map[string]struct{})
		for _, sec := range state.Blueprint.Sections {
			for _, u := range sec.CiteURLs {
				u = strings.TrimSpace(u)
				if u == "" {
					continue
				}
				if _, dup := seen[u]; dup {
					continue
				}
				seen[u] = struct{}{}
				if !isGitHubFileFetchURL(u) {
					continue
				}
				body, err := fetch.Call(ctx, u)
				if err != nil {
					fmt.Fprintf(os.Stderr, "warning: prefetch_sources skip %q: %v\n", u, err)
					continue
				}
				body = strings.TrimSpace(body)
				if body == "" || body == "(empty body)" {
					continue
				}
				state.PrefetchedSources = append(state.PrefetchedSources, PrefetchedSource{
					URL:  u,
					Body: body,
				})
			}
		}
		if n := len(state.PrefetchedSources); n > 0 {
			fmt.Fprintf(os.Stderr, "prefetch_sources: loaded %d GitHub file(s) for drafting\n", n)
		}
		return state, nil
	}
}

func writeRichSectionNode(
	llm llms.Model,
	sectionTools []tools.Tool,
	o Options,
) func(context.Context, State) (State, error) {
	return func(ctx context.Context, state State) (State, error) {
		if state.CurrentIndex < 0 || state.CurrentIndex >= len(state.Blueprint.Sections) {
			return state, fmt.Errorf("invalid CurrentIndex %d for %d sections",
				state.CurrentIndex, len(state.Blueprint.Sections))
		}
		if strings.TrimSpace(state.RunID) == "" {
			state.RunID = fmt.Sprintf("run_%d", time.Now().UnixNano())
		}
		spec := state.Blueprint.Sections[state.CurrentIndex]
		sectionIdx := state.CurrentIndex

		md, toolText, err := runSectionWritingAgent(ctx, llm, sectionTools, spec, state, "", o)
		if err != nil {
			return state, fmt.Errorf("draft_section: %w", err)
		}
		md = strings.TrimSpace(md)
		if o.SectionThinRetry && utf8.RuneCountInString(md) < o.SectionMinBodyRunes {
			fmt.Fprintf(os.Stderr, "warning: section %q body short (%d runes vs min %d); retrying once\n",
				spec.Title, utf8.RuneCountInString(md), o.SectionMinBodyRunes)
			md2, tool2, err2 := runSectionWritingAgent(ctx, llm, sectionTools, spec, state, promptSectionThinRetry, o)
			if err2 == nil && strings.TrimSpace(md2) != "" {
				md2 = strings.TrimSpace(md2)
				if utf8.RuneCountInString(md2) > utf8.RuneCountInString(md) {
					md, toolText = md2, tool2
				}
			}
		}
		if o.SectionMissingCodeRetry && sectionRequiresCodeArtifact(spec) &&
			nonMermaidCodeFenceCount(md) == 0 {
			fmt.Fprintf(os.Stderr, "warning: section %q expected code-* artifact but has no non-mermaid code fence; retrying once\n",
				spec.Title)
			md3, tool3, err3 := runSectionWritingAgent(ctx, llm, sectionTools, spec, state, promptSectionMissingCodeRetry, o)
			if err3 == nil && strings.TrimSpace(md3) != "" {
				md3 = strings.TrimSpace(md3)
				if nonMermaidCodeFenceCount(md3) > nonMermaidCodeFenceCount(md) {
					md, toolText = md3, tool3
				}
			}
		}
		if sectionDebugEnabled() {
			logSectionAfterDraft(spec.Title, md)
		}
		if md == "" {
			fmt.Fprintf(
				os.Stderr,
				"warning: draft_section produced empty markdown for %q\n",
				spec.Title,
			)
		}

		state.SessionToolCorpus = appendSessionToolCorpus(state.SessionToolCorpus, spec, toolText, o.SessionToolCorpusMax)
		state.Sections = append(state.Sections, SectionDraft{
			ID:       spec.ID,
			Title:    spec.Title,
			Markdown: md,
		})
		if store := o.SectionDraftStore; store != nil {
			if err := store.PersistSectionDraft(ctx, state.RunID, sectionIdx, spec, md); err != nil {
				fmt.Fprintf(os.Stderr, "warning: section draft persist idx=%d: %v\n", sectionIdx, err)
			}
		}
		state.CurrentIndex++
		return state, nil
	}
}

func sectionRouter(_ context.Context, state State) string {
	if state.CurrentIndex < len(state.Blueprint.Sections) {
		return "draft_section"
	}
	return "assemble_post"
}

func assembleNode() func(context.Context, State) (State, error) {
	return func(_ context.Context, state State) (State, error) {
		state.FinalPost = AssembleMarkdown(state.Blueprint.Title, state.Sections)
		return state, nil
	}
}

func saveNode(o Options) func(context.Context, State) (State, error) {
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
		if rid := strings.TrimSpace(state.RunID); rid != "" && o.SectionDraftStore != nil {
			fmt.Printf("  run_id (section store): %s\n", rid)
		}
		return state, nil
	}
}

func sectionRequiresCodeArtifact(spec SectionSpec) bool {
	for _, a := range spec.Artifacts {
		if strings.HasPrefix(a, "code-") {
			return true
		}
	}
	return false
}
