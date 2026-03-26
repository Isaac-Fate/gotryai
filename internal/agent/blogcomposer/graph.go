package blogcomposer

import (
	"github.com/smallnest/langgraphgo/graph"
	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/tools"
)

// NewGraph builds the blog composing graph with all nodes and edges wired.
//
// sectionExtraTools is appended after webSearch for the section agent (e.g. Fetch_Page_Text);
// pass nil if none. Use opts to override defaults (see With* in options.go).
//
// Default flow: analyze_draft → design_blueprint → [draft_section]* → assemble_post → publish_polish → save.
// With WithUpfrontResearch(true): analyze_draft → research → design_blueprint → prefetch_sources → draft_section → …
//
// The returned graph is ready for AddGlobalListener / AddNodeListener and CompileListenable().
func NewGraph(
	llm, llmStructured llms.Model,
	webSearch tools.Tool,
	sectionExtraTools []tools.Tool,
	opts ...Option,
) *graph.ListenableStateGraph[State] {
	o := buildOptions(opts...)

	var fetchTool tools.Tool
	for _, t := range sectionExtraTools {
		if t != nil && t.Name() == "Fetch_Page_Text" {
			fetchTool = t
			break
		}
	}
	sectionTools := append([]tools.Tool{webSearch}, sectionExtraTools...)
	g := graph.NewListenableStateGraph[State]()

	g.AddNode("analyze_draft", "Parse draft hints + detect post type",
		analyzeDraftNode(llmStructured))

	g.AddNode("design_blueprint", "Outline: sections, artifacts, cite_urls",
		designBlueprintNode(llmStructured, o))

	g.AddNode("draft_section", "One section via agent (search + optional fetch); append tool corpus",
		writeRichSectionNode(llm, sectionTools, o))

	g.AddNode("assemble_post", "Concatenate sections into full post",
		assembleNode())

	g.AddNode("publish_polish", "Single editorial pass: structure, citations, mermaid, light voice",
		publishPolishNode(llm, o))

	g.AddNode("save", "Persist final post to disk",
		saveNode(o))

	if o.UpfrontResearch {
		g.AddNode("research", "Batch web search + synthesize knowledge base",
			researchNode(llm, llmStructured, webSearch, o))
		g.AddNode("prefetch_sources", "Fetch GitHub file bodies from blueprint cite_urls (blob/raw only)",
			prefetchBlueprintSourcesNode(fetchTool))
		g.AddEdge("analyze_draft", "research")
		g.AddEdge("research", "design_blueprint")
		g.AddEdge("design_blueprint", "prefetch_sources")
		g.AddEdge("prefetch_sources", "draft_section")
	} else {
		g.AddEdge("analyze_draft", "design_blueprint")
		g.AddEdge("design_blueprint", "draft_section")
	}

	g.AddConditionalEdge("draft_section", sectionRouter)
	g.AddEdge("assemble_post", "publish_polish")
	g.AddEdge("publish_polish", "save")
	g.AddEdge("save", graph.END)
	g.SetEntryPoint("analyze_draft")

	return g
}
