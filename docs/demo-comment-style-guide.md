# Demo Comment Style Guide

A reproducible style guide for comments in gotryai examples. Use this when building demos to keep code readable and teachable.

## Principles

- **Teach, don't narrate**: Comments explain *why* and *how*, not what the code obviously does
- **Scannable**: Section separators and structure help readers jump to relevant parts
- **Consistent**: Same patterns across demos reduce cognitive load
- **Minimal**: Prefer clear code over comments; avoid redundancy

---

## 1. Package Documentation

Every demo must have a package doc before `package main`.

### Format

```go
// One-line summary of what this example demonstrates.
//
// Optional paragraph(s) explaining the concept, pattern, or flow.
// Use numbered lists for internal flow or step-by-step logic.
//
// Run: go run . (requires DEEPSEEK_API_KEY in .env)
package main
```

### Rules

- **First line**: Start with "This example demonstrates..." or "Package main demonstrates..."
- **Body**: 1–3 paragraphs max; explain the *concept* the demo teaches
- **Internal flow** (optional): Use numbered list for prebuilt/agent flow, graph execution, etc.
- **Run instruction**: Always end with `Run: go run . (requires DEEPSEEK_API_KEY in .env)` or equivalent

### Example

```go
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
```

---

## 2. Section Separators

Use `// --- Section ---` to divide the file into logical blocks.

### Standard Sections (in order)

| Section      | Use when                                      |
|--------------|------------------------------------------------|
| `// --- Types ---`    | Type definitions, structs, constants           |
| `// --- Tools ---`   | Tool implementations (agent demos)            |
| `// --- Listeners ---` | Event listeners (graph demos)                |
| `// --- Main ---`    | `main()` and helper functions                  |

### Rules

- Blank line before and after the separator
- Use title case
- Omit sections that don't apply (e.g. no `// --- Tools ---` if no tools)

---

## 3. Main Function Structure

Use numbered step comments to structure `main()`.

### Format

```go
func main() {
	godotenv.Load()

	// --- 1. Setup / Sample input ---
	// ...

	// --- 2. Build / Configure ---
	// ...

	// --- 3. Run / Execute ---
	// ...
}
```

### Rules

- **Format**: `// --- N. Short label ---`
- **Labels**: Use verbs (Setup, Build, Run, Attach, Compile)
- **Count**: 2–5 steps typical; avoid deep nesting
- **Inline**: Add brief comments only when the *why* isn't obvious

---

## 4. Type and Struct Documentation

Document exported types and non-obvious structs.

### Format

```go
// TypeName is a one-line description.
//
// Optional second paragraph for details, constraints, or usage.
type TypeName struct { ... }
```

### Rules

- **Exported types**: Always document
- **Unexported types**: Document if non-obvious (e.g. custom unmarshaler, schema types)
- **First line**: "X is the..." or "X does/returns..."
- **Keep it short**: One sentence usually suffices

### Example

```go
// SaveTodoItemsInput is the structured input for save_todo_items.
type SaveTodoItemsInput struct { ... }

// FlexibleDate parses dates from LLMs that often omit timezone (e.g. "2025-03-14T23:59:59").
// time.Time expects RFC3339; FlexibleDate tries multiple layouts before failing.
// It is a good example of specifying custom unmarshal logic for a field.
type FlexibleDate time.Time

// MyState is the graph state; keys: "summary" (string), "todo_items" ([]TodoItem).
type MyState map[string]any
```

---

## 5. Function and Method Documentation

Document exported functions and non-obvious helpers.

### Format

```go
// FunctionName does X.
//
// Optional paragraph: when to use, edge cases, relationship to other code.
func FunctionName() { ... }
```

### Rules

- **Exported**: Always document
- **Helpers** (e.g. `printAgentResponse`): Document in one line
- **Methods**: Document interface implementations, especially when behavior differs from default

### Example

```go
// GetCurrentDateTimeTool returns the current time.
//
// Does not implement ToolWithSchema, so prebuilt.getToolSchema() returns the
// default schema. The LLM sends {"input":"..."} and the agent extracts args["input"].
type GetCurrentDateTimeTool struct{}

// printAgentResponse prints the conversation: human → ai (+ tool calls) → tool → ai.
func printAgentResponse(resp map[string]any) { ... }
```

---

## 6. Inline Comments

Use sparingly; prefer self-explanatory code.

### When to Use

- **Non-obvious behavior**: e.g. "Chain events are stream-only—NodeListeners never receive them"
- **Intent**: e.g. "Inspect the schema that getToolSchema() returns and passes to the LLM"
- **Dual-purpose setup**: e.g. `// For free-form text (summary)` vs `// For JSON output (todo extraction)`

### When to Avoid

- **Obvious code**: e.g. `// Call the LLM` right before `llm.Call(...)` — the code is clear
- **Redundant**: e.g. `// Create a variable` before `x := ...`
- **Step-by-step narration**: Prefer section separators and clear structure

### Node Logic (Graph Demos)

Inside node functions, use short labels only when the step isn't obvious:

```go
// Prompt template
promptTemplate := prompts.NewPromptTemplate(...)

// Create the prompt
prompt, err := promptTemplate.Format(...)
```

For simple linear flows (template → format → call → parse → update state), these can be omitted if the code is readable.

---

## 7. Trailing Comments

Use for brief inline clarification.

```go
g.AddGlobalListener(&EventLogger{})                   // Logs all node events and state
extractTodoItemsNode.AddListener(&TodoItemReporter{}) // Node-specific: only extract_todo_items
```

- **Keep short**: One phrase
- **Align** (optional): Use spaces for readability when multiple trailing comments

---

## 8. Quick Reference

```
Package doc:  Summary + concept + Run instruction
Sections:     // --- Types ---  // --- Tools ---  // --- Main ---
Main steps:   // --- 1. Setup ---  // --- 2. Build ---  // --- 3. Run ---
Type doc:     // TypeName is/does...
Func doc:     // FuncName does... (one line for helpers)
Inline:       Only when why/intent isn't obvious
```

---

## See Also

- `demo-printing-style-guide.md` — Terminal output formatting for demos
