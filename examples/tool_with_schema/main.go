// This example demonstrates tools with custom JSON Schema in LangGraphGo.
//
// Tools implement langchaingo tools.Tool (Name, Description, Call). For structured
// input (e.g. array of objects), implement prebuilt.ToolWithSchema and return a
// JSON Schema—the agent passes it to the LLM so it knows how to format arguments.
//
// Internal flow (langgraphgo prebuilt):
//  1. CreateAgentMap iterates inputTools and builds llms.Tool defs for each.
//  2. For each tool, it calls getToolSchema(t): if t implements ToolWithSchema,
//     returns t.Schema(); else returns default {"type":"object","properties":{"input":{...}}}.
//  3. Schema becomes FunctionDefinition.Parameters, sent to LLM via WithTools().
//  4. When LLM returns a tool call, the agent node checks ToolWithSchema: if present,
//     passes tc.FunctionCall.Arguments (raw JSON) to Call(); else extracts args["input"].
//
// Run: go run . (requires DEEPSEEK_API_KEY in .env)
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/invopop/jsonschema"
	"github.com/joho/godotenv"
	"github.com/smallnest/langgraphgo/prebuilt"
	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/llms/openai"
	"github.com/tmc/langchaingo/tools"
)

// --- Types ---

// SaveTodoItemsInput is the structured input for save_todo_items.
type SaveTodoItemsInput struct {
	TodoItems []TodoItem `json:"todo_items"`
}

type TodoItem struct {
	Title       string           `json:"title"`
	Description string           `json:"description,omitempty"`
	Priority    TodoItemPriority `json:"priority"              jsonschema:"enum=low,enum=normal,enum=high,default=normal,description=Priority of the todo item"`
	DueDate     *FlexibleDate    `json:"due_date,omitempty"    jsonschema:"description=Due date (YYYY-MM-DD or ISO 8601)"`
}

// FlexibleDate parses dates from LLMs that often omit timezone (e.g. "2025-03-14T23:59:59").
// time.Time expects RFC3339; FlexibleDate tries multiple layouts before failing.
// It is a good example of specifying custom unmarshal logic for a field.
type FlexibleDate time.Time

func (f *FlexibleDate) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	if s == "" {
		return nil
	}
	for _, layout := range []string{
		time.RFC3339,
		"2006-01-02T15:04:05Z",
		"2006-01-02T15:04:05",
		"2006-01-02",
	} {
		if t, err := time.Parse(layout, s); err == nil {
			*f = FlexibleDate(t)
			return nil
		}
	}
	return fmt.Errorf("invalid date: %q", s)
}

func (FlexibleDate) JSONSchema() *jsonschema.Schema {
	return &jsonschema.Schema{
		Type:        "string",
		Format:      "date-time",
		Description: "Due date (YYYY-MM-DD or ISO 8601)",
	}
}

type TodoItemPriority string

const (
	TodoItemPriorityLow    TodoItemPriority = "low"
	TodoItemPriorityNormal TodoItemPriority = "normal"
	TodoItemPriorityHigh   TodoItemPriority = "high"
)

// --- Tools ---

// GetCurrentDateTimeTool returns the current time.
//
// Does not implement ToolWithSchema, so prebuilt.getToolSchema() returns the
// default schema: {"type":"object","properties":{"input":{"type":"string"}}}. The
// LLM sends {"input":"..."} and the agent node extracts args["input"] before Call().
type GetCurrentDateTimeTool struct{}

func (t *GetCurrentDateTimeTool) Name() string        { return "get_current_date_time" }
func (t *GetCurrentDateTimeTool) Description() string { return "Get the current date and time." }

func (t *GetCurrentDateTimeTool) Call(ctx context.Context, input string) (string, error) {
	return time.Now().Format(time.RFC3339), nil
}

