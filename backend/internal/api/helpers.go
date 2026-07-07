package api

import (
        "crypto/hmac"
        "crypto/rand"
        "crypto/sha256"
        "encoding/hex"
        "encoding/json"
        "errors"
        "io"
        "net/http"
        "os"
        "path/filepath"
        "sort"
        "strings"

        "github.com/gorilla/websocket"
        "github.com/omniagent/omniagent/internal/adapter"
        "github.com/omniagent/omniagent/internal/log"
)

// randHex returns n random hex characters (n/2 random bytes).
func randHex(n int) string {
        b := make([]byte, (n+1)/2)
        if _, err := rand.Read(b); err != nil {
                // fall back to a constant; should never happen in practice
                for i := range b {
                        b[i] = 0xab
                }
        }
        return hex.EncodeToString(b)[:n]
}

// hmacHex returns hex(HMAC-SHA256(key, msg)).
func hmacHex(msg, key []byte) string {
        h := hmac.New(sha256.New, key)
        h.Write(msg)
        return hex.EncodeToString(h.Sum(nil))
}

// decodeJSON decodes a small JSON body into v.
func decodeJSON(r *http.Request, v any) error {
        defer r.Body.Close()
        dec := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 1<<20))
        if err := dec.Decode(v); err != nil {
                if errors.Is(err, io.EOF) {
                        return errors.New("empty body")
                }
                return err
        }
        return nil
}

// newID returns a short random id (16 hex chars).
func newID() string { return randHex(16) }

// signCookie returns a simple signature for the cookie value.
// We embed the token and a short HMAC prefix so that tampering with the
// cookie value will fail validation.
func signCookie(token, secret string) string {
        sig := hmacHex([]byte(token), []byte(secret))
        return token + "." + sig[:16]
}

// verifyCookie returns the token embedded in a signed cookie, or empty.
func verifyCookie(val, secret string) string {
        idx := strings.LastIndex(val, ".")
        if idx <= 0 {
                return ""
        }
        token := val[:idx]
        want := hmacHex([]byte(token), []byte(secret))[:16]
        if want != val[idx+1:] {
                return ""
        }
        return token
}

// readFileCapped reads up to limit bytes from path.
func readFileCapped(path string, limit int64) ([]byte, error) {
        f, err := os.Open(path)
        if err != nil {
                return nil, err
        }
        defer f.Close()
        buf := make([]byte, 0, 4096)
        tmp := make([]byte, 4096)
        for int64(len(buf)) < limit {
                n, err := f.Read(tmp)
                if n > 0 {
                        buf = append(buf, tmp[:n]...)
                }
                if err == io.EOF {
                        break
                }
                if err != nil {
                        return nil, err
                }
        }
        return buf, nil
}

// writeFile writes b to path, creating directories if needed.
func writeFile(path string, b []byte) error {
        if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
                return err
        }
        return os.WriteFile(path, b, 0o644)
}

// listDir returns entries (name + is_dir + size) under dir.
func listDir(dir string) ([]map[string]any, error) {
        entries, err := os.ReadDir(dir)
        if err != nil {
                return nil, err
        }
        out := make([]map[string]any, 0, len(entries))
        for _, e := range entries {
                info, _ := e.Info()
                out = append(out, map[string]any{
                        "name":    e.Name(),
                        "is_dir":  e.IsDir(),
                        "size":    func() int64 { if info != nil { return info.Size() }; return 0 }(),
                        "mod":     func() int64 { if info != nil { return info.ModTime().Unix() }; return 0 }(),
                })
        }
        sort.Slice(out, func(i, j int) bool {
                id, _ := out[i]["is_dir"].(bool)
                jd, _ := out[j]["is_dir"].(bool)
                if id != jd {
                        return id
                }
                ni, _ := out[i]["name"].(string)
                nj, _ := out[j]["name"].(string)
                return ni < nj
        })
        return out, nil
}

// listSubdirs returns the names of immediate subdirectories under root.
func listSubdirs(root string) []string {
        entries, err := os.ReadDir(root)
        if err != nil {
                return nil
        }
        var out []string
        for _, e := range entries {
                if e.IsDir() {
                        out = append(out, e.Name())
                }
        }
        sort.Strings(out)
        return out
}

// wsUpgrader is the WebSocket upgrader.
var wsUpgrader = websocket.Upgrader{
        ReadBufferSize:  1024,
        WriteBufferSize: 4096,
        CheckOrigin:     func(r *http.Request) bool { return true },
}

// handleWS upgrades to a WebSocket and streams session events.
func (d *Deps) handleWS(w http.ResponseWriter, r *http.Request) {
        // Auth: token from cookie or ?token=
        token := r.URL.Query().Get("token")
        if token == "" {
                if c, err := r.Cookie("omni_token"); err == nil {
                        token = verifyCookie(c.Value, d.Cfg.CookieSecret)
                }
        }
        if token == "" {
                writeJSON(w, 401, map[string]any{"error": "unauthorized"})
                return
        }
        _, err := d.Auth.Verify(r.Context(), token, "", clientIP(r))
        if err != nil {
                writeJSON(w, 401, map[string]any{"error": err.Error()})
                return
        }

        conn, err := wsUpgrader.Upgrade(w, r, nil)
        if err != nil {
                log.Warn("ws upgrade failed", "err", err)
                return
        }
        defer conn.Close()

        sessionID := r.URL.Query().Get("session")
        if sessionID == "" {
                _ = conn.WriteJSON(map[string]any{"type": "error", "text": "missing session"})
                return
        }

        // Authorize: session must belong to this token
        s, err := d.Store.GetSession(r.Context(), sessionID)
        if err != nil || s == nil || s.UserToken != token {
                _ = conn.WriteJSON(map[string]any{"type": "error", "text": "session not found"})
                return
        }

        sub := d.Sessions.Subscribe(sessionID)
        defer d.Sessions.Unsubscribe(sessionID, sub.ID)
        defer close(sub.Done)

        // Replay recent messages first
        msgs, _ := d.Store.ListMessages(r.Context(), sessionID, 100)
        for _, m := range msgs {
                _ = conn.WriteJSON(map[string]any{
                        "type":      "replay",
                        "role":      m.Role,
                        "content":   m.Content,
                        "tool_name": m.ToolName,
                })
        }
        _ = conn.WriteJSON(map[string]any{"type": "ready"})

        // Pump
        go func() {
                for {
                        if _, _, err := conn.ReadMessage(); err != nil {
                                return
                        }
                }
        }()

        for {
                select {
                case ev, ok := <-sub.Ch:
                        if !ok {
                                return
                        }
                        if err := conn.WriteJSON(ev); err != nil {
                                return
                        }
                case <-r.Context().Done():
                        return
                }
        }
}

// adapterRef is set by main.go via SetAdapterRegistry so the api package
// can avoid an import cycle on session.
var adapterRef *adapter.Registry

// SetAdapterRegistry lets main wire the adapter registry into the api package.
func SetAdapterRegistry(r *adapter.Registry) { adapterRef = r }

// lookupAdapter returns the adapter registered under id.
func (d *Deps) lookupAdapter(id string) (adapter.Adapter, bool) {
        if adapterRef == nil {
                return nil, false
        }
        return adapterRef.Get(id)
}
