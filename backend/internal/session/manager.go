// Package session coordinates in-memory agent runs and broadcasts events to
// subscribed channels (WebSocket or Telegram bot).
package session

import (
        "context"
        "errors"
        "sync"
        "time"

        "github.com/google/uuid"

        "github.com/omniagent/omniagent/internal/adapter"
        "github.com/omniagent/omniagent/internal/store"
)

// Manager owns active runs and subscribers.
type Manager struct {
        mu        sync.RWMutex
        runs      map[string]*Run           // session id -> active run
        subs      map[string][]*Subscriber  // session id -> subscribers
        store     *store.Store
        workspace string
        agentTimeout time.Duration
}

// Subscriber receives streamed events for one session.
type Subscriber struct {
        ID   string
        Ch   chan adapter.Event
        Done chan struct{}
}

// Run tracks one active agent invocation.
type Run struct {
        SessionID string
        Cancel    context.CancelFunc
        startedAt time.Time
}

// NewManager returns a manager.
func NewManager(st *store.Store, workspace string, agentTimeout time.Duration) *Manager {
        return &Manager{
                runs: make(map[string]*Run),
                subs: make(map[string][]*Subscriber),
                store: st,
                workspace: workspace,
                agentTimeout: agentTimeout,
        }
}

// Subscribe registers a new subscriber for a session.
// Returns the subscriber (already holding any buffered replay) and an
// unsubscribe function.
func (m *Manager) Subscribe(sessionID string) *Subscriber {
        sub := &Subscriber{ID: uuid.NewString(), Ch: make(chan adapter.Event, 32), Done: make(chan struct{})}
        m.mu.Lock()
        m.subs[sessionID] = append(m.subs[sessionID], sub)
        m.mu.Unlock()
        return sub
}

// Unsubscribe removes a subscriber.
func (m *Manager) Unsubscribe(sessionID, subID string) {
        m.mu.Lock()
        defer m.mu.Unlock()
        subs := m.subs[sessionID]
        for i, s := range subs {
                if s.ID == subID {
                        close(s.Done)
                        close(s.Ch)
                        m.subs[sessionID] = append(subs[:i], subs[i+1:]...)
                        break
                }
        }
        if len(m.subs[sessionID]) == 0 {
                delete(m.subs, sessionID)
        }
}

// broadcast sends an event to all subscribers of a session, non-blocking.
func (m *Manager) broadcast(sessionID string, ev adapter.Event) {
        m.mu.RLock()
        subs := m.subs[sessionID]
        m.mu.RUnlock()
        for _, s := range subs {
                select {
                case s.Ch <- ev:
                case <-s.Done:
                default:
                        // drop if subscriber's channel is full
                }
        }
}

// Start launches an adapter run for the session and streams events.
// The caller is responsible for having already created the session row.
func (m *Manager) Start(ctx context.Context, sess *store.Session, ad adapter.Adapter, spec adapter.Spec) error {
        m.mu.Lock()
        if _, exists := m.runs[sess.ID]; exists {
                m.mu.Unlock()
                return ErrAlreadyRunning
        }
        runCtx, cancel := context.WithTimeout(ctx, m.agentTimeout)
        r := &Run{SessionID: sess.ID, Cancel: cancel, startedAt: time.Now()}
        m.runs[sess.ID] = r
        m.mu.Unlock()

        // persist the user prompt
        _ = m.store.AppendMessage(ctx, &store.Message{
                SessionID: sess.ID, Role: "user", Content: spec.Prompt,
        })
        m.broadcast(sess.ID, adapter.Event{Type: "text", Text: spec.Prompt})

        writer := &storeWriter{mgr: m, sessID: sess.ID, store: m.store}
        go func() {
                defer func() {
                        m.mu.Lock()
                        delete(m.runs, sess.ID)
                        m.mu.Unlock()
                }()
                _ = ad.Run(runCtx, spec, writer)
                _ = m.store.TouchSession(ctx, sess.ID, extractTitle(spec.Prompt))
        }()
        return nil
}

// Cancel interrupts an active run.
func (m *Manager) Cancel(sessionID string) bool {
        m.mu.RLock()
        r, ok := m.runs[sessionID]
        m.mu.RUnlock()
        if !ok {
                return false
        }
        r.Cancel()
        return true
}

// IsRunning reports whether a run is currently active for a session.
func (m *Manager) IsRunning(sessionID string) bool {
        m.mu.RLock()
        _, ok := m.runs[sessionID]
        m.mu.RUnlock()
        return ok
}

// ActiveCount returns the number of active runs.
func (m *Manager) ActiveCount() int {
        m.mu.RLock()
        defer m.mu.RUnlock()
        return len(m.runs)
}

// storeWriter implements adapter.EventWriter, broadcasting to subscribers
// while persisting assistant text to the database.
type storeWriter struct {
        mgr    *Manager
        sessID string
        store  *store.Store
        buf    []byte // accumulator for assistant text
}

func (w *storeWriter) WriteEvent(ev adapter.Event) error {
        w.mgr.broadcast(w.sessID, ev)
        switch ev.Type {
        case "text":
                w.buf = append(w.buf, ev.Text...)
                w.buf = append(w.buf, '\n')
        case "tool":
                _ = w.store.AppendMessage(context.Background(), &store.Message{
                        SessionID: w.sessID, Role: "tool", Content: ev.ToolInput, ToolName: ev.ToolName,
                })
        case "error":
                _ = w.store.AddAudit(context.Background(), "", "agent", w.sessID, "error", ev.Text, false)
        case "done":
                if len(w.buf) > 0 {
                        _ = w.store.AppendMessage(context.Background(), &store.Message{
                                SessionID: w.sessID, Role: "assistant", Content: string(w.buf),
                        })
                }
        }
        return nil
}

func (w *storeWriter) Close() error { return nil }

// extractTitle returns the first ~60 chars of the prompt as the session title.
func extractTitle(prompt string) string {
        prompt = trimSpace(prompt)
        if len(prompt) > 60 {
                return prompt[:60] + "…"
        }
        return prompt
}

// trimSpace is a tiny local replacement for strings.TrimSpace so we can keep
// the import list minimal here.
func trimSpace(s string) string {
        for len(s) > 0 && (s[0] == ' ' || s[0] == '\n' || s[0] == '\t') {
                s = s[1:]
        }
        for len(s) > 0 && (s[len(s)-1] == ' ' || s[len(s)-1] == '\n' || s[len(s)-1] == '\t') {
                s = s[:len(s)-1]
        }
        return s
}

// ErrAlreadyRunning is returned when a session already has an active run.
var ErrAlreadyRunning = errors.New("session already running")
