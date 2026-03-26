package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"gotryai/internal/agent/blogcomposer"

	"github.com/joho/godotenv"
	"github.com/smallnest/langgraphgo/graph"
	"github.com/tmc/langchaingo/llms/openai"
	"github.com/tmc/langchaingo/tools"

	mytool "gotryai/pkg/tool"
)

func main() {
	godotenv.Load()

	draft := strings.TrimSpace(`
Title idea: try langgraphgo

Rough notes: it is this package smallnest/langgraphgo; basic examples of making an AI agent app, invoke agent, structured output, workflow graph, agent, etc.
`)

	webSearch, err := mytool.NewDuckDuckGoSearch(
		mytool.WithDuckCount(8),
		mytool.WithDuckMarkdown(true),
	)
	// webSearch, err := tool.NewBochaSearch(os.Getenv("BOCHA_API_KEY"))
	if err != nil {
		panic(err)
	}
	fetchText, err := mytool.NewFetchPageText()
	if err != nil {
		panic(err)
	}

	// DeepSeek (commented — switch back by restoring these blocks and removing OpenRouter below)
	llm, err := openai.New(
		openai.WithBaseURL("https://api.deepseek.com"),
		openai.WithToken(os.Getenv("DEEPSEEK_API_KEY")),
		openai.WithModel("deepseek-chat"),
	)
	if err != nil {
		panic(err)
	}
	llmStructured, err := openai.New(
		openai.WithBaseURL("https://api.deepseek.com"),
		openai.WithToken(os.Getenv("DEEPSEEK_API_KEY")),
		openai.WithModel("deepseek-chat"),
		openai.WithResponseFormat(openai.ResponseFormatJSON),
	)
	if err != nil {
		panic(err)
	}

	// llm, err := openai.New(
	// 	openai.WithBaseURL("https://openrouter.ai/api/v1"),
	// 	openai.WithToken(os.Getenv("OPENROUTER_API_KEY")),
	// 	openai.WithModel("openai/gpt-4o-mini"),
	// )
	// if err != nil {
	// 	panic(err)
	// }

	// llmStructured, err := openai.New(
	// 	openai.WithBaseURL("https://openrouter.ai/api/v1"),
	// 	openai.WithToken(os.Getenv("OPENROUTER_API_KEY")),
	// 	openai.WithModel("openai/gpt-4o-mini"),
	// 	openai.WithResponseFormat(openai.ResponseFormatJSON),
	// )
	// if err != nil {
	// 	panic(err)
	// }

	graphOpts := []blogcomposer.Option{
		blogcomposer.WithUpfrontResearch(true),
	}
	if p := strings.TrimSpace(os.Getenv("BLOG_COMPOSER_SQLITE_PATH")); p != "" {
		secStore, err := blogcomposer.OpenSQLiteSectionDraftStore(p)
		if err != nil {
			panic(err)
		}
		defer blogcomposer.CloseSectionDraftStore(secStore)
		graphOpts = append(graphOpts, blogcomposer.WithSectionDraftStore(secStore))
	}

	g := blogcomposer.NewGraph(llm, llmStructured, webSearch, []tools.Tool{fetchText}, graphOpts...)
	g.AddGlobalListener(&nodeLogger{})

	runnable, err := g.CompileListenable()
	if err != nil {
		panic(err)
	}

	ctx := context.Background()
	initial := blogcomposer.State{Draft: draft}

	fmt.Println()
	fmt.Printf("[%s] chain start\n", ts())
	final, err := runnable.Invoke(ctx, initial)
	fmt.Printf("[%s] chain end\n", ts())
	if err != nil {
		panic(err)
	}

	out, _ := json.MarshalIndent(final.Blueprint, "", "  ")
	fmt.Printf("\nblueprint:\n%s\n", out)
	fmt.Printf("\nword count: %d\n", blogcomposer.CountWords(final.FinalPost))
}

func ts() string { return time.Now().Format("15:04:05.000") }

type nodeLogger struct{}

func (l *nodeLogger) OnNodeEvent(
	_ context.Context, event graph.NodeEvent, nodeName string, state blogcomposer.State, err error,
) {
	switch event {
	case graph.NodeEventStart:
		fmt.Printf("[%s] ▶ %q\n", ts(), nodeName)
	case graph.NodeEventComplete:
		fmt.Printf("[%s] ✓ %q\n", ts(), nodeName)
	case graph.NodeEventError:
		fmt.Printf("[%s] ✗ %q: %v\n", ts(), nodeName, err)
	}
}
