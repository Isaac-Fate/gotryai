// Package blogcomposer implements a LangGraph-style pipeline that turns rough notes into a cited technical blog post.
//
// Layout:
//   - graph.go — NewGraph(llm, llmStructured, webSearch, extraTools, opts...) wires the pipeline
//   - options.go — Options + Option (functional options for NewGraph)
//   - state.go, schema.go — graph state and LLM JSON DTOs
//   - nodes.go — pipeline steps (analyze, research, blueprint, draft, assemble, save)
//   - publish.go — final LLM polish (monolith or chunked)
//   - section_agent.go — per-section ReAct writer
//   - prompts.go — prompt strings and section prompt builders
//   - knowledge.go — KB formatting, prefetch, session corpus, publish chunk context
//   - util.go — draft parsing, string helpers, markdown assembly, fence counting
//   - section_store*.go — SectionDraftStore (SQLite / Postgres constructors)
//   - debug_section.go — BLOG_COMPOSER_DEBUG_SECTION diagnostics
package blogcomposer
