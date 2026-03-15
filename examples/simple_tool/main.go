// This example demonstrates an agent with a simple tool.
//
// RollDiceTool implements tools.Tool (Name, Description, Call). It does not
// implement ToolWithSchema, so the agent uses the default schema: {"input":"string"}.
//
// Run: go run . (requires DEEPSEEK_API_KEY in .env)
package main

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
	"github.com/smallnest/langgraphgo/prebuilt"
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

	inputTools := []tools.Tool{&RollDiceTool{}}

	runnable, err := prebuilt.CreateAgentMap(llm, inputTools, 10)
	if err != nil {
		panic(err)
	}

	ctx := context.Background()

	// --- 2. Run ---
	initialState := map[string]any{
		"messages": []llms.MessageContent{
			llms.TextParts(
				llms.ChatMessageTypeHuman,
				"Roll a dice for 3 times and tell me the result.",
			),
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
				if len(text) > 200 {
					text = text[:200] + "..."
				}
				fmt.Printf("    %s\n", strings.ReplaceAll(text, "\n", "\n    "))
			case llms.ToolCall:
				if p.FunctionCall != nil {
					args := p.FunctionCall.Arguments
					if len(args) > 100 {
						args = args[:100] + "..."
					}
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

// --- Tools ---

// RollDiceTool rolls a 6-sided dice and returns the result.
type RollDiceTool struct{}

func (t *RollDiceTool) Name() string {
	return "roll_dice"
}

func (t *RollDiceTool) Description() string {
	return "Roll a 6-sided dice and return the result."
}

func (t *RollDiceTool) Call(ctx context.Context, input string) (string, error) {
	return strconv.Itoa(rand.Intn(6) + 1), nil
}
