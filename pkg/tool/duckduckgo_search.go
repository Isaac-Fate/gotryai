package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/JohannesKaufmann/html-to-markdown/v2/converter"
	"github.com/JohannesKaufmann/html-to-markdown/v2/plugin/base"
	"github.com/JohannesKaufmann/html-to-markdown/v2/plugin/commonmark"
	"github.com/PuerkitoBio/goquery"
)

var (
	htmlSnippetConverter     *converter.Converter
	htmlSnippetConverterOnce sync.Once
)

func snippetHTMLToMarkdown(snippetHTML string) string {
	snippetHTML = strings.TrimSpace(snippetHTML)
	if snippetHTML == "" {
		return ""
	}
	htmlSnippetConverterOnce.Do(func() {
		htmlSnippetConverter = converter.NewConverter(
			converter.WithPlugins(
				base.NewBasePlugin(),
				commonmark.NewCommonmarkPlugin(),
			),
		)
	})
	out, err := htmlSnippetConverter.ConvertString("<div>" + snippetHTML + "</div>")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

// Browser-like UA reduces DuckDuckGo “anomaly” bot interstitials compared to Go’s default client string.
const ddgChromeUserAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/122.0.0.0 Safari/537.36"

const ddgHTMLSearchURL = "https://html.duckduckgo.com/html/"
const ddgJSONAPIURL = "https://api.duckduckgo.com/"

// DuckDuckGoSearch implements langchaingo/tools.Tool like langgraphgo’s BraveSearch and BochaSearch:
// Name, Description, and Call(ctx, input string). No API key required.
type DuckDuckGoSearch struct {
	Count int
	// Markdown, when true, formats Call() output as Markdown: headings, [title](url) links,
	// HTML snippets converted via html-to-markdown, and multi-line code-like snippets fenced
	// so downstream tools can extract verbatim listings from search text.
	Markdown bool
	Client   *http.Client
}

type DuckOption func(*DuckDuckGoSearch)

// WithDuckMarkdown selects Markdown output for Call() instead of the legacy plain-text layout.
func WithDuckMarkdown(markdown bool) DuckOption {
	return func(d *DuckDuckGoSearch) {
		d.Markdown = markdown
	}
}

// WithDuckCount limits how many items to include in the formatted result (1–20).
func WithDuckCount(count int) DuckOption {
	return func(d *DuckDuckGoSearch) {
		if count < 1 {
			count = 1
		}
		if count > 20 {
			count = 20
		}
		d.Count = count
	}
}

// WithDuckHTTPClient sets the HTTP client (for tests or custom timeouts).
func WithDuckHTTPClient(c *http.Client) DuckOption {
	return func(d *DuckDuckGoSearch) {
		if c != nil {
			d.Client = c
		}
	}
}

// NewDuckDuckGoSearch builds a DuckDuckGo search tool with optional configuration.
// Use WithDuckMarkdown(true) so Call returns Markdown (html snippets → markdown via html-to-markdown;
// code-shaped snippets fenced). Default is legacy plain-text lines (“Title:… URL:…”).
func NewDuckDuckGoSearch(opts ...DuckOption) (*DuckDuckGoSearch, error) {
	d := &DuckDuckGoSearch{
		Count:  10,
		Client: http.DefaultClient,
	}
	for _, o := range opts {
		o(d)
	}
	return d, nil
}

// Name returns the tool name exposed to the LLM.
func (d *DuckDuckGoSearch) Name() string {
	return "DuckDuckGo_Search"
}

// Description returns the tool description for the LLM.
func (d *DuckDuckGoSearch) Description() string {
	return "A privacy-focused web search using DuckDuckGo. " +
		"Useful for current information and research. " +
		"Input should be a single search query string."
}

// Call runs the search and returns a human-readable block of results (same contract as Brave_Search / Bocha_Search).
func (d *DuckDuckGoSearch) Call(ctx context.Context, input string) (string, error) {
	q := strings.TrimSpace(input)
	if q == "" {
		return "", fmt.Errorf("empty search query")
	}

	if out, ok := d.searchHTML(ctx, q); ok {
		return out, nil
	}
	return d.searchJSON(ctx, q)
}

type ddgHit struct {
	Title   string
	URL     string
	Snippet string
}

func escapeMDLinkText(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "[", "\\[")
	s = strings.ReplaceAll(s, "]", "\\]")
	return s
}

func looksLikeCodeSample(s string) bool {
	s = strings.TrimSpace(s)
	if len(s) < 8 {
		return false
	}
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) == 1 {
		t := strings.TrimSpace(lines[0])
		if strings.HasPrefix(t, "go get ") || strings.HasPrefix(t, "go install ") ||
			strings.HasPrefix(t, "curl ") || strings.HasPrefix(t, "wget ") {
			return true
		}
	}
	if len(lines) < 2 {
		return false
	}
	prefixes := []string{
		"package ", "import ", "func ", "type ", "var ", "const ", "struct ",
		"#include", "def ", "class ", "fn ", "let ", "pub ", "use ",
	}
	for _, line := range lines {
		t := strings.TrimSpace(line)
		for _, p := range prefixes {
			if strings.HasPrefix(t, p) {
				return true
			}
		}
	}
	if strings.Count(s, "{") >= 2 && strings.Count(s, "}") >= 2 {
		return true
	}
	return false
}

