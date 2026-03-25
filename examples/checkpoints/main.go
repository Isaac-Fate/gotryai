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
	"github.com/smallnest/langgraphgo/store/sqlite"
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

	llm, err := openai.New( // For free-form text (summary)
		openai.WithBaseURL("https://api.deepseek.com"),
		openai.WithToken(os.Getenv("DEEPSEEK_API_KEY")),
		openai.WithModel("deepseek-chat"),
	)
	if err != nil {
		panic(err)
	}

	llmStructured, err := openai.New( // For JSON output (todo extraction)
		openai.WithBaseURL("https://api.deepseek.com"),
		openai.WithToken(os.Getenv("DEEPSEEK_API_KEY")),
		openai.WithModel("deepseek-chat"),
		openai.WithResponseFormat(openai.ResponseFormatJSON),
	)
	if err != nil {
		panic(err)
	}

	// --- 2. Build the graph: nodes and edges ---
	g := graph.NewCheckpointableStateGraph[MyState]()

	dbPath := os.Getenv("CHECKPOINT_STORE_SQLITE_DB_PATH")

	// Init checkpoint store
	checkpointStore, err := sqlite.NewSqliteCheckpointStore(sqlite.SqliteOptions{
		Path:      dbPath,
		TableName: "demo_checkpoints",
	})
	if err != nil {
		panic(err)
	}

	g.SetCheckpointConfig(graph.CheckpointConfig{
		Store:    checkpointStore,
		AutoSave: true,
		// SaveInterval:   1 * time.Second,
		// MaxCheckpoints: 10,
	})

	g.AddNode(
		"summarize_email",
		"Summarize the email",
		func(ctx context.Context, state MyState) (MyState, error) {
			promptTemplate := prompts.NewPromptTemplate(`
			You are a helpful assistant that summarizes emails.

			The email is:
			{{.email}}

			Your summary is:
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

	extractTodoItemsNode := g.AddNode(
		"extract_todo_items", // Depends on summary from previous node
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

	// --- 3. Attach listeners (must be after AddNode) ---
	g.AddGlobalListener(&EventLogger{})                   // Logs all node events and state
	extractTodoItemsNode.AddListener(&TodoItemReporter{}) // Node-specific: only extract_todo_items

	// --- 4. Compile and run ---
	runnable, err := g.CompileCheckpointable()
	if err != nil {
		panic(err)
	}

	ctx := context.Background()
	initialState := MyState{}

	// Chain events (EventChainStart, EventChainEnd) are stream-only—NodeListeners
	// never receive them. We log chain boundaries manually when using Invoke().
	fmt.Println()
	fmt.Printf("[%s] 🚀 Chain started\n", time.Now().Format("15:04:05.000"))
	result, err := runnable.InvokeWithConfig(ctx, initialState, &graph.Config{
		Configurable: map[string]any{
			"thread_id": "123",
		},
	})
	fmt.Printf("[%s] 🏁 Chain ended\n", time.Now().Format("15:04:05.000"))
	if err != nil {
		panic(err)
	}

	if len(result) > 0 {
		stateJSON, _ := json.MarshalIndent(result, "  ", "  ")
		fmt.Println()
		fmt.Println("┌─────────────────────────────────────────────────────────────")
		fmt.Println("│ Final state")
		fmt.Println("└─────────────────────────────────────────────────────────────")
		fmt.Println(string(stateJSON))
		fmt.Println()
	}
}

// --- Types ---

// MyState is the graph state; keys: "summary" (string), "todo_items" ([]TodoItem).
type MyState map[string]any

type TodoItem struct {
	Title       string     `json:"title"`
	Description string     `json:"description,omitempty"`
	DueDate     *time.Time `json:"due_date,omitempty"    jsonschema:"description=Due date for the todo item"`
}

type TodoItemExtractionResult struct {
	TodoItems []TodoItem `json:"todo_items"`
}

// --- Listeners ---

// EventLogger logs every node event (start, complete, error) and prints state.
// Receives: NodeEventStart, NodeEventComplete, NodeEventError. Does NOT receive
// EventChainStart/EventChainEnd—those are emitted only to Stream()'s channel.
type EventLogger struct{}

func (l *EventLogger) OnNodeEvent(
	ctx context.Context, event graph.NodeEvent, nodeName string, state MyState, err error,
) {
	ts := time.Now().Format("15:04:05.000")

	printState := func() {
		if len(state) == 0 {
			return
		}
		stateJSON, _ := json.MarshalIndent(state, "    ", "  ")
		fmt.Printf("    state:\n%s\n", stateJSON)
	}

	switch event {
	case graph.NodeEventStart:
		fmt.Printf("[%s] ▶️  Node '%s' started\n", ts, nodeName)
		printState()
	case graph.NodeEventComplete:
		fmt.Printf("[%s] ✅ Node '%s' completed\n", ts, nodeName)
		printState()
	case graph.NodeEventError:
		fmt.Printf("[%s] ❌ Node '%s' failed: %v\n", ts, nodeName, err)
		printState()
	default:
		fmt.Printf("[%s] %s: %s\n", ts, event, nodeName)
		printState()
	}
}

// TodoItemReporter pretty-prints extracted todo items. Attach via node.AddListener()
// so it only receives events from that node—no need to filter by nodeName.
//
// In this demo it only prints; in practice, the same pattern would persist to a DB,
// send an email, create calendar events, invoke external services, or trigger other downstream actions.
type TodoItemReporter struct{}

func (r *TodoItemReporter) OnNodeEvent(
	ctx context.Context, event graph.NodeEvent, nodeName string, state MyState, err error,
) {
	if event != graph.NodeEventComplete {
		return
	}
	v, ok := state["todo_items"]
	if !ok || v == nil {
		return
	}
	items, ok := v.([]TodoItem)
	if !ok {
		return
	}
	fmt.Println("\n  📋 Todo items:")
	for i, item := range items {
		fmt.Printf("    %d. %s\n", i+1, item.Title)
		if item.Description != "" {
			fmt.Printf("       %s\n", item.Description)
		}
		if item.DueDate != nil {
			fmt.Printf("       due: %s\n", item.DueDate.Format("2006-01-02"))
		}
	}
}
