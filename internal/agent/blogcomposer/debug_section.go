// Debug logging (stderr) for section agent:
//
//	BLOG_COMPOSER_DEBUG_SECTION=1|true — section traces and optional corpus digest.
//	BLOG_COMPOSER_DEBUG_SECTION=verbose|2|all|deep — also tool search text HEAD/TAIL and full verification corpus HEAD/TAIL
//	  (KB + verbatim + tool responses; for human inspection only).
package blogcomposer

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/tmc/langchaingo/llms"
)

func sectionDebugEnabled() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("BLOG_COMPOSER_DEBUG_SECTION")))
	switch v {
	case "", "0", "false", "no", "off":
		return false
	default:
		return true
	}
}

// sectionDebugVerbose enables long corpus samples (env: 2, verbose, all, deep).
func sectionDebugVerbose() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("BLOG_COMPOSER_DEBUG_SECTION")))
	switch v {
	case "2", "verbose", "all", "deep":
		return true
	default:
		return false
	}
}

// buildDebugVerificationCorpus joins tool transcripts + KB + verbatim for stderr diagnostics only.
func buildDebugVerificationCorpus(msgs []llms.MessageContent, kb, verbatim string) string {
	var b strings.Builder
	b.WriteString(searchToolCorpus(msgs))
	if kb != "" {
		b.WriteString("\n\n")
		b.WriteString(kb)
	}
	if verbatim != "" {
		b.WriteString("\n\n")
		b.WriteString(verbatim)
	}
	return b.String()
}

func logVerificationCorpusDigest(label, corpus string) {
	if !sectionDebugEnabled() {
		return
	}
	corpus = strings.TrimSpace(corpus)
	nr := len([]rune(corpus))
	fmt.Fprintf(os.Stderr, "[blogcomposer:corpus] label=%q runes=%d bytes=%d markdown_fences=%d non_mermaid_inners=%d\n",
		label, nr, len(corpus), countTripleBacktickFences(corpus), nonMermaidCodeFenceCount(corpus))
	if !sectionDebugVerbose() || corpus == "" {
		return
	}
	const maxChunk = 1800
	fmt.Fprintf(os.Stderr, "[blogcomposer:corpus] label=%q HEAD (%d runes max):\n%s\n",
		label, maxChunk, previewRunes(corpus, maxChunk))
	tail := tailRunes(corpus, maxChunk)
	if tail != "" && tail != previewRunes(corpus, maxChunk) {
		fmt.Fprintf(os.Stderr, "[blogcomposer:corpus] label=%q TAIL (%d runes max):\n%s\n",
			label, maxChunk, tail)
	}
}

func logToolSearchTextSample(label string, toolCorpus string) {
	if !sectionDebugVerbose() {
		return
	}
	toolCorpus = strings.TrimSpace(toolCorpus)
	if toolCorpus == "" {
		fmt.Fprintf(os.Stderr, "[blogcomposer:tool-corpus] label=%q (empty)\n", label)
		return
	}
	const maxChunk = 2200
	fmt.Fprintf(os.Stderr, "[blogcomposer:tool-corpus] label=%q bytes=%d runes=%d triple_backtick_sequences=%d\n",
		label, len(toolCorpus), len([]rune(toolCorpus)), strings.Count(toolCorpus, "```"))
	fmt.Fprintf(os.Stderr, "%s\n",
		previewRunes(toolCorpus, maxChunk))
	if len([]rune(toolCorpus)) > maxChunk {
		fmt.Fprintf(os.Stderr, "[blogcomposer:tool-corpus] label=%q TAIL:\n%s\n", label, tailRunes(toolCorpus, maxChunk))
	}
}

func previewRunes(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "\n… [truncated]"
}

func tailRunes(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return "… [truncated]\n" + string(r[len(r)-max:])
}

func countTripleBacktickFences(s string) int {
	return strings.Count(s, "```") / 2
}

func logSectionAgentTrace(
	title string,
	msgs []llms.MessageContent,
	kb, verbatim, corpus, rawMD string,
	iterationCount int,
	artifacts []string,
) {
	toolParts := countToolResponses(msgs)
	toolCorpus := searchToolCorpus(msgs)
	fmt.Fprintf(os.Stderr,
		"[blogcomposer:section-debug] %q iter=%d artifacts=%v tool_msgs=%d tool_corpus_bytes=%d kb_runes=%d verbatim_runes=%d full_corpus_runes=%d raw_md_runes=%d raw_fences=%s\n",
		title,
		iterationCount,
		artifacts,
		toolParts,
		len(toolCorpus),
		len([]rune(kb)),
		len([]rune(verbatim)),
		len([]rune(corpus)),
		len([]rune(rawMD)),
		summarizeFenceLangs(rawMD),
	)
	if toolParts == 0 {
		fmt.Fprintf(os.Stderr,
			"[blogcomposer:section-debug] %q WARN: no tool response messages — if the model used search, tool extraction may be broken; "+
				"draft relies on KB/verbatim in prompt only.\n",
			title,
		)
	}
	logToolSearchTextSample("section:"+title, toolCorpus)
	if sectionDebugVerbose() {
		logVerificationCorpusDigest("section:"+title+" full_corpus", corpus)
	}
}

func logSectionAfterDraft(title, md string) {
	fmt.Fprintf(os.Stderr,
		"[blogcomposer:section-debug] %q drafted fences=%s runes=%d\n",
		title,
		summarizeFenceLangs(md),
		len([]rune(md)),
	)
}

func countToolResponses(msgs []llms.MessageContent) int {
	n := 0
	for _, msg := range msgs {
		if msg.Role != llms.ChatMessageTypeTool {
			continue
		}
		for _, part := range msg.Parts {
			if _, ok := part.(llms.ToolCallResponse); ok {
				n++
			}
		}
	}
	return n
}

func summarizeFenceLangs(md string) string {
	counts := map[string]int{}
	i := 0
	for i < len(md) {
		idx := strings.Index(md[i:], "```")
		if idx < 0 {
			break
		}
		absStart := i + idx
		rest := md[absStart+3:]
		nl := strings.IndexByte(rest, '\n')
		if nl < 0 {
			break
		}
		info := strings.TrimSpace(rest[:nl])
		lang := "text"
		if f := strings.Fields(info); len(f) > 0 {
			lang = strings.ToLower(f[0])
		}
		bodyStart := absStart + 3 + nl + 1
		closeRel := strings.Index(md[bodyStart:], "```")
		if closeRel < 0 {
			break
		}
		counts[lang]++
		i = bodyStart + closeRel + 3
	}
	if len(counts) == 0 {
		return "(none)"
	}
	keys := make([]string, 0, len(counts))
	for k := range counts {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var parts []string
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s:%d", k, counts[k]))
	}
	return strings.Join(parts, ",")
}
