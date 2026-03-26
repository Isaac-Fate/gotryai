package blogcomposer

// Options holds tunable budgets for the blog composer pipeline.
// Pass functional options to NewGraph (see With* helpers).
//
// Pattern: https://dave.cheney.net/2014/10/17/functional-options-for-friendly-apis
type Options struct {
	ResearchHitMaxRunes     int
	RawSearchCapRunes       int
	ResearchSynthMaxRunes   int
	SectionKBMaxRunes       int
	SectionVerbatimMaxRunes int
	BlueprintKBMaxRunes     int
	DesignVerbatimMaxRunes  int
	FinalEditKBMaxRunes     int
	FinalEditVerbatimMax    int
	SessionToolCorpusMax    int
	SectionPrefetchMax      int
	SectionAgentMaxIter     int
	SectionMinBodyRunes     int
	PublishChunkMinRunes    int
	ChunkPriorTailRunes     int
	SectionMissingCodeRetry bool
	UpfrontResearch         bool
	SectionThinRetry        bool
	// SectionDraftStore persists each drafted section; nil skips persistence.
	SectionDraftStore SectionDraftStore
}

// Option mutates Options when building a graph.
type Option func(*Options)

func defaultOptions() Options {
	return Options{
		ResearchHitMaxRunes:     24000,
		RawSearchCapRunes:       120000,
		ResearchSynthMaxRunes:   90000,
		SectionKBMaxRunes:       100000,
		SectionVerbatimMaxRunes: 48000,
		BlueprintKBMaxRunes:     120000,
		DesignVerbatimMaxRunes:  16000,
		FinalEditKBMaxRunes:     100000,
		FinalEditVerbatimMax:    48000,
		SessionToolCorpusMax:    300000,
		SectionPrefetchMax:      100000,
		SectionAgentMaxIter:     26,
		SectionMinBodyRunes:     380,
		PublishChunkMinRunes:    12000,
		ChunkPriorTailRunes:     650,
		SectionMissingCodeRetry: true,
		UpfrontResearch:         false,
		SectionThinRetry:        true,
	}
}

func buildOptions(opts ...Option) Options {
	o := defaultOptions()
	for _, apply := range opts {
		if apply != nil {
			apply(&o)
		}
	}
	return o
}

func WithResearchHitMaxRunes(n int) Option {
	return func(o *Options) { o.ResearchHitMaxRunes = n }
}

func WithRawSearchCapRunes(n int) Option {
	return func(o *Options) { o.RawSearchCapRunes = n }
}

func WithResearchSynthMaxRunes(n int) Option {
	return func(o *Options) { o.ResearchSynthMaxRunes = n }
}

func WithSectionKBMaxRunes(n int) Option {
	return func(o *Options) { o.SectionKBMaxRunes = n }
}

func WithSectionVerbatimMaxRunes(n int) Option {
	return func(o *Options) { o.SectionVerbatimMaxRunes = n }
}

func WithBlueprintKBMaxRunes(n int) Option {
	return func(o *Options) { o.BlueprintKBMaxRunes = n }
}

func WithDesignVerbatimMaxRunes(n int) Option {
	return func(o *Options) { o.DesignVerbatimMaxRunes = n }
}

func WithFinalEditKBMaxRunes(n int) Option {
	return func(o *Options) { o.FinalEditKBMaxRunes = n }
}

func WithFinalEditVerbatimMaxRunes(n int) Option {
	return func(o *Options) { o.FinalEditVerbatimMax = n }
}

func WithSessionToolCorpusMaxRunes(n int) Option {
	return func(o *Options) { o.SessionToolCorpusMax = n }
}

func WithSectionPrefetchMaxRunes(n int) Option {
	return func(o *Options) { o.SectionPrefetchMax = n }
}

func WithSectionAgentMaxIter(n int) Option {
	return func(o *Options) { o.SectionAgentMaxIter = n }
}

func WithSectionMinBodyRunes(n int) Option {
	return func(o *Options) { o.SectionMinBodyRunes = n }
}

func WithPublishChunkMinRunes(n int) Option {
	return func(o *Options) { o.PublishChunkMinRunes = n }
}

func WithChunkPriorTailRunes(n int) Option {
	return func(o *Options) { o.ChunkPriorTailRunes = n }
}

func WithSectionMissingCodeRetry(v bool) Option {
	return func(o *Options) { o.SectionMissingCodeRetry = v }
}

func WithUpfrontResearch(v bool) Option {
	return func(o *Options) { o.UpfrontResearch = v }
}

func WithSectionThinRetry(v bool) Option {
	return func(o *Options) { o.SectionThinRetry = v }
}

// WithSectionDraftStore sets where section drafts are persisted (SQLite, Postgres, or custom).
// Pass nil to disable.
func WithSectionDraftStore(s SectionDraftStore) Option {
	return func(o *Options) { o.SectionDraftStore = s }
}
