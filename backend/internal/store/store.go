// Package store is a thin SQLite persistence layer for OmniAgent.
package store

import (
	"database/sql"
	"errors"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// Store wraps a *sql.DB.
type Store struct {
	DB *sql.DB
}

// Open opens (and migrates) the SQLite database at path.
func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite3", path+"?_busy_timeout=5000&_journal_mode=WAL&_foreign_keys=on")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(8)
	db.SetMaxIdleConns(4)
	s := &Store{DB: db}
	if err := s.migrate(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) migrate() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS sessions (
			id            TEXT PRIMARY KEY,
			user_token    TEXT NOT NULL,
			channel       TEXT NOT NULL,           -- 'web' | 'telegram'
			remote_id     TEXT NOT NULL,           -- user id in channel
			agent         TEXT NOT NULL,           -- 'claude' | 'codex' | ...
			project       TEXT NOT NULL,           -- relative path under workspace
			title         TEXT NOT NULL DEFAULT '',
			created_at    INTEGER NOT NULL,
			updated_at    INTEGER NOT NULL,
			closed        INTEGER NOT NULL DEFAULT 0
		);`,
		`CREATE INDEX IF NOT EXISTS idx_sessions_user ON sessions(user_token);`,
		`CREATE INDEX IF NOT EXISTS idx_sessions_remote ON sessions(channel, remote_id);`,

		`CREATE TABLE IF NOT EXISTS messages (
			id            INTEGER PRIMARY KEY AUTOINCREMENT,
			session_id    TEXT NOT NULL,
			role          TEXT NOT NULL,           -- 'user' | 'assistant' | 'tool'
			content       TEXT NOT NULL,
			tool_name     TEXT NOT NULL DEFAULT '',
			created_at    INTEGER NOT NULL,
			FOREIGN KEY(session_id) REFERENCES sessions(id) ON DELETE CASCADE
		);`,
		`CREATE INDEX IF NOT EXISTS idx_messages_session ON messages(session_id, created_at);`,

		`CREATE TABLE IF NOT EXISTS ban (
			key           TEXT PRIMARY KEY,        -- ip or tg:user_id
			fails         INTEGER NOT NULL DEFAULT 0,
			first_fail_at INTEGER NOT NULL DEFAULT 0,
			banned_until  INTEGER NOT NULL DEFAULT 0
		);`,

		`CREATE TABLE IF NOT EXISTS audit (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			ts         INTEGER NOT NULL,
			user_token TEXT NOT NULL DEFAULT '',
			channel    TEXT NOT NULL DEFAULT '',
			remote_id  TEXT NOT NULL DEFAULT '',
			action     TEXT NOT NULL,
			detail     TEXT NOT NULL DEFAULT '',
			ok         INTEGER NOT NULL DEFAULT 0
		);`,
		`CREATE INDEX IF NOT EXISTS idx_audit_ts ON audit(ts);`,

		`CREATE TABLE IF NOT EXISTS usage (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			ts         INTEGER NOT NULL,
			user_token TEXT NOT NULL,
			agent      TEXT NOT NULL,
			session_id TEXT NOT NULL,
			tokens_in  INTEGER NOT NULL DEFAULT 0,
			tokens_out INTEGER NOT NULL DEFAULT 0,
			duration_ms INTEGER NOT NULL DEFAULT 0
		);`,
		`CREATE INDEX IF NOT EXISTS idx_usage_ts ON usage(ts);`,
	}
	for _, q := range stmts {
		if _, err := s.DB.Exec(q); err != nil {
			return errors.New("migrate: " + err.Error() + " (stmt: " + q + ")")
		}
	}
	return nil
}

// Close closes the database.
func (s *Store) Close() error { return s.DB.Close() }

// nowUnix returns the current time as unix seconds.
func nowUnix() int64 { return time.Now().Unix() }
