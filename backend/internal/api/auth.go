package api

import (
        "net/http"

        "github.com/omniagent/omniagent/internal/store"
)

// handleLogin verifies token + Turnstile and sets an HttpOnly cookie.
func (d *Deps) handleLogin(w http.ResponseWriter, r *http.Request) {
        var req struct {
                Token           string `json:"token"`
                TurnstileToken  string `json:"turnstile_token"`
        }
        if err := decodeJSON(r, &req); err != nil {
                writeJSON(w, 400, map[string]any{"error": "invalid body: " + err.Error()})
                return
        }
        ip := clientIP(r)
        p, err := d.Auth.Verify(r.Context(), req.Token, req.TurnstileToken, ip)
        if err != nil {
                _ = d.Store.AddAudit(r.Context(), "", "web", ip, "login", err.Error(), false)
                writeJSON(w, 401, map[string]any{"error": err.Error()})
                return
        }
        // Cookie: signed HMAC of token (we use CookieSecret as a salt).
        cookie := signCookie(p.Token, d.Cfg.CookieSecret)
        http.SetCookie(w, &http.Cookie{
                Name:     "omni_token",
                Value:    cookie,
                Path:     "/",
                HttpOnly: true,
                SameSite: http.SameSiteLaxMode,
                Secure:   r.TLS != nil,
                MaxAge:   7 * 24 * 3600,
        })
        _ = d.Store.AddAudit(r.Context(), p.Token, "web", ip, "login", "", true)
        writeJSON(w, 200, map[string]any{"label": p.Label, "token": p.Token})
}

// handleLogout clears the cookie.
func (d *Deps) handleLogout(w http.ResponseWriter, r *http.Request) {
        http.SetCookie(w, &http.Cookie{Name: "omni_token", Value: "", Path: "/", MaxAge: -1})
        writeJSON(w, 200, map[string]any{"ok": true})
}

// handleMe returns the current principal's label.
func (d *Deps) handleMe(w http.ResponseWriter, r *http.Request) {
        p := principal(r)
        if p == nil {
                writeJSON(w, 401, map[string]any{"error": "unauthorized"})
                return
        }
        writeJSON(w, 200, map[string]any{"label": p.Label, "token": p.Token})
}

// handleListSessions returns open sessions for the current user.
func (d *Deps) handleListSessions(w http.ResponseWriter, r *http.Request) {
        p := principal(r)
        sessions, err := d.Store.ListSessions(r.Context(), p.Token)
        if err != nil {
                writeJSON(w, 500, map[string]any{"error": err.Error()})
                return
        }
        out := make([]map[string]any, 0, len(sessions))
        for _, s := range sessions {
                out = append(out, sessionToJSON(s))
        }
        writeJSON(w, 200, out)
}

// handleCreateSession creates a new session.
func (d *Deps) handleCreateSession(w http.ResponseWriter, r *http.Request) {
        p := principal(r)
        var req struct {
                Agent   string `json:"agent"`
                Project string `json:"project"`
                Title   string `json:"title"`
        }
        if err := decodeJSON(r, &req); err != nil {
                writeJSON(w, 400, map[string]any{"error": err.Error()})
                return
        }
        if _, ok := d.lookupAdapter(req.Agent); !ok {
                writeJSON(w, 400, map[string]any{"error": "unknown agent"})
                return
        }
        sess := &store.Session{
                ID:        newID(),
                UserToken: p.Token,
                Channel:   "web",
                RemoteID:  p.IP,
                Agent:     req.Agent,
                Project:   req.Project,
                Title:     req.Title,
        }
        if err := d.Store.CreateSession(r.Context(), sess); err != nil {
                writeJSON(w, 500, map[string]any{"error": err.Error()})
                return
        }
        _ = d.Store.AddAudit(r.Context(), p.Token, "web", p.IP, "session.create", sess.ID, true)
        writeJSON(w, 201, sessionToJSON(sess))
}

// handleGetSession returns a session by id.
func (d *Deps) handleGetSession(w http.ResponseWriter, r *http.Request) {
        p := principal(r)
        s, err := d.Store.GetSession(r.Context(), chiURLParam(r, "id"))
        if err != nil || s == nil || s.UserToken != p.Token {
                writeJSON(w, 404, map[string]any{"error": "not found"})
                return
        }
        writeJSON(w, 200, sessionToJSON(s))
}

// handleCloseSession marks the session closed.
func (d *Deps) handleCloseSession(w http.ResponseWriter, r *http.Request) {
        p := principal(r)
        id := chiURLParam(r, "id")
        s, err := d.Store.GetSession(r.Context(), id)
        if err != nil || s == nil || s.UserToken != p.Token {
                writeJSON(w, 404, map[string]any{"error": "not found"})
                return
        }
        _ = d.Sessions.Cancel(id)
        _ = d.Store.CloseSession(r.Context(), id)
        writeJSON(w, 200, map[string]any{"ok": true})
}

// handleListMessages returns messages for a session.
func (d *Deps) handleListMessages(w http.ResponseWriter, r *http.Request) {
        p := principal(r)
        id := chiURLParam(r, "id")
        s, err := d.Store.GetSession(r.Context(), id)
        if err != nil || s == nil || s.UserToken != p.Token {
                writeJSON(w, 404, map[string]any{"error": "not found"})
                return
        }
        msgs, err := d.Store.ListMessages(r.Context(), id, 500)
        if err != nil {
                writeJSON(w, 500, map[string]any{"error": err.Error()})
                return
        }
        out := make([]map[string]any, 0, len(msgs))
        for _, m := range msgs {
                out = append(out, map[string]any{
                        "id":         m.ID,
                        "role":       m.Role,
                        "content":    m.Content,
                        "tool_name":  m.ToolName,
                        "created_at": m.CreatedAt,
                })
        }
        writeJSON(w, 200, out)
}