func guessFenceLang(s string) string {
	s = strings.TrimSpace(s)
	switch {
	case strings.Contains(s, "package ") || (strings.Contains(s, "func ") && strings.Contains(s, "{")):
		return "go"
	case strings.HasPrefix(strings.TrimSpace(s), "#include") || strings.Contains(s, "int main"):
		return "c"
	case strings.Contains(s, "def ") && strings.Contains(s, ":"):
		return "python"
	case strings.Contains(s, "=>") || strings.Contains(s, "function "):
		return "javascript"
	default:
		return "text"
	}
}

func maybeFenceSnippetBody(body string) string {
	body = strings.TrimSpace(body)
	if body == "" || strings.HasPrefix(body, "```") || strings.Contains(body, "\n```") {
		return body
	}
	if looksLikeCodeSample(body) {
		return "```" + guessFenceLang(body) + "\n" + body + "\n```"
	}
	return body
}

func formatHitsPlain(hits []ddgHit) string {
	var sb strings.Builder
	for i, h := range hits {
		sb.WriteString(fmt.Sprintf("%d. Title: %s\nURL: %s\n", i+1, h.Title, h.URL))
		if h.Snippet != "" {
			sb.WriteString(fmt.Sprintf("Description: %s\n", h.Snippet))
		}
		sb.WriteString("\n")
	}
	return sb.String()
}

func formatHitsMarkdown(hits []ddgHit) string {
	var sb strings.Builder
	sb.WriteString("# DuckDuckGo search results\n\n")
	for i, h := range hits {
		sb.WriteString(fmt.Sprintf("## %d. [%s](%s)\n\n", i+1, escapeMDLinkText(h.Title), h.URL))
		if h.Snippet != "" {
			body := maybeFenceSnippetBody(h.Snippet)
			sb.WriteString(body)
			sb.WriteString("\n\n---\n\n")
		} else {
			sb.WriteString("---\n\n")
		}
	}
	return strings.TrimSpace(sb.String())
}

func (d *DuckDuckGoSearch) searchHTML(ctx context.Context, query string) (string, bool) {
	form := url.Values{}
	form.Set("q", query)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, ddgHTMLSearchURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", false
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", ddgChromeUserAgent)
	req.Header.Set("Referer", "https://html.duckduckgo.com/")

	resp, err := d.Client.Do(req)
	if err != nil {
		return "", false
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil || resp.StatusCode != http.StatusOK {
		return "", false
	}

	if strings.Contains(string(body), "anomaly-modal__title") {
		return "", false
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(string(body)))
	if err != nil {
		return "", false
	}

	var hits []ddgHit
	doc.Find("div.web-result").Each(func(_ int, s *goquery.Selection) {
		if len(hits) >= d.Count {
			return
		}
		a := s.Find("a.result__a").First()
		href, _ := a.Attr("href")
		title := strings.TrimSpace(a.Text())
		if title == "" || href == "" {
			return
		}
		snip := s.Find("a.result__snippet").First()
		if snip.Length() == 0 {
			snip = s.Find(".result__snippet").First()
		}
		snippetText := strings.TrimSpace(snip.Text())
		var snippet string
		if d.Markdown {
			if htmlFrag, _ := snip.Html(); htmlFrag != "" {
				if conv := snippetHTMLToMarkdown(htmlFrag); conv != "" {
					snippet = conv
				}
			}
			if snippet == "" {
				snippet = snippetText
			}
		} else {
			snippet = snippetText
		}
		hits = append(hits, ddgHit{Title: title, URL: href, Snippet: snippet})
	})

	if len(hits) == 0 {
		return "", false
	}
	if d.Markdown {
		return formatHitsMarkdown(hits), true
	}
	return formatHitsPlain(hits), true
}

func (d *DuckDuckGoSearch) searchJSON(ctx context.Context, query string) (string, error) {
	u, err := url.Parse(ddgJSONAPIURL)
	if err != nil {
		return "", err
	}
	q := u.Query()
	q.Set("q", query)
	q.Set("format", "json")
	q.Set("no_html", "1")
	q.Set("no_redirect", "1")
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return "", fmt.Errorf("duckduckgo json: %w", err)
	}
	req.Header.Set("User-Agent", ddgChromeUserAgent)

	resp, err := d.Client.Do(req)
	if err != nil {
		return "", fmt.Errorf("duckduckgo json request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("duckduckgo json: status %d", resp.StatusCode)
	}

	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("duckduckgo json decode: %w", err)
	}

	if d.Markdown {
		return formatJSONResultMarkdown(result, d.Count), nil
	}

	var sb strings.Builder
	if t := strings.TrimSpace(toStr(result["AbstractText"])); t != "" {
		sb.WriteString("Summary:\n" + t + "\n\n")
	}
	if heading := strings.TrimSpace(toStr(result["Heading"])); heading != "" && sb.Len() == 0 {
		sb.WriteString("Heading: " + heading + "\n\n")
	}
	if a := strings.TrimSpace(toStr(result["Answer"])); a != "" {
		sb.WriteString("Answer:\n" + a + "\n\n")
	}

	if rt, ok := result["RelatedTopics"].([]any); ok {
		lines := flattenRelatedTopics(rt, d.Count)
		for i, line := range lines {
			sb.WriteString(fmt.Sprintf("%d. %s\n\n", i+1, line))
		}
	}

	if results, ok := result["Results"].([]any); ok {
		idx := 0
		for _, rly := range results {
			if idx >= d.Count {
				break
			}
			m, ok := rly.(map[string]any)
			if !ok {
				continue
			}
			t := strings.TrimSpace(toStr(m["Text"]))
			if t == "" {
				t = strings.TrimSpace(toStr(m["Result"]))
			}
			u := strings.TrimSpace(toStr(m["FirstURL"]))
			if t == "" {
				continue
			}
			idx++
			if u != "" {
				sb.WriteString(fmt.Sprintf("%d. %s\nURL: %s\n\n", idx, t, u))
			} else {
				sb.WriteString(fmt.Sprintf("%d. %s\n\n", idx, t))
			}
		}
	}

	if sb.Len() == 0 {
		return "No results found", nil
	}
	return sb.String(), nil
}

