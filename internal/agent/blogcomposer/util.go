package blogcomposer

import (
	"regexp"
	"strings"
)

// AssembleMarkdown joins the H1 title and section bodies into one markdown document.
func AssembleMarkdown(title string, sections []SectionDraft) string {
	var b strings.Builder
	b.WriteString("# ")
	b.WriteString(title)
	b.WriteString("\n\n")
	for _, sec := range sections {
		b.WriteString(sec.Markdown)
		b.WriteString("\n\n")
	}
	return strings.TrimSpace(b.String())
}

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

func draftBody(state State) string {
	if body := state.DraftMeta.FreeformBody; body != "" {
		return body
	}
	return state.Draft
}

func voiceOrDefault(meta DraftMeta) string {
	if v := strings.TrimSpace(meta.Voice); v != "" {
		return v
	}
	return "experienced engineer on personal blog — direct, occasionally wry, doesn't oversell"
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

func CountWords(s string) int {
	return len(strings.Fields(s))
}

var httpsURLRegexp = regexp.MustCompile(`https://[^\s\)<>"']+`)

func trimTrailingURLPunct(u string) string {
	return strings.TrimRight(u, ".,;:!?*)]\"'")
}

func uniqueHTTPSURLs(s string, max int) []string {
	if max <= 0 {
		return nil
	}
	raw := httpsURLRegexp.FindAllString(s, -1)
	seen := make(map[string]struct{})
	var out []string
	for _, u := range raw {
		u = trimTrailingURLPunct(u)
		if u == "" {
			continue
		}
		if _, ok := seen[u]; ok {
			continue
		}
		seen[u] = struct{}{}
		out = append(out, u)
		if len(out) >= max {
			break
		}
	}
	return out
}

// nonMermaidCodeFenceCount counts ``` blocks whose info line does not start with mermaid.
func nonMermaidCodeFenceCount(md string) int {
	n := 0
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
		bodyStart := absStart + 3 + nl + 1
		closeRel := strings.Index(md[bodyStart:], "```")
		if closeRel < 0 {
			break
		}
		lang0 := ""
		if f := strings.Fields(info); len(f) > 0 {
			lang0 = strings.ToLower(f[0])
		}
		if !strings.HasPrefix(lang0, "mermaid") {
			n++
		}
		i = bodyStart + closeRel + 3
	}
	return n
}
