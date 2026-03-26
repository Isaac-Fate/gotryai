package blogcomposer

import "context"

// SectionDraftStore persists each section markdown after draft_section completes
// (durability, diff/rebuild, analytics). Implementations: SQLite, Postgres, or custom.
type SectionDraftStore interface {
	PersistSectionDraft(ctx context.Context, runID string, sectionIndex int, spec SectionSpec, markdown string) error
}

// CloseSectionDraftStore closes stores returned by OpenSQLiteSectionDraftStore or OpenPostgresSectionDraftStore.
// Other implementations are no-ops.
func CloseSectionDraftStore(s SectionDraftStore) error {
	if s == nil {
		return nil
	}
	type closer interface {
		Close() error
	}
	if c, ok := s.(closer); ok {
		return c.Close()
	}
	return nil
}
