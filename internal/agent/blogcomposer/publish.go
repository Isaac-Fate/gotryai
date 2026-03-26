package blogcomposer

import (
	"context"
	"fmt"
	"os"
	"strings"
	"unicode/utf8"

	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/prompts"
)

func publishPolishNode(llm llms.Model, o Options) func(context.Context, State) (State, error) {
	return func(ctx context.Context, state State) (State, error) {
		n := utf8.RuneCountInString(state.FinalPost)
		if n >= o.PublishChunkMinRunes && len(state.Sections) > 1 {
			fmt.Fprintf(os.Stderr, "publish_polish: using per-section mode (%d runes >= %d) to limit output truncation\n",
				n, o.PublishChunkMinRunes)
			return publishPolishChunked(ctx, llm, state, o)
		}
		return publishPolishMonolith(ctx, llm, state, o)
	}
}

func buildPublishKnowledgePack(state State, o Options) (kb string, verbatim string) {
	kb = strings.TrimSpace(state.KnowledgeBase)
	supp := strings.TrimSpace(knowledgeSupplementFromDrafting(state))
	switch {
	case kb != "" && supp != "":
		kb = kb + "\n\n" + supp
	case kb != "":
	case supp != "":
		kb = supp
	default:
		kb = ""
	}
	if kb == "" {
		kb = "(no knowledge base — preserve existing links only; do not invent URLs)"
	} else {
		kb = truncateRunes(kb, o.FinalEditKBMaxRunes)
	}
	verbatim = strings.TrimSpace(state.KnowledgeVerbatim)
	if verbatim == "" {
		verbatim = "(none)"
	} else {
		verbatim = truncateRunes(verbatim, o.FinalEditVerbatimMax)
	}
	return kb, verbatim
}

func publishPolishMonolith(ctx context.Context, llm llms.Model, state State, o Options) (State, error) {
	pt := prompts.NewPromptTemplate(promptPublishPolish,
		[]string{"voice", "post", "knowledge_base", "verbatim_snippets"})
	kb, verbatim := buildPublishKnowledgePack(state, o)
	prompt, err := pt.Format(map[string]any{
		"voice":             voiceOrDefault(state.DraftMeta),
		"post":              state.FinalPost,
		"knowledge_base":    kb,
		"verbatim_snippets": verbatim,
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

func publishPolishChunked(ctx context.Context, llm llms.Model, state State, o Options) (State, error) {
	kb, verbatim := buildPublishKnowledgePack(state, o)
	pt := prompts.NewPromptTemplate(promptPublishPolishSection,
		[]string{"voice", "section_context", "section_body", "knowledge_base", "verbatim_snippets"})
	voice := voiceOrDefault(state.DraftMeta)
	for i := range state.Sections {
		ctxBlock := buildChunkPublishContext(state, i, o.ChunkPriorTailRunes)
		prompt, err := pt.Format(map[string]any{
			"voice":             voice,
			"section_context":   ctxBlock,
			"section_body":      state.Sections[i].Markdown,
			"knowledge_base":    kb,
			"verbatim_snippets": verbatim,
		})
		if err != nil {
			return state, err
		}
		resp, err := llms.GenerateFromSinglePrompt(ctx, llm, prompt)
		if err != nil {
			return state, err
		}
		state.Sections[i].Markdown = strings.TrimSpace(resp)
	}
	state.FinalPost = AssembleMarkdown(state.Blueprint.Title, state.Sections)

	wc := CountWords(state.FinalPost)
	if wc < 1800 {
		fmt.Fprintf(os.Stderr, "warning: final post is %d words (target: 2000-3500)\n", wc)
	}
	return state, nil
}
