package blogcomposer

import (
	"fmt"
	"strings"
)

func parseDraftMeta(raw string) DraftMeta {
	raw = strings.TrimSpace(raw)
	parts := strings.SplitN(raw, "\n---", 2)
	if len(parts) < 2 {
		return DraftMeta{FreeformBody: raw}
	}

	frontmatter := strings.TrimSpace(parts[0])
	body := strings.TrimSpace(parts[1])
	frontmatter = strings.TrimPrefix(frontmatter, "---")
	frontmatter = strings.TrimSpace(frontmatter)

	meta := DraftMeta{FreeformBody: body}
	var currentKey string
	var mustInclude []string

	for _, line := range strings.Split(frontmatter, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "- ") {
			val := strings.TrimSpace(strings.TrimPrefix(trimmed, "- "))
			val = strings.Trim(val, `"'`)
			if currentKey == "must_include" {
				mustInclude = append(mustInclude, val)
			}
			continue
		}
		if idx := strings.Index(trimmed, ":"); idx > 0 {
			key := strings.TrimSpace(trimmed[:idx])
			val := strings.TrimSpace(trimmed[idx+1:])
			val = strings.Trim(val, `"'`)
			currentKey = key
			switch key {
			case "voice":
				meta.Voice = val
			case "style":
				meta.Style = val
			case "must_include":
				if val != "" {
					mustInclude = append(mustInclude, val)
				}
			}
		}
	}
	meta.MustInclude = mustInclude
	return meta
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

func normalizeQueries(qs []string, maxCount int) []string {
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
		if len(out) >= maxCount {
			break
		}
	}
	return out
}

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
			if url := strings.TrimSpace(e.URL); url != "" {
				b.WriteString(fmt.Sprintf(" [source](%s)", url))
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

func relevantKB(kb string, hints []string, sectionTitle string, maxRunes int) string {
	if len([]rune(kb)) <= maxRunes {
		return kb
	}
	var tokens []string
	for _, h := range hints {
		for _, w := range strings.Fields(h) {
			if len(w) > 2 {
				tokens = append(tokens, strings.ToLower(w))
			}
		}
	}
	for _, w := range strings.Fields(sectionTitle) {
		if len(w) > 2 {
			tokens = append(tokens, strings.ToLower(w))
		}
	}

	lines := strings.Split(kb, "\n")
	var scored, rest []string
	for _, line := range lines {
		lower := strings.ToLower(line)
		match := false
		for _, t := range tokens {
			if strings.Contains(lower, t) {
				match = true
				break
			}
		}
		if match {
			scored = append(scored, line)
		} else {
			rest = append(rest, line)
		}
	}
	combined := append(scored, rest...)
	result := strings.Join(combined, "\n")
	return truncateRunes(result, maxRunes)
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

// CountWords returns the whitespace-split word count of s.
func CountWords(s string) int {
	return len(strings.Fields(s))
}

func voiceOrDefault(meta DraftMeta) string {
	if v := strings.TrimSpace(meta.Voice); v != "" {
		return v
	}
	return "experienced engineer on personal blog — direct, occasionally wry, doesn't oversell"
}