// formatJSONResultMarkdown renders the DuckDuckGo instant-answer JSON payload as Markdown.
func formatJSONResultMarkdown(result map[string]any, maxTopics int) string {
	var parts []string
	header := "# DuckDuckGo (instant answer API)\n\n"

	if t := strings.TrimSpace(toStr(result["AbstractText"])); t != "" {
		u := strings.TrimSpace(toStr(result["AbstractURL"]))
		sec := "## Summary\n\n"
		if u != "" {
			sec += fmt.Sprintf("Source: <%s>\n\n", u)
		}
		sec += maybeFenceSnippetBody(t)
		parts = append(parts, sec)
	}
	if heading := strings.TrimSpace(toStr(result["Heading"])); heading != "" && len(parts) == 0 {
		parts = append(parts, "## Heading\n\n"+heading)
	}
	if a := strings.TrimSpace(toStr(result["Answer"])); a != "" {
		parts = append(parts, "## Answer\n\n"+maybeFenceSnippetBody(a))
	}

	if rt, ok := result["RelatedTopics"].([]any); ok {
		lines := flattenRelatedTopics(rt, maxTopics)
		if len(lines) > 0 {
			var b strings.Builder
			b.WriteString("## Related\n\n")
			for _, line := range lines {
				b.WriteString("- ")
				b.WriteString(strings.ReplaceAll(line, "\n", " "))
				b.WriteString("\n")
			}
			parts = append(parts, b.String())
		}
	}

	if results, ok := result["Results"].([]any); ok {
		idx := 0
		for _, rly := range results {
			if idx >= maxTopics {
				break
			}
			m, ok := rly.(map[string]any)
			if !ok {
				continue
			}
			t := strings.TrimSpace(toStr(m["Text"]))
			if t == "" {
				t = strings.TrimSpace(toStr(m["Result"]))
			}
			u := strings.TrimSpace(toStr(m["FirstURL"]))
			if t == "" {
				continue
			}
			idx++
			sec := fmt.Sprintf("## %d. ", idx)
			if u != "" {
				sec += fmt.Sprintf("[%s](%s)\n\n", escapeMDLinkText(t), u)
			} else {
				sec += t + "\n\n"
			}
			parts = append(parts, strings.TrimSpace(sec))
		}
	}

	if len(parts) == 0 {
		return "No results found"
	}
	return strings.TrimSpace(header + strings.Join(parts, "\n\n---\n\n"))
}

func toStr(v any) string {
	if v == nil {
		return ""
	}
	s, _ := v.(string)
	return s
}

func flattenRelatedTopics(topics []any, maxN int) []string {
	var out []string
	var walk func([]any)
	walk = func(items []any) {
		for _, it := range items {
			if len(out) >= maxN {
				return
			}
			m, ok := it.(map[string]any)
			if !ok {
				continue
			}
			if nested, ok := m["Topics"].([]any); ok && len(nested) > 0 {
				walk(nested)
				continue
			}
			text := strings.TrimSpace(toStr(m["Text"]))
			first := strings.TrimSpace(toStr(m["FirstURL"]))
			if text == "" {
				continue
			}
			if first != "" {
				out = append(out, text+"\nURL: "+first)
			} else {
				out = append(out, text)
			}
		}
	}
	walk(topics)
	return out
}
