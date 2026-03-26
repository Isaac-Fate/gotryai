package blogcomposer

// State is the single source of truth for the blog composing graph.
// See package doc for pipeline overview.
type State struct {
	RunID             string             `json:"run_id,omitempty"`
	Draft             string             `json:"draft"`
	DraftMeta         DraftMeta          `json:"draft_meta,omitempty"`
	KnowledgeBase     string             `json:"knowledge_base,omitempty"`
	KnowledgeVerbatim string             `json:"knowledge_verbatim,omitempty"`
	SessionToolCorpus string             `json:"session_tool_corpus,omitempty"`
	PrefetchedSources []PrefetchedSource `json:"prefetched_sources,omitempty"`
	Blueprint         Blueprint          `json:"blueprint,omitempty"`
	Sections          []SectionDraft     `json:"sections,omitempty"`
	CurrentIndex      int                `json:"current_index"`
	FinalPost         string             `json:"final_post,omitempty"`
	SavedPath         string             `json:"saved_path,omitempty"`
}

type PrefetchedSource struct {
	URL  string `json:"url"`
	Body string `json:"body"`
}

type DraftMeta struct {
	Voice        string   `json:"voice,omitempty"`
	Style        string   `json:"style,omitempty"`
	MustInclude  []string `json:"must_include,omitempty"`
	FreeformBody string   `json:"freeform_body"`
	PostType     string   `json:"post_type,omitempty"`
	Audience     string   `json:"audience,omitempty"`
	CoreClaim    string   `json:"core_claim,omitempty"`
}

type Blueprint struct {
	Title        string        `json:"title"`
	Thesis       string        `json:"thesis"`
	Audience     string        `json:"audience,omitempty"`
	PostType     string        `json:"post_type"`
	NarrativeArc string        `json:"narrative_arc"`
	Sections     []SectionSpec `json:"sections"`
}

type SectionSpec struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Type        string   `json:"type"`
	Purpose     string   `json:"purpose"`
	Artifacts   []string `json:"artifacts,omitempty"`
	SearchHints []string `json:"search_hints,omitempty"`
	CiteURLs    []string `json:"cite_urls,omitempty"`
}

type SectionDraft struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Markdown string `json:"markdown"`
}
