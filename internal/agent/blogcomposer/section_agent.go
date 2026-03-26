package blogcomposer

import (
	"context"
	"fmt"
	"strings"

	"github.com/smallnest/langgraphgo/prebuilt"
	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/tools"
)

func runSectionWritingAgent(
	ctx context.Context,
	llm llms.Model,
	toolList []tools.Tool,
	spec SectionSpec,
	state State,
	userSuffix string,
	o Options,
) (finalMarkdown string, toolTranscript string, err error) {
	if len(toolList) == 0 {
		return "", "", fmt.Errorf("section agent: no tools")
	}
	runnable, err := prebuilt.CreateAgentMap(llm, toolList, o.SectionAgentMaxIter,
		prebuilt.WithSystemMessage(promptSectionAgentSystem),
	)
	if err != nil {
		return "", "", err
	}
	baseKB := relevantKB(state.KnowledgeBase, o.SectionKBMaxRunes)
	pc := buildPrefetchContextForSection(spec, state.PrefetchedSources, o.SectionPrefetchMax)
	kb := baseKB
	if pc != "" {
		kb = pc + "\n\n" + baseKB
	}
	verbatim := strings.TrimSpace(state.KnowledgeVerbatim)
	if verbatim != "" {
		verbatim = truncateRunes(verbatim, o.SectionVerbatimMaxRunes)
	}

	userText := buildSectionAgentUserContent(
		spec, state.Blueprint, kb, verbatim,
		priorContext(state.Sections), voiceOrDefault(state.DraftMeta), draftBody(state),
	)
	if userSuffix != "" {
		userText += "\n\n---\n\n" + userSuffix
	}

	initial := map[string]any{
		"messages": []llms.MessageContent{
			llms.TextParts(llms.ChatMessageTypeHuman, userText),
		},
	}

	resp, err := runnable.Invoke(ctx, initial)
	if err != nil {
		return "", "", err
	}

	msgs, _ := resp["messages"].([]llms.MessageContent)
	toolTranscript = searchToolCorpus(msgs)
	md := strings.TrimSpace(finalAIMessageText(msgs))
	if md == "" {
		return "", toolTranscript, fmt.Errorf("section agent produced empty markdown for %q", spec.Title)
	}

	iter := 0
	if ic, ok := resp["iteration_count"].(int); ok {
		iter = ic
	}
	if sectionDebugEnabled() {
		corpus := buildDebugVerificationCorpus(msgs, kb, verbatim)
		logSectionAgentTrace(spec.Title, msgs, kb, verbatim, corpus, md, iter, spec.Artifacts)
	}
	return md, toolTranscript, nil
}

func searchToolCorpus(msgs []llms.MessageContent) string {
	var b strings.Builder
	for _, msg := range msgs {
		if msg.Role != llms.ChatMessageTypeTool {
			continue
		}
		for _, part := range msg.Parts {
			if p, ok := part.(llms.ToolCallResponse); ok {
				b.WriteString(p.Content)
				b.WriteByte('\n')
			}
		}
	}
	return b.String()
}

func finalAIMessageText(msgs []llms.MessageContent) string {
	for i := len(msgs) - 1; i >= 0; i-- {
		msg := msgs[i]
		if msg.Role != llms.ChatMessageTypeAI {
			continue
		}
		hasTool := false
		var tb strings.Builder
		for _, part := range msg.Parts {
			switch p := part.(type) {
			case llms.ToolCall:
				hasTool = true
			case llms.TextContent:
				tb.WriteString(p.Text)
			}
		}
		if hasTool {
			continue
		}
		s := strings.TrimSpace(tb.String())
		if s != "" {
			return s
		}
	}
	return ""
}
