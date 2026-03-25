package blogcomposer

import (
	"github.com/smallnest/langgraphgo/graph"
	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/tools"
)

// NewGraph builds the blog composing graph with all nodes and edges wired.
// Research URLs flow into the knowledge base; blueprint sections carry cite_urls;
// section writing and final_edit prompts require inline [text](url) citations from that KB.
// Prompts also enforce codeGroundingRules so fenced code in any language is KB-grounded, not fabricated.
// The returned graph is ready for the caller to add listeners via
// AddGlobalListener / AddNodeListener and then CompileListenable().
func NewGraph(llm, llmStructured llms.Model, webSearch tools.Tool) *graph.ListenableStateGraph[State] {
	g := graph.NewListenableStateGraph[State]()

	g.AddNode("analyze_draft", "Parse draft hints + detect post type",
		analyzeDraftNode(llmStructured))

	g.AddNode("research", "Batch web search + synthesize knowledge base",
		researchNode(llm, llmStructured, webSearch))

	g.AddNode("design_blueprint", "Creative structure with section types + artifacts",
		designBlueprintNode(llmStructured))

	g.AddNode("write_rich_section", "Draft one section with type-specific prompt",
		writeRichSectionNode(llm))

	g.AddNode("assemble", "Concatenate sections into full post",
		assembleNode())

	g.AddNode("voice_pass", "Inject personal voice and transitions",
		voicePassNode(llm))

	g.AddNode("final_edit", "Polish for publication",
		finalEditNode(llm))

	g.AddNode("save", "Persist final post to disk",
		saveNode())

	g.AddEdge("analyze_draft", "research")
	g.AddEdge("research", "design_blueprint")
	g.AddEdge("design_blueprint", "write_rich_section")
	g.AddConditionalEdge("write_rich_section", sectionRouter)
	g.AddEdge("assemble", "voice_pass")
	g.AddEdge("voice_pass", "final_edit")
	g.AddEdge("final_edit", "save")
	g.AddEdge("save", graph.END)
	g.SetEntryPoint("analyze_draft")

	return g
}
