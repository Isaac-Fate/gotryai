package blogcomposer

import (
	"context"
	"database/sql"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// OpenPostgresSectionDraftStore connects with database/sql using the pgx driver
// (libpq-style DSN, e.g. postgres://user:pass@host:5432/dbname?sslmode=disable).
func OpenPostgresSectionDraftStore(dsn string) (SectionDraftStore, error) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, err
	}
	st := &postgresSectionDraftStore{db: db}
	if err := st.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return st, nil
}

type postgresSectionDraftStore struct {
	db *sql.DB
}

func (s *postgresSectionDraftStore) migrate() error {
	_, err := s.db.Exec(`
CREATE TABLE IF NOT EXISTS blog_section (
	id BIGSERIAL PRIMARY KEY,
	run_id TEXT NOT NULL,
	section_index INTEGER NOT NULL,
	section_id TEXT,
	title TEXT,
	markdown TEXT NOT NULL,
	created_at BIGINT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_blog_section_run ON blog_section(run_id, section_index);
`)
	return err
}

func (s *postgresSectionDraftStore) PersistSectionDraft(ctx context.Context, runID string, sectionIndex int, spec SectionSpec, markdown string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO blog_section(run_id, section_index, section_id, title, markdown, created_at) VALUES($1,$2,$3,$4,$5,$6)`,
		runID, sectionIndex, spec.ID, spec.Title, markdown, time.Now().Unix(),
	)
	return err
}

// Close releases the pool handle.
func (s *postgresSectionDraftStore) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

var _ SectionDraftStore = (*postgresSectionDraftStore)(nil)
