package api

import (
        "net/http"
        "strings"
        "time"

        "github.com/go-chi/chi/v5"

        "github.com/omniagent/omniagent/internal/adapter"
        "github.com/omniagent/omniagent/internal/store"
)

// handleSend kicks off an adapter run for a session.
func (d *Deps) handleSend(w http.ResponseWriter, r *http.Request) {
        p := principal(r)
        id := chiURLParam(r, "id")
        s, err := d.Store.GetSession(r.Context(), id)
        if err != nil || s == nil || s.UserToken != p.Token {
                writeJSON(w, 404, map[string]any{"error": "not found"})
                return
        }

        var req struct {
                Prompt string `json:"prompt"`
                Model  string `json:"model"`
        }
        if err := decodeJSON(r, &req); err != nil {
                writeJSON(w, 400, map[string]any{"error": err.Error()})
                return
        }
        if strings.TrimSpace(req.Prompt) == "" {
                writeJSON(w, 400, map[string]any{"error": "empty prompt"})
                return
        }

        ad, ok := d.lookupAdapter(s.Agent)
        if !ok {
                writeJSON(w, 500, map[string]any{"error": "agent unavailable"})
                return
        }

        projectDir, err := adapter.SafeJoin(d.Cfg.Workspace, s.Project)
        if err != nil {
                writeJSON(w, 400, map[string]any{"error": err.Error()})
                return
        }

        spec := adapter.Spec{
                Prompt:     req.Prompt,
                ProjectDir: projectDir,
                Model:      req.Model,
                Env:        d.Cfg.AdapterEnv,
        }
        if err := d.Sessions.Start(r.Context(), s, ad, spec); err != nil {
                writeJSON(w, 409, map[string]any{"error": err.Error()})
                return
        }
        _ = d.Store.AddAudit(r.Context(), p.Token, "web", p.IP, "session.send", id, true)
        writeJSON(w, 202, map[string]any{"ok": true, "session_id": id})
}

// handleCancel interrupts an active run.
func (d *Deps) handleCancel(w http.ResponseWriter, r *http.Request) {
        p := principal(r)
        id := chiURLParam(r, "id")
        s, err := d.Store.GetSession(r.Context(), id)
        if err != nil || s == nil || s.UserToken != p.Token {
                writeJSON(w, 404, map[string]any{"error": "not found"})
                return
        }
        ok := d.Sessions.Cancel(id)
        writeJSON(w, 200, map[string]any{"cancelled": ok})
}

// handleListProjects returns the list of project subdirs in the workspace plus
// any previously-used project paths.
func (d *Deps) handleListProjects(w http.ResponseWriter, r *http.Request) {
        p := principal(r)
        prev, _ := d.Store.ListProjects(r.Context(), p.Token)
        out := map[string]any{
                "workspace": d.Cfg.Workspace,
                "recent":    prev,
                "dirs":      listSubdirs(d.Cfg.Workspace),
        }
        writeJSON(w, 200, out)
}

// handleFileList returns a directory listing.
func (d *Deps) handleFileList(w http.ResponseWriter, r *http.Request) {
        rel := r.URL.Query().Get("path")
        abs, err := adapter.SafeJoin(d.Cfg.Workspace, rel)
        if err != nil {
                writeJSON(w, 400, map[string]any{"error": err.Error()})
                return
        }
        entries, err := listDir(abs)
        if err != nil {
                writeJSON(w, 500, map[string]any{"error": err.Error()})
                return
        }
        writeJSON(w, 200, map[string]any{"path": rel, "entries": entries})
}

// handleFileRead returns the contents of a file (UTF-8, capped at 256KB).
func (d *Deps) handleFileRead(w http.ResponseWriter, r *http.Request) {
        rel := r.URL.Query().Get("path")
        abs, err := adapter.SafeJoin(d.Cfg.Workspace, rel)
        if err != nil {
                writeJSON(w, 400, map[string]any{"error": err.Error()})
                return
        }
        b, err := readFileCapped(abs, 256*1024)
        if err != nil {
                writeJSON(w, 500, map[string]any{"error": err.Error()})
                return
        }
        writeJSON(w, 200, map[string]any{"path": rel, "content": string(b)})
}

