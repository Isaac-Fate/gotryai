# Demo Printing Style Guide

A reproducible style guide for terminal output in gotryai examples. Use this when building demos to keep output readable and consistent.

## Principles

- **Scannable**: Section headers and separators make it easy to find information
- **Hierarchical**: Indentation shows parent-child relationships
- **Consistent**: Same patterns across demos reduce cognitive load
- **Minimal**: Avoid visual noise; use symbols sparingly

---

## 1. Section Blocks

Use box-drawing characters for major section headers.

### Format

```
┌─────────────────────────────────────────────────────────────
│ Section title (optional subtitle)
└─────────────────────────────────────────────────────────────
```

### Rules

- **Width**: 61 characters (60 `─` plus corners)
- **Top/bottom**: `┌` `└` with `─` between
- **Title line**: `│` + space + title
- **Blank line** before and after the block

### Example

```
┌─────────────────────────────────────────────────────────────
│ Schema for save_todo_items (Parameters sent to LLM)
└─────────────────────────────────────────────────────────────

{ ... JSON content ... }

```

### When to Use

- Inspectable data (schema, config) shown before execution
- Summary stats (e.g. "Agent completed in N iteration(s)")
- Any section that introduces a distinct phase of output

---

## 2. Message / Item Headers

Use `▸` for message or item headers in a sequence.

### Format

```
▸ [n] Role
    content...
```

### Rules

- **Symbol**: `▸` (U+25B8, right-pointing triangle)
- **Index**: 1-based `[n]` for ordering
- **Label**: Role or item type (Human, AI, Tool, etc.)
- **Indent**: 4 spaces for content under the header

### Example

```
▸ [1] Human
    Extract todo items from the email below...

▸ [2] AI
    I'll help you extract...
    → tool: get_current_date_time({"input": "..."})

▸ [3] Tool
    ← get_current_date_time: 2026-03-15T21:39:00+08:00
```

---

## 3. Lists and Items

### Unordered list: `•`

```
  Saved:
    • Item one (priority: high due 2026-03-14)
    • Item two (priority: normal)
```

- **Bullet**: `•` (U+2022)
- **Indent**: 4 spaces for list container, 4 more for items

### Numbered list: `n.`

```
  📋 Todo items:
    1. Review API documentation
       Located in /docs/api-v2
       due: 2026-03-14
    2. Coordinate with DevOps
```

- **Index**: `n.` with space after
- **Sub-content**: 7 spaces (aligned under text, not number)

---

## 4. Tool Call Indicators

### Outgoing (AI → tool)

```
    → tool: tool_name({"arg": "value"})
```

- **Symbol**: `→` (arrow)
- **Format**: `tool: name(args)`
- **Truncation**: If args > 100 chars, truncate with `...`

### Incoming (tool → AI)

```
    ← tool_name: result string
```

- **Symbol**: `←` (arrow)
- **Format**: `name: content`

---

## 5. Event / Status Symbols (Listenable / Graph demos)

| Symbol | Meaning      | Example                          |
|--------|--------------|----------------------------------|
| 🚀     | Chain start  | `[17:42:23] 🚀 Chain started`   |
| 🏁     | Chain end    | `[17:42:37] 🏁 Chain ended`      |
| ▶️     | Node started | `[17:42:23] ▶️  Node 'X' started` |
| ✅     | Node done    | `[17:42:27] ✅ Node 'X' completed` |
| ❌     | Node failed  | `[17:42:27] ❌ Node 'X' failed`  |

- **Timestamp**: `15:04:05.000` (with milliseconds)
- **Spacing**: One space between timestamp and symbol, two before node name (for alignment)

---

## 6. Truncation

| Content type | Max length | Suffix |
|--------------|------------|--------|
| Message text | 200 chars  | `...`  |
| Tool args    | 100 chars  | `...`  |

Apply truncation only when displaying; do not truncate when logging or persisting.

---

## 7. Indentation

| Level | Spaces | Use case                    |
|-------|--------|-----------------------------|
| 0     | 0      | Section headers, `▸` lines |
| 1     | 4      | Content under header        |
| 2     | 7–8    | Nested content (list items) |
| 3     | 10+    | Deep nesting (rare)         |

Use spaces, not tabs.

---

## 8. Spacing

- **Between sections**: One blank line
- **Between messages**: One blank line after each message block
- **Before first output**: One blank line after program start (optional, for clarity)

---

## 9. JSON / Structured Data

- **Indent**: 2 spaces (`json.MarshalIndent(..., "", "  ")`)
- **Placement**: After a section block header, or under an indented content block
- **No extra box** around JSON; the section header provides context

---

## 10. Reference Implementation

See also `docs/demo-comment-style-guide.md` for comment conventions.

See `examples/tool_with_schema/main.go` for:

- Section blocks (schema, agent completed)
- Message printing (`printAgentResponse`)
- Tool output formatting (`SaveTodoItemsTool.Call`)

See `examples/listenable_graph/main.go` for:

- Event logging (EventLogger)
- Todo list formatting (TodoItemReporter)

---

## Quick Reference

```
┌─────────────────────────────────────────────────────────────
│ Section title
└─────────────────────────────────────────────────────────────

▸ [1] Human
    content

    → tool: name(args)
    ← name: result

  List:
    • item one
    • item two
```
