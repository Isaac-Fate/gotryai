package blogcomposer

// State is the single source of truth for the blog composing graph.
//
// Pipeline: analyze_draft → research → design_blueprint → [write_rich_section]* →
// assemble → voice_pass → final_edit → save
// (research builds KB URLs; blueprint assigns cite_urls per section; writers + final_edit weave links.)
type State struct {
	Draft         string         `json:"draft"`
	DraftMeta     DraftMeta      `json:"draft_meta,omitempty"`
	KnowledgeBase string         `json:"knowledge_base,omitempty"`
	Blueprint     Blueprint      `json:"blueprint,omitempty"`
	Sections      []SectionDraft `json:"sections,omitempty"`
	CurrentIndex  int            `json:"current_index"`
	FinalPost     string         `json:"final_post,omitempty"`
	SavedPath     string         `json:"saved_path,omitempty"`
}

// DraftMeta holds optional hints parsed from YAML frontmatter plus LLM-detected analysis.
type DraftMeta struct {
	Voice        string   `json:"voice,omitempty"`
	Style        string   `json:"style,omitempty"`
	MustInclude  []string `json:"must_include,omitempty"`
	FreeformBody string   `json:"freeform_body"`

	PostType  string `json:"post_type,omitempty"`
	Audience  string `json:"audience,omitempty"`
	CoreClaim string `json:"core_claim,omitempty"`
}

// Blueprint is the creative structure produced by design_blueprint.
// After design_blueprint, downstream nodes treat Blueprint fields as canonical.
type Blueprint struct {
	Title        string        `json:"title"`
	Thesis       string        `json:"thesis"`
	Audience     string        `json:"audience,omitempty"`
	PostType     string        `json:"post_type"`
	NarrativeArc string        `json:"narrative_arc"`
	Sections     []SectionSpec `json:"sections"`
}

// SectionSpec describes one planned section — its type drives the writing prompt.
type SectionSpec struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Type        string   `json:"type"`
	Purpose     string   `json:"purpose"`
	Artifacts   []string `json:"artifacts,omitempty"`
	SearchHints []string `json:"search_hints,omitempty"`
	// CiteURLs are URLs copied from the knowledge base; writers must weave them as inline [text](url).
	CiteURLs []string `json:"cite_urls,omitempty"`
}

// SectionDraft is the rendered markdown for one section.
type SectionDraft struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Markdown string `json:"markdown"`
}

// --- JSON shapes for structured LLM responses (unexported) ---

type draftAnalysisJSON struct {
	PostType  string `json:"post_type"`
	Audience  string `json:"audience"`
	CoreClaim string `json:"core_claim"`
}

type searchQueriesJSON struct {
	Queries []string `json:"queries"`
}

type knowledgeBaseJSON struct {
	Overview      string             `json:"overview"`
	EvidenceItems []evidenceItemJSON `json:"evidence_items"`
	KeyResources  []keyResourceJSON  `json:"key_resources"`
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
