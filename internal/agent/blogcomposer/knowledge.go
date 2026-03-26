package blogcomposer

import (
	"fmt"
	"net/url"
	"strings"
)

func knowledgeBaseToMarkdown(kb knowledgeBaseJSON) string {
	var b strings.Builder
	if overview := strings.TrimSpace(kb.Overview); overview != "" {
		b.WriteString(overview)
	}
	if len(kb.EvidenceItems) > 0 {
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString("### Evidence\n\n")
		for _, e := range kb.EvidenceItems {
			fact := strings.TrimSpace(e.Fact)
			if fact == "" {
				continue
			}
			b.WriteString("- ")
			b.WriteString(fact)
			if urlStr := strings.TrimSpace(e.URL); urlStr != "" {
				fmt.Fprintf(&b, " [source](%s)", urlStr)
			}
			b.WriteString("\n")
		}
	}
	if len(kb.KeyResources) > 0 {
		if b.Len() > 0 {
			b.WriteString("\n")
		}
		b.WriteString("### Key Resources\n\n")
		for _, r := range kb.KeyResources {
			urlStr := strings.TrimSpace(r.URL)
			if urlStr == "" {
				continue
			}
			title := strings.TrimSpace(r.Title)
			if title == "" {
				title = urlStr
			}
			if note := strings.TrimSpace(r.Note); note != "" {
				fmt.Fprintf(&b, "- [%s](%s) — %s\n", title, urlStr, note)
			} else {
				fmt.Fprintf(&b, "- [%s](%s)\n", title, urlStr)
			}
		}
	}
	return strings.TrimSpace(b.String())
}

func verbatimSnippetsToMarkdown(snips []verbatimSnippetJSON) string {
	if len(snips) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("### Verbatim code from web search (copy exactly; do not invent APIs)\n\n")
	for i, s := range snips {
		lang := strings.TrimSpace(s.Language)
		if lang == "" {
			lang = "text"
		}
		urlStr := strings.TrimSpace(s.SourceURL)
		if urlStr == "" {
			urlStr = "(URL not parsed from search hit — attribute in prose with the closest Key Resource link from the Knowledge base)"
		}
		fmt.Fprintf(&b, "#### Snippet %d (%s) — %s\n\n", i+1, lang, urlStr)
		if note := strings.TrimSpace(s.Note); note != "" {
			b.WriteString(note)
			b.WriteString("\n\n")
		}
		b.WriteString("```")
		b.WriteString(lang)
		b.WriteString("\n")
		b.WriteString(strings.TrimSpace(s.Snippet))
		b.WriteString("\n```\n\n")
	}
	return strings.TrimSpace(b.String())
}

// relevantKB trims the knowledge base to maxRunes. When the KB has a raw-search preamble before "---",
// the preamble is kept in full (or head-only if huge) and only the synthesized tail is truncated.
func relevantKB(kb string, maxRunes int) string {
	const sep = "\n\n---\n\n"
	if strings.HasPrefix(strings.TrimSpace(kb), "### Raw web search") && strings.Contains(kb, sep) {
		parts := strings.SplitN(kb, sep, 2)
		if len(parts) == 2 {
			head, body := parts[0], parts[1]
			headN := len([]rune(head))
			sepN := len([]rune(sep))
			bodyN := len([]rune(body))
			if headN+sepN+bodyN <= maxRunes {
				return head + sep + body
			}
			if headN >= maxRunes {
				return truncateRunes(head, maxRunes)
			}
			remain := maxRunes - headN - sepN
			if remain <= 0 {
				return truncateRunes(head, maxRunes)
			}
			return head + sep + truncateRunes(body, remain)
		}
	}
	if len([]rune(kb)) <= maxRunes {
		return kb
	}
	return truncateRunes(kb, maxRunes)
}

func priorContext(sections []SectionDraft) string {
	if len(sections) == 0 {
		return ""
	}
	var titles []string
	for _, s := range sections {
		titles = append(titles, s.Title)
	}
	last := sections[len(sections)-1]
	tail := truncateRunes(last.Markdown, 400)
	return fmt.Sprintf("Prior sections: %s\n\nEnd of previous section:\n%s",
		strings.Join(titles, ", "), tail)
}

