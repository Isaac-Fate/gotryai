// This example demonstrates streaming graph execution.
//
// CompileListenable() returns a runnable that supports Stream(). Each event
// (chain start/end, node start/complete/error) is emitted as it happens.
//
// Run: go run . (requires DEEPSEEK_API_KEY in .env)
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/invopop/jsonschema"
	"github.com/joho/godotenv"
	"github.com/smallnest/langgraphgo/graph"
	"github.com/tmc/langchaingo/llms/openai"
	"github.com/tmc/langchaingo/prompts"
)

// --- Main ---

func main() {
	godotenv.Load()

	// --- 1. Setup: LLMs and sample input ---
	email := `
From: sarah@company.com
To: john@company.com
Subject: Action Items for Q4 Launch

Hi John,

Hope you had a good weekend! Did you catch the game on Saturday? Anyway, I've been meaning to reach out—sorry for the delay, things have been crazy on my end. The new coffee machine in the break room is finally fixed, small wins right?

So, following our sync yesterday, I need you to take care of a few things. Please review and sign off on the API documentation by March 14th—the draft is in the shared drive under /docs/api-v2. Coordinate with the DevOps team on the staging environment setup by March 18th since they're waiting for your deployment configs. Could you also update the runbook with the new monitoring alerts we discussed? Emma from SRE can help if needed—no hard deadline on that one, just whenever you get to it.

Let me know if you want to grab lunch sometime this week to catch up. Talk soon!

Best regards,
Sarah
`

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

	g := graph.NewStreamingStateGraph[MyState]()

	g.AddNode(
		"summarize_email",
		"Summarize the email",
		func(ctx context.Context, state MyState) (MyState, error) {
			promptTemplate := prompts.NewPromptTemplate(`
			You are a helpful assistant that summarizes emails.

			The email is:
			{{.email}}

			Your summary is (only return the summary, no other text):
			`,
				[]string{"email"},
			)

			prompt, err := promptTemplate.Format(map[string]any{
				"email": email,
			})
			if err != nil {
				panic(err)
			}

			resp, err := llm.Call(ctx, prompt)
			if err != nil {
				panic(err)
			}

			state["summary"] = resp

			return state, nil
		},
	)

	g.AddNode(
		"extract_todo_items",
		"Extract todo items from the summary",
		func(ctx context.Context, state MyState) (MyState, error) {
			promptTemplate := prompts.NewPromptTemplate(`
			You are a helpful assistant that extracts todo items from a summary.
			If the title of the todo item is clear enough, you don't need to add a description.
			
			Current date time is:
			{{.date_time}}

			The summary is:
			{{.summary}}

			You must return a JSON object that matches the following schema:
			{{ .schema }}
			`,
				[]string{"date_time", "summary", "schema"},
			)

			schema := jsonschema.Reflect(&TodoItemExtractionResult{})
			schemaBytes, err := json.MarshalIndent(schema, "", "  ")
			if err != nil {
				panic(err)
			}

			schemaString := string(schemaBytes)

			prompt, err := promptTemplate.Format(map[string]any{
				"date_time": time.Now().Format(time.RFC3339),
				"summary":   state["summary"],
				"schema":    schemaString,
			})
			if err != nil {
				panic(err)
			}

			resp, err := llmStructured.Call(ctx, prompt)
			if err != nil {
				panic(err)
			}

			result := TodoItemExtractionResult{}
			err = json.Unmarshal([]byte(resp), &result)
			if err != nil {
				panic(err)
			}

			state["todo_items"] = result.TodoItems

			return state, nil
		},
	)

	g.AddEdge("summarize_email", "extract_todo_items")
	g.AddEdge("extract_todo_items", graph.END)
	g.SetEntryPoint("summarize_email")

	ctx := context.Background()

	initialState := MyState{	}

	// --- 2. Build the graph: nodes and edges ---
	// (nodes and edges defined above)

	// --- 3. Stream events ---
	fmt.Println()

	runnable, err := g.CompileListenable()
	if err != nil {
		panic(err)
	}

	events := runnable.Stream(ctx, initialState)
	for event := range events {
		printEvent(event)
	}

}

// printEvent handles each stream event and prints node state.
func printEvent(event graph.StreamEvent[MyState]) {
	ts := event.Timestamp.Format("15:04:05.000")

	printState := func() {
		if len(event.State) == 0 {
			return
		}
		stateJSON, _ := json.MarshalIndent(event.State, "    ", "  ")
		fmt.Printf("  state:\n%s\n", stateJSON)
	}

	switch event.Event {
	case graph.EventChainStart:
		fmt.Printf("[%s] 🚀 Chain started\n", ts)
	case graph.EventChainEnd:
		fmt.Printf("[%s] 🏁 Chain ended\n", ts)
		printState()
	case graph.NodeEventStart:
		fmt.Printf("[%s] ▶️  Node '%s' started\n", ts, event.NodeName)
		printState()
	case graph.NodeEventComplete:
		dur := ""
		if event.Duration > 0 {
			dur = fmt.Sprintf(" (%v)", event.Duration.Round(time.Millisecond))
		}
		fmt.Printf("[%s] ✅ Node '%s' completed%s\n", ts, event.NodeName, dur)
		printState()
	case graph.NodeEventError:
		fmt.Printf("[%s] ❌ Node '%s' failed: %v\n", ts, event.NodeName, event.Error)
		printState()
	default:
		fmt.Printf("[%s] %s: %s\n", ts, event.Event, event.NodeName)
		printState()
	}
}

// --- Types ---

// MyState is the graph state; keys: "summary" (string), "todo_items" ([]TodoItem).
type MyState map[string]any

// TodoItem represents a single todo item extracted from the summary.
type TodoItem struct {
	Title       string     `json:"title"`
	Description string     `json:"description,omitempty"`
	DueDate     *time.Time `json:"due_date,omitempty"    jsonschema:"description=The date and time the todo item is due"`
}

// TodoItemExtractionResult is the JSON shape returned by the extract_todo_items node.
type TodoItemExtractionResult struct {
	TodoItems []TodoItem `json:"todo_items"`
}
