package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/joho/godotenv"
	"github.com/smallnest/langgraphgo/prebuilt"
	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/llms/openai"
	"github.com/tmc/langchaingo/tools"
)

// Browser-like UA reduces DuckDuckGo “anomaly” bot interstitials compared to Go’s default client string.
const ddgChromeUserAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/122.0.0.0 Safari/537.36"

const ddgHTMLSearchURL = "https://html.duckduckgo.com/html/"
const ddgJSONAPIURL = "https://api.duckduckgo.com/"

// DuckDuckGoSearch implements langchaingo/tools.Tool like langgraphgo’s BraveSearch and BochaSearch:
// Name, Description, and Call(ctx, input string). No API key required.
type DuckDuckGoSearch struct {
	Count  int
	Client *http.Client
}

type DuckOption func(*DuckDuckGoSearch)

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

	var sb strings.Builder
	n := 0
	doc.Find("div.web-result").Each(func(_ int, s *goquery.Selection) {
		if n >= d.Count {
			return
		}
		a := s.Find("a.result__a").First()
		href, _ := a.Attr("href")
		title := strings.TrimSpace(a.Text())
		if title == "" || href == "" {
			return
		}
		snippet := strings.TrimSpace(s.Find("a.result__snippet").Text())
		if snippet == "" {
			snippet = strings.TrimSpace(s.Find(".result__snippet").Text())
		}
		n++
		sb.WriteString(fmt.Sprintf("%d. Title: %s\nURL: %s\n", n, title, href))
		if snippet != "" {
			sb.WriteString(fmt.Sprintf("Description: %s\n", snippet))
		}
		sb.WriteString("\n")
	})

	if sb.Len() == 0 {
		return "", false
	}
	return sb.String(), true
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

// --- Demo ---

func main() {
	godotenv.Load()

	ddg, err := NewDuckDuckGoSearch(WithDuckCount(5))
	if err != nil {
		panic(err)
	}

	ctx := context.Background()

	fmt.Println("=== Direct DuckDuckGo_Search.Call (no LLM) ===")
	out, err := ddg.Call(ctx, "langgraphgo golang agent")
	if err != nil {
		fmt.Fprintf(os.Stderr, "search: %v\n", err)
	} else {
		fmt.Println(out)
	}

	fmt.Println("=== LangGraphGo ReAct agent with DuckDuckGo_Search ===")

	if os.Getenv("DEEPSEEK_API_KEY") == "" {
		fmt.Println("Skip: set DEEPSEEK_API_KEY to run the prebuilt.CreateAgentMap demo (direct search above needs no keys).")
		return
	}

	llm, err := openai.New(
		openai.WithBaseURL("https://api.deepseek.com"),
		openai.WithToken(os.Getenv("DEEPSEEK_API_KEY")),
		openai.WithModel("deepseek-chat"),
	)
	if err != nil {
		panic(err)
	}

	inputTools := []tools.Tool{ddg}
	runnable, err := prebuilt.CreateAgentMap(llm, inputTools, 8)
	if err != nil {
		panic(err)
	}

	initialState := map[string]any{
		"messages": []llms.MessageContent{
			llms.TextParts(llms.ChatMessageTypeHuman,
				"Use DuckDuckGo_Search once to find what langgraphgo is, then reply in one short paragraph."),
		},
	}

	resp, err := runnable.Invoke(ctx, initialState)
	if err != nil {
		panic(err)
	}

	fmt.Println()
	printAgentResponse(resp)
}

func printAgentResponse(resp map[string]any) {
	msgs, ok := resp["messages"].([]llms.MessageContent)
	if !ok {
		fmt.Printf("%+v\n", resp)
		return
	}

	if n, ok := resp["iteration_count"].(int); ok {
		fmt.Println("┌─────────────────────────────────────────────────────────────")
		fmt.Printf("│ Agent completed in %d iteration(s)\n", n)
		fmt.Println("└─────────────────────────────────────────────────────────────")
		fmt.Println()
	}

	numMessages := len(msgs)
	for i, msg := range msgs {
		role := "?"
		switch msg.Role {
		case llms.ChatMessageTypeHuman:
			role = "Human"
		case llms.ChatMessageTypeAI:
			role = "AI"
		case llms.ChatMessageTypeTool:
			role = "Tool"
		}

		fmt.Printf("▸ [%d] %s\n", i+1, role)

		for _, part := range msg.Parts {
			switch p := part.(type) {
			case llms.TextContent:
				text := strings.TrimSpace(p.Text)
				if i < numMessages-1 {
					text = truncateText(text, 200)
				}
				fmt.Printf("    %s\n", strings.ReplaceAll(text, "\n", "\n    "))
			case llms.ToolCall:
				if p.FunctionCall != nil {
					args := p.FunctionCall.Arguments
					args = truncateText(args, 100)
					fmt.Printf("    → tool: %s(%s)\n", p.FunctionCall.Name, args)
				}
			case llms.ToolCallResponse:
				fmt.Printf("    ← %s: %s\n", p.Name, strings.TrimSpace(truncateText(p.Content, 400)))
			default:
				fmt.Printf("    %v\n", part)
			}
		}
		fmt.Println()
	}
}

func truncateText(text string, maxLength int) string {
	if maxLength <= 0 {
		return text
	}
	if len(text) > maxLength {
		return text[:maxLength] + "..."
	}
	return text
}
