// This example demonstrates structured JSON output from an LLM.
//
// Use openai.WithResponseFormat(openai.ResponseFormatJSON) and pass a JSON
// schema in the prompt so the model returns parseable JSON matching the schema.
//
// Run: go run . (requires DEEPSEEK_API_KEY in .env)
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/invopop/jsonschema"
	"github.com/joho/godotenv"
	"github.com/tmc/langchaingo/llms/openai"
	"github.com/tmc/langchaingo/prompts"
)

// --- Main ---

func main() {
	godotenv.Load()

	// --- 1. Setup schema and LLM ---
	reflector := jsonschema.Reflector{DoNotReference: true}
	schema := reflector.Reflect(&[]TodoItem{})

	b, err := json.MarshalIndent(schema, "", "  ")
	if err != nil {
		panic(err)
	}

	schemaString := string(b)

	llm, err := openai.New(
		openai.WithBaseURL("https://api.deepseek.com"),
		openai.WithToken(os.Getenv("DEEPSEEK_API_KEY")),
		openai.WithModel("deepseek-chat"),
		openai.WithResponseFormat(openai.ResponseFormatJSON),
	)
	if err != nil {
		panic(err)
	}

	ctx := context.Background()

	// --- 2. Call LLM and parse response ---
	promptTemplate := prompts.NewPromptTemplate(`{{.input}}
	You must return a JSON object that matches the following schema:
	{{ .schema }}
	`, []string{"input", "schema"})

	prompt, err := promptTemplate.Format(map[string]any{
		"input":  "Need to buy a cup of coffee and then learn langchaingo package. After that, watch a movie if there is time.",
		"schema": schemaString,
	})
	if err != nil {
		panic(err)
	}

	resp, err := llm.Call(
		ctx,
		prompt,
	)
	if err != nil {
		panic(err)
	}

	todoItems := []TodoItem{}

	err = json.Unmarshal([]byte(resp), &todoItems)
	if err != nil {
		panic(err)
	}

	fmt.Println()
	fmt.Println("┌─────────────────────────────────────────────────────────────")
	fmt.Println("│ Structured output (Todo items)")
	fmt.Println("└─────────────────────────────────────────────────────────────")
	fmt.Println()
	fmt.Println("  📋 Todo items:")
	for i, item := range todoItems {
		fmt.Printf("    %d. %s\n", i+1, item.Title)
		if item.Description != "" {
			fmt.Printf("       %s\n", item.Description)
		}
		fmt.Printf("       priority: %s\n", item.Priority)
	}
	fmt.Println()
}

// --- Types ---

// TodoItem represents a todo item with title, description, and priority.
type TodoItem struct {
	Title       string           `json:"title"`
	Description string           `json:"description,omitempty"`
	Priority    TodoItemPriority `json:"priority"              jsonschema:"enum=low,enum=normal,enum=high,default=normal,description=Priority of the todo item"`
}

type TodoItemPriority string

const (
	TodoItemPriorityLow    TodoItemPriority = "low"
	TodoItemPriorityNormal TodoItemPriority = "normal"
	TodoItemPriorityHigh   TodoItemPriority = "high"
)
