package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/joho/godotenv"
	"github.com/smallnest/langgraphgo/prebuilt"
	"github.com/smallnest/langgraphgo/tool"
	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/llms/openai"
	"github.com/tmc/langchaingo/tools"
)

// --- Main ---

func main() {
	godotenv.Load()

	// --- 1. Setup LLM and agent ---
	llm, err := openai.New(
		openai.WithBaseURL("https://api.deepseek.com"),
		openai.WithToken(os.Getenv("DEEPSEEK_API_KEY")),
		openai.WithModel("deepseek-chat"),
	)
	if err != nil {
		panic(err)
	}

	webSearch, err := tool.NewBochaSearch(os.Getenv("BOCHA_API_KEY"))
	// webSearch, err := tool.NewBraveSearch(os.Getenv("BRAVE_API_KEY"))
	if err != nil {
		panic(err)
	}

	inputTools := []tools.Tool{
		webSearch,
	}

	runnable, err := prebuilt.CreateAgentMap(llm, inputTools, 10)
	if err != nil {
		panic(err)
	}

	ctx := context.Background()

	// --- 2. Run ---
	initialState := map[string]any{
		"messages": []llms.MessageContent{
			llms.TextParts(llms.ChatMessageTypeHuman, "Tell me about langgraphgo package"),
		},
	}

	resp, err := runnable.Invoke(ctx, initialState)
	if err != nil {
		panic(err)
	}

	fmt.Println()
	printAgentResponse(resp)
}

// printAgentResponse prints the conversation: human → ai (+ tool calls) → tool → ai.
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
				fmt.Printf("    ← %s: %s\n", p.Name, strings.TrimSpace(p.Content))
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
