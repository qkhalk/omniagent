package store

import (
	"context"
	"time"
)

// Session is a conversation between a user and an agent.
type Session struct {
	ID        string
	UserToken string
	Channel   string // "web" | "telegram"
	RemoteID  string
	Agent     string
	Project   string
	Title     string
	CreatedAt int64
	UpdatedAt int64
	Closed    bool
}

// Message is a single message in a session.
type Message struct {
	ID        int64
	SessionID string
	Role      string // user | assistant | tool
	Content   string
	ToolName  string
	CreatedAt int64
}

// CreateSession inserts a new session row.
func (s *Store) CreateSession(ctx context.Context, sess *Session) error {
	sess.CreatedAt = nowUnix()
	sess.UpdatedAt = sess.CreatedAt
	_, err := s.DB.ExecContext(ctx,
		`INSERT INTO sessions(id, user_token, channel, remote_id, agent, project, title, created_at, updated_at, closed)
		 VALUES(?,?,?,?,?,?,?,?,?,0)`,
		sess.ID, sess.UserToken, sess.Channel, sess.RemoteID, sess.Agent, sess.Project,
		sess.Title, sess.CreatedAt, sess.UpdatedAt)
	return err
}

// TouchSession updates the updated_at timestamp and optional title.
func (s *Store) TouchSession(ctx context.Context, id, title string) error {
	now := nowUnix()
	_, err := s.DB.ExecContext(ctx,
		`UPDATE sessions SET updated_at=?, title=CASE WHEN ?='' THEN title ELSE ? END WHERE id=?`,
		now, title, title, id)
	return err
}

