package logdb

import (
	"context"
	"database/sql"
	"errors"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

type Store struct {
	db *sql.DB
}

type Entry struct {
	Timestamp   time.Time
	ClientIP    string
	Repo        string
	RequestPath string
	Upstream    string
	Method      string
	Bytes       int64
	Duration    time.Duration
}

type UsageSummary struct {
	TotalCount int64
	TotalBytes int64
}

func Open(path string) (*Store, error) {
	if path == "" {
		return nil, errors.New("logdb: path is required")
	}
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return nil, err
	}
	if err := migrate(db); err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *Store) RecordUsage(ctx context.Context, e Entry) error {
	if s == nil || s.db == nil {
		return nil
	}
	if e.Timestamp.IsZero() {
		e.Timestamp = time.Now()
	}
	const stmt = `
INSERT INTO usage_logs (
	created_at, client_ip, repo, request_path, upstream, method, bytes, duration_ms
) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`
	_, err := s.db.ExecContext(ctx, stmt,
		e.Timestamp.UTC(),
		e.ClientIP,
		e.Repo,
		e.RequestPath,
		e.Upstream,
		e.Method,
		e.Bytes,
		e.Duration.Milliseconds(),
	)
	return err
}

func (s *Store) UsageSummary(ctx context.Context) (UsageSummary, error) {
	var summary UsageSummary
	if s == nil || s.db == nil {
		return summary, nil
	}
	const stmt = `
SELECT
	COUNT(*) AS total_count,
	COALESCE(SUM(bytes), 0) AS total_bytes
FROM usage_logs
WHERE repo <> ''`
	if err := s.db.QueryRowContext(ctx, stmt).Scan(&summary.TotalCount, &summary.TotalBytes); err != nil {
		return UsageSummary{}, err
	}
	return summary, nil
}

func migrate(db *sql.DB) error {
	const table = `
CREATE TABLE IF NOT EXISTS usage_logs (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	created_at TIMESTAMP NOT NULL,
	client_ip TEXT,
	repo TEXT,
	request_path TEXT,
	upstream TEXT,
	method TEXT,
	bytes INTEGER,
	duration_ms INTEGER
)`
	if _, err := db.Exec(table); err != nil {
		return err
	}
	const idxRepo = `CREATE INDEX IF NOT EXISTS idx_usage_logs_repo ON usage_logs(repo)`
	if _, err := db.Exec(idxRepo); err != nil {
		return err
	}
	const idxCreated = `CREATE INDEX IF NOT EXISTS idx_usage_logs_created_at ON usage_logs(created_at)`
	if _, err := db.Exec(idxCreated); err != nil {
		return err
	}
	return nil
}