func knowledgeSupplementFromDrafting(state State) string {
	var b strings.Builder
	b.WriteString(state.SessionToolCorpus)
	b.WriteByte('\n')
	b.WriteString(state.FinalPost)
	urls := uniqueHTTPSURLs(b.String(), 48)
	if len(urls) == 0 {
		return ""
	}
	var m strings.Builder
	m.WriteString("### URLs seen during drafting (use only these when adding or fixing citations)\n\n")
	for _, u := range urls {
		m.WriteString("- ")
		m.WriteString(u)
		m.WriteByte('\n')
	}
	return m.String()
}

func buildChunkPublishContext(state State, sectionIndex int, priorTailMaxRunes int) string {
	var b strings.Builder
	fmt.Fprintf(&b,
		"Article title: %s\nThesis: %s\nPost type: %s\n",
		state.Blueprint.Title,
		state.Blueprint.Thesis,
		state.Blueprint.PostType,
	)
	if ar := strings.TrimSpace(state.Blueprint.NarrativeArc); ar != "" {
		b.WriteString("Narrative arc (this section should feel like one beat in this arc, not a disconnected essay):\n")
		b.WriteString(ar)
		b.WriteString("\n\n")
	}
	b.WriteString("Outline — you ONLY rewrite the section marked →; respect neighbors' roles:\n")
	for j, spec := range state.Blueprint.Sections {
		marker := "   "
		if j == sectionIndex {
			marker = "→  "
		}
		fmt.Fprintf(&b, "%s%d. [%s] %s\n", marker, j+1, spec.Type, spec.Title)
	}
	b.WriteString("\n")
	if sectionIndex > 0 && sectionIndex <= len(state.Sections) {
		tail := strings.TrimSpace(state.Sections[sectionIndex-1].Markdown)
		if tail != "" && priorTailMaxRunes > 0 {
			b.WriteString(
				"Tail of the previous section (already polished). Let the opening of THIS section bridge forward:\n" +
					"avoid repeating the same metaphor, rhetorical question, or \"What surprised me was\" pattern if it appears below.\n",
			)
			b.WriteString(truncateRunes(tail, priorTailMaxRunes))
			b.WriteString("\n\n")
		}
	}
	if sectionIndex+1 < len(state.Blueprint.Sections) {
		nx := state.Blueprint.Sections[sectionIndex+1]
		fmt.Fprintf(&b,
			"Next section (do not summarize it; don't steal its conclusion): [%s] %s\n\n",
			nx.Type, nx.Title,
		)
	}
	fmt.Fprintf(&b,
		"Polish ONLY section %d of %d. The first line of your output MUST equal the first line of the fragment (the ## heading).\n",
		sectionIndex+1, len(state.Blueprint.Sections),
	)
	return b.String()
}

func isGitHubFileFetchURL(raw string) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme != "https" {
		return false
	}
	host := strings.ToLower(u.Hostname())
	if host == "raw.githubusercontent.com" {
		return true
	}
	if host != "github.com" {
		return false
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	return len(parts) >= 4 && parts[2] == "blob"
}

func buildPrefetchContextForSection(spec SectionSpec, sources []PrefetchedSource, maxRunes int) string {
	if len(sources) == 0 || len(spec.CiteURLs) == 0 {
		return ""
	}
	want := make(map[string]struct{})
	for _, u := range spec.CiteURLs {
		u = strings.TrimSpace(u)
		if u != "" {
			want[u] = struct{}{}
		}
	}
	var b strings.Builder
	for _, ps := range sources {
		if _, ok := want[strings.TrimSpace(ps.URL)]; !ok {
			continue
		}
		b.WriteString("### Full file text (verbatim source for ``` blocks — **Code source:** must use this exact URL)\n")
		b.WriteString(ps.URL)
		b.WriteString("\n\n")
		b.WriteString(strings.TrimSpace(ps.Body))
		b.WriteString("\n\n")
	}
	s := strings.TrimSpace(b.String())
	if s == "" {
		return ""
	}
	return truncateRunes(s, maxRunes)
}

func appendSessionToolCorpus(existing string, spec SectionSpec, toolText string, maxRunes int) string {
	toolText = strings.TrimSpace(toolText)
	if toolText == "" {
		return existing
	}
	block := fmt.Sprintf("### section-tools id=%s title=%q\n\n%s", spec.ID, spec.Title, toolText)
	s := strings.TrimSpace(existing)
	if s == "" {
		s = block
	} else {
		s = s + "\n\n" + block
	}
	r := []rune(s)
	if len(r) <= maxRunes {
		return s
	}
	return string(r[len(r)-maxRunes:])
}
