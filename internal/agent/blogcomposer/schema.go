package blogcomposer

// LLM JSON response shapes (structured output). Kept separate from state.go for readability.

type draftAnalysisJSON struct {
	PostType  string `json:"post_type"`
	Audience  string `json:"audience"`
	CoreClaim string `json:"core_claim"`
}

type searchQueriesJSON struct {
	Queries []string `json:"queries"`
}

type knowledgeBaseJSON struct {
	Overview         string                `json:"overview"`
	EvidenceItems    []evidenceItemJSON    `json:"evidence_items"`
	KeyResources     []keyResourceJSON     `json:"key_resources"`
	VerbatimSnippets []verbatimSnippetJSON `json:"verbatim_snippets"`
}

type verbatimSnippetJSON struct {
	Language  string `json:"language"`
	Snippet   string `json:"snippet"`
	SourceURL string `json:"source_url"`
	Note      string `json:"note,omitempty"`
}

type evidenceItemJSON struct {
	Fact string `json:"fact"`
	URL  string `json:"url,omitempty"`
}

type keyResourceJSON struct {
	Title string `json:"title"`
	URL   string `json:"url"`
	Note  string `json:"note,omitempty"`
}

type blueprintJSON struct {
	Title        string        `json:"title"`
	Thesis       string        `json:"thesis"`
	Audience     string        `json:"audience,omitempty"`
	PostType     string        `json:"post_type"`
	NarrativeArc string        `json:"narrative_arc"`
	Sections     []SectionSpec `json:"sections"`
}