// CloseSession marks a session as closed.
func (s *Store) CloseSession(ctx context.Context, id string) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE sessions SET closed=1, updated_at=? WHERE id=?`, nowUnix(), id)
	return err
}

// GetSession returns a session by id.
func (s *Store) GetSession(ctx context.Context, id string) (*Session, error) {
	row := s.DB.QueryRowContext(ctx,
		`SELECT id, user_token, channel, remote_id, agent, project, title, created_at, updated_at, closed
		 FROM sessions WHERE id=?`, id)
	var sess Session
	var closed int
	if err := row.Scan(&sess.ID, &sess.UserToken, &sess.Channel, &sess.RemoteID, &sess.Agent,
		&sess.Project, &sess.Title, &sess.CreatedAt, &sess.UpdatedAt, &closed); err != nil {
		return nil, err
	}
	sess.Closed = closed == 1
	return &sess, nil
}

// ListSessions returns open sessions for a user ordered by most-recent.
func (s *Store) ListSessions(ctx context.Context, userToken string) ([]*Session, error) {
	rows, err := s.DB.QueryContext(ctx,
		`SELECT id, user_token, channel, remote_id, agent, project, title, created_at, updated_at, closed
		 FROM sessions WHERE user_token=? ORDER BY updated_at DESC LIMIT 100`, userToken)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Session
	for rows.Next() {
		var sess Session
		var closed int
		if err := rows.Scan(&sess.ID, &sess.UserToken, &sess.Channel, &sess.RemoteID, &sess.Agent,
			&sess.Project, &sess.Title, &sess.CreatedAt, &sess.UpdatedAt, &closed); err != nil {
			return nil, err
		}
		sess.Closed = closed == 1
		out = append(out, &sess)
	}
	return out, rows.Err()
}

// AppendMessage inserts a message row.
func (s *Store) AppendMessage(ctx context.Context, m *Message) error {
	m.CreatedAt = nowUnix()
	res, err := s.DB.ExecContext(ctx,
		`INSERT INTO messages(session_id, role, content, tool_name, created_at) VALUES(?,?,?,?,?)`,
		m.SessionID, m.Role, m.Content, m.ToolName, m.CreatedAt)
	if err != nil {
		return err
	}
	m.ID, _ = res.LastInsertId()
	_, err = s.DB.ExecContext(ctx, `UPDATE sessions SET updated_at=? WHERE id=?`, m.CreatedAt, m.SessionID)
	return err
}

// ListMessages returns messages for a session ordered oldest-first.
func (s *Store) ListMessages(ctx context.Context, sessionID string, limit int) ([]*Message, error) {
	if limit <= 0 || limit > 1000 {
		limit = 500
	}
	rows, err := s.DB.QueryContext(ctx,
		`SELECT id, session_id, role, content, tool_name, created_at
		 FROM messages WHERE session_id=? ORDER BY id DESC LIMIT ?`, sessionID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Message
	for rows.Next() {
		var m Message
		if err := rows.Scan(&m.ID, &m.SessionID, &m.Role, &m.Content, &m.ToolName, &m.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, &m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// reverse
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out, nil
}

// AddAudit records an audit entry.
func (s *Store) AddAudit(ctx context.Context, userToken, channel, remoteID, action, detail string, ok bool) error {
	flag := 0
	if ok {
		flag = 1
	}
	_, err := s.DB.ExecContext(ctx,
		`INSERT INTO audit(ts, user_token, channel, remote_id, action, detail, ok)
		 VALUES(?,?,?,?,?,?,?)`,
		nowUnix(), userToken, channel, remoteID, action, detail, flag)
	return err
}

// AddUsage records a usage entry.
func (s *Store) AddUsage(ctx context.Context, userToken, agent, sessionID string, tokensIn, tokensOut, durationMs int) error {
	_, err := s.DB.ExecContext(ctx,
		`INSERT INTO usage(ts, user_token, agent, session_id, tokens_in, tokens_out, duration_ms)
		 VALUES(?,?,?,?,?,?,?)`,
		nowUnix(), userToken, agent, sessionID, tokensIn, tokensOut, durationMs)
	return err
}

// BanEntry is a row in the ban table.
type BanEntry struct {
	Key         string
	Fails       int
	FirstFailAt int64
	BannedUntil int64
}

// GetBan returns the ban row for a key (ip or tg:user_id).
func (s *Store) GetBan(ctx context.Context, key string) (*BanEntry, error) {
	row := s.DB.QueryRowContext(ctx,
		`SELECT key, fails, first_fail_at, banned_until FROM ban WHERE key=?`, key)
	var b BanEntry
	if err := row.Scan(&b.Key, &b.Fails, &b.FirstFailAt, &b.BannedUntil); err != nil {
		return nil, err
	}
	return &b, nil
}

// UpsertBan updates fail counters and ban window.
func (s *Store) UpsertBan(ctx context.Context, key string, maxFails, windowSecs, banSecs int) (*BanEntry, error) {
	now := nowUnix()
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var b BanEntry
	err = tx.QueryRow(`SELECT key, fails, first_fail_at, banned_until FROM ban WHERE key=?`, key).
		Scan(&b.Key, &b.Fails, &b.FirstFailAt, &b.BannedUntil)
	if err != nil {
		if err.Error() == "sql: no rows in result set" {
			b = BanEntry{Key: key, Fails: 1, FirstFailAt: now}
		} else {
			return nil, err
		}
	} else {
		// window reset
		if now-b.FirstFailAt > int64(windowSecs) {
			b.Fails = 0
			b.FirstFailAt = now
		}
		b.Fails++
	}
	if b.Fails >= maxFails {
		b.BannedUntil = now + int64(banSecs)
	}
	if _, err := tx.Exec(
		`INSERT INTO ban(key, fails, first_fail_at, banned_until) VALUES(?,?,?,?)
		 ON CONFLICT(key) DO UPDATE SET fails=excluded.fails, first_fail_at=excluded.first_fail_at, banned_until=excluded.banned_until`,
		b.Key, b.Fails, b.FirstFailAt, b.BannedUntil); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &b, nil
}

// ClearBan resets ban counter (used by admin unban).
func (s *Store) ClearBan(ctx context.Context, key string) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM ban WHERE key=?`, key)
	return err
}

// ListBans returns all currently-banned keys plus any key with fails>0.
func (s *Store) ListBans(ctx context.Context) ([]*BanEntry, error) {
	now := nowUnix()
	rows, err := s.DB.QueryContext(ctx,
		`SELECT key, fails, first_fail_at, banned_until FROM ban
		 WHERE banned_until>? OR fails>0 ORDER BY banned_until DESC LIMIT 500`, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*BanEntry
	for rows.Next() {
		var b BanEntry
		if err := rows.Scan(&b.Key, &b.Fails, &b.FirstFailAt, &b.BannedUntil); err != nil {
			return nil, err
		}
		out = append(out, &b)
	}
	return out, rows.Err()
}

// ListProjects returns distinct project paths used by a user.
func (s *Store) ListProjects(ctx context.Context, userToken string) ([]string, error) {
	rows, err := s.DB.QueryContext(ctx,
		`SELECT DISTINCT project FROM sessions WHERE user_token=? AND project<>'' ORDER BY project`, userToken)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// touchTime helper, exported as TouchTime so other packages can call consistently.
func TouchTime(t time.Time) int64 { return t.Unix() }