// handleFileWrite writes a file.
func (d *Deps) handleFileWrite(w http.ResponseWriter, r *http.Request) {
        p := principal(r)
        var req struct {
                Path    string `json:"path"`
                Content string `json:"content"`
        }
        if err := decodeJSON(r, &req); err != nil {
                writeJSON(w, 400, map[string]any{"error": err.Error()})
                return
        }
        abs, err := adapter.SafeJoin(d.Cfg.Workspace, req.Path)
        if err != nil {
                writeJSON(w, 400, map[string]any{"error": err.Error()})
                return
        }
        if err := writeFile(abs, []byte(req.Content)); err != nil {
                writeJSON(w, 500, map[string]any{"error": err.Error()})
                return
        }
        _ = d.Store.AddAudit(r.Context(), p.Token, "web", p.IP, "file.write", req.Path, true)
        writeJSON(w, 200, map[string]any{"ok": true})
}

// handleListBans returns the current ban table.
func (d *Deps) handleListBans(w http.ResponseWriter, r *http.Request) {
        bans, err := d.Store.ListBans(r.Context())
        if err != nil {
                writeJSON(w, 500, map[string]any{"error": err.Error()})
                return
        }
        out := make([]map[string]any, 0, len(bans))
        for _, b := range bans {
                out = append(out, map[string]any{
                        "key":          b.Key,
                        "fails":        b.Fails,
                        "first_fail":   b.FirstFailAt,
                        "banned_until": b.BannedUntil,
                        "expired":      b.BannedUntil < time.Now().Unix(),
                })
        }
        writeJSON(w, 200, out)
}

// handleUnban clears a ban entry.
func (d *Deps) handleUnban(w http.ResponseWriter, r *http.Request) {
        key := chiURLParam(r, "key")
        if err := d.Auth.ClearBan(r.Context(), key); err != nil {
                writeJSON(w, 500, map[string]any{"error": err.Error()})
                return
        }
        writeJSON(w, 200, map[string]any{"ok": true})
}

// handleListAudit returns the latest audit log entries.
func (d *Deps) handleListAudit(w http.ResponseWriter, r *http.Request) {
        rows, err := d.Store.DB.Query(`SELECT ts, user_token, channel, remote_id, action, detail, ok
                FROM audit ORDER BY ts DESC LIMIT 200`)
        if err != nil {
                writeJSON(w, 500, map[string]any{"error": err.Error()})
                return
        }
        defer rows.Close()
        out := make([]map[string]any, 0)
        for rows.Next() {
                var ts int64
                var tok, ch, rid, act, det string
                var ok int
                _ = rows.Scan(&ts, &tok, &ch, &rid, &act, &det, &ok)
                out = append(out, map[string]any{
                        "ts":         ts,
                        "user_token": mask(tok),
                        "channel":    ch,
                        "remote_id":  rid,
                        "action":     act,
                        "detail":     det,
                        "ok":         ok == 1,
                })
        }
        writeJSON(w, 200, out)
}

// handleListUsage returns aggregated usage per day.
func (d *Deps) handleListUsage(w http.ResponseWriter, r *http.Request) {
        rows, err := d.Store.DB.Query(`SELECT date(ts,'unixepoch') as day, agent, count(*) as n,
                sum(tokens_in) as ti, sum(tokens_out) as to_
                FROM usage GROUP BY day, agent ORDER BY day DESC LIMIT 90`)
        if err != nil {
                writeJSON(w, 500, map[string]any{"error": err.Error()})
                return
        }
        defer rows.Close()
        out := make([]map[string]any, 0)
        for rows.Next() {
                var day, agent string
                var n int
                var ti, to int64
                _ = rows.Scan(&day, &agent, &n, &ti, &to)
                out = append(out, map[string]any{
                        "day":         day,
                        "agent":       agent,
                        "requests":    n,
                        "tokens_in":   ti,
                        "tokens_out":  to,
                })
        }
        writeJSON(w, 200, out)
}

// ---------- helpers ----------

// sessionToJSON converts a *store.Session to a JSON-friendly map.
func sessionToJSON(s *store.Session) map[string]any {
        return map[string]any{
                "id":         s.ID,
                "agent":      s.Agent,
                "project":    s.Project,
                "title":      s.Title,
                "channel":    s.Channel,
                "created_at": s.CreatedAt,
                "updated_at": s.UpdatedAt,
                "closed":     s.Closed,
        }
}

// chiURLParam reads a URL parameter set by chi.
func chiURLParam(r *http.Request, name string) string { return chi.URLParam(r, name) }

// mask hides all but the first 6 and last 3 chars of a token.
func mask(s string) string {
        if len(s) <= 9 {
                return strings.Repeat("*", len(s))
        }
        return s[:6] + "..." + s[len(s)-3:]
}
