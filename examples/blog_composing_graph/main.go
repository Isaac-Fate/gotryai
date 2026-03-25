package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"gotryai/internal/agent/blogcomposer"
	mytool "gotryai/pkg/tool"

	"github.com/joho/godotenv"
	"github.com/smallnest/langgraphgo/graph"
	"github.com/tmc/langchaingo/llms/openai"
)

func main() {
	godotenv.Load()

	draft := strings.TrimSpace(`
Title idea: try langgraphgo

Rough notes: it is this package smallnest/langgraphgo; basic examples of making an AI agent app, invoke agent, structured output, workflow graph, agent, etc.
`)

	webSearch, err := mytool.NewDuckDuckGoSearch(mytool.WithDuckCount(8))
	if err != nil {
		panic(err)
	}

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

	g := blogcomposer.NewGraph(llm, llmStructured, webSearch)
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