// SaveTodoItemsTool saves todo items.
//
// Implements ToolWithSchema, so getToolSchema() calls Schema() and uses the result
// as FunctionDefinition.Parameters. The LLM receives this schema and produces JSON
// matching it. When the agent executes the tool, it passes the raw JSON string
// directly to Call() (no extraction of an "input" field).
type SaveTodoItemsTool struct{}

func (t *SaveTodoItemsTool) Name() string        { return "save_todo_items" }
func (t *SaveTodoItemsTool) Description() string { return "Save the todo items to the database." }

// Schema implements prebuilt.ToolWithSchema. Called once at agent creation when
// building tool definitions. LLM APIs require type: "object" at root—use
// jsonschema.Reflector{ExpandedStruct: true} to inline the schema instead of $ref.
func (t *SaveTodoItemsTool) Schema() map[string]any {
	r := &jsonschema.Reflector{ExpandedStruct: true}
	schema := r.Reflect(&SaveTodoItemsInput{})
	data, _ := json.Marshal(schema)
	var schemaMap map[string]any
	_ = json.Unmarshal(data, &schemaMap)
	return schemaMap
}

// Call receives the raw JSON from the LLM's tool call (tc.FunctionCall.Arguments).
// With ToolWithSchema, the agent passes it directly; without it, would pass args["input"].
func (t *SaveTodoItemsTool) Call(ctx context.Context, input string) (string, error) {
	var req SaveTodoItemsInput
	if err := json.Unmarshal([]byte(input), &req); err != nil {
		return "", err
	}
	fmt.Println("  Saved:")
	for _, item := range req.TodoItems {
		due := ""
		if item.DueDate != nil {
			due = " due " + time.Time(*item.DueDate).Format("2006-01-02")
		}
		fmt.Printf("    • %s (priority: %s%s)\n", item.Title, item.Priority, due)
	}
	return "Todo items saved successfully.", nil
}

// --- Main ---

func main() {
	godotenv.Load()

	// --- 1. Sample input ---
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

	// --- 2. Setup LLM and tools ---
	llm, err := openai.New(
		openai.WithBaseURL("https://api.deepseek.com"),
		openai.WithToken(os.Getenv("DEEPSEEK_API_KEY")),
		openai.WithModel("deepseek-chat"),
	)
	if err != nil {
		panic(err)
	}

	saveTool := &SaveTodoItemsTool{}
	inputTools := []tools.Tool{
		&GetCurrentDateTimeTool{},
		saveTool,
	}

	// Inspect the schema that getToolSchema() returns and passes to the LLM.
	schemaJSON, _ := json.MarshalIndent(saveTool.Schema(), "", "  ")
	fmt.Println()
	fmt.Println("┌─────────────────────────────────────────────────────────────")
	fmt.Println("│ Schema for save_todo_items (Parameters sent to LLM)")
	fmt.Println("└─────────────────────────────────────────────────────────────")
	fmt.Println(string(schemaJSON))
	fmt.Println()

	// CreateAgentMap builds tool defs (Name, Description, Parameters=getToolSchema(t))
	// and wires the agent loop: agent node → tool node → agent node.
	runnable, err := prebuilt.CreateAgentMap(llm, inputTools, 10,
		prebuilt.WithSystemMessage(
			"You must call get_current_date_time first to get the current date, then extract todo items and save them. Use the current date when interpreting relative dates (e.g. 'by Friday' or 'next week').",
		),
	)
	if err != nil {
		panic(err)
	}

	// --- 3. Run: agent only sees messages; include email in the prompt ---
	ctx := context.Background()
	initialState := map[string]any{
		"messages": []llms.MessageContent{
			llms.TextParts(
				llms.ChatMessageTypeHuman,
				"Extract todo items from the email below and save them using the save_todo_items tool. Call get_current_date_time first.\n\nEmail:\n"+email,
			),
		},
	}

	resp, err := runnable.Invoke(ctx, initialState)
	if err != nil {
		panic(err)
	}

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
		fmt.Println()
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
