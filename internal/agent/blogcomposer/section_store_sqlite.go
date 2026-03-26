package blogcomposer

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

// OpenSQLiteSectionDraftStore opens a file-backed SQLite store and ensures schema exists.
func OpenSQLiteSectionDraftStore(path string) (SectionDraftStore, error) {
	path = filepath.Clean(path)
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}
	}
	dsn := "file:" + path + "?_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, err
	}
	st := &sqliteSectionDraftStore{db: db}
	if err := st.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return st, nil
}

type sqliteSectionDraftStore struct {
	db *sql.DB
}

func (s *sqliteSectionDraftStore) migrate() error {
	_, err := s.db.Exec(`
CREATE TABLE IF NOT EXISTS blog_section (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	run_id TEXT NOT NULL,
	section_index INTEGER NOT NULL,
	section_id TEXT,
	title TEXT,
	markdown TEXT NOT NULL,
	created_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_blog_section_run ON blog_section(run_id, section_index);
`)
	return err
}

func (s *sqliteSectionDraftStore) PersistSectionDraft(ctx context.Context, runID string, sectionIndex int, spec SectionSpec, markdown string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO blog_section(run_id, section_index, section_id, title, markdown, created_at) VALUES(?,?,?,?,?,?)`,
		runID, sectionIndex, spec.ID, spec.Title, markdown, time.Now().Unix(),
	)
	return err
}

// Close releases the DB handle.
func (s *sqliteSectionDraftStore) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

var _ SectionDraftStore = (*sqliteSectionDraftStore)(nil)
