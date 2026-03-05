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

func main() {
	godotenv.Load()

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

	fmt.Printf("Todo items: %+v\n", todoItems)
}

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
