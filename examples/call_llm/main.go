// This example demonstrates a minimal LLM call.
//
// No agent, no tools—just llm.Call() with a prompt.
//
// Run: go run . (requires DEEPSEEK_API_KEY in .env)
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/joho/godotenv"
	"github.com/tmc/langchaingo/llms/openai"
)

// --- Main ---

func main() {
	godotenv.Load()

	// --- 1. Setup and call ---
	llm, err := openai.New(
		openai.WithBaseURL("https://api.deepseek.com"),
		openai.WithToken(os.Getenv("DEEPSEEK_API_KEY")),
		openai.WithModel("deepseek-chat"),
	)
	if err != nil {
		panic(err)
	}

	ctx := context.Background()

	resp, err := llm.Call(ctx, "Who are you?")
	if err != nil {
		panic(err)
	}

	fmt.Println()
	fmt.Println("┌─────────────────────────────────────────────────────────────")
	fmt.Println("│ LLM Response")
	fmt.Println("└─────────────────────────────────────────────────────────────")
	fmt.Println()
	fmt.Println(resp)
	fmt.Println()
}
