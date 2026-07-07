package api

import (
        "context"
        "encoding/json"
        "errors"
        "net/http"
        "strings"
        "time"

        "github.com/omniagent/omniagent/internal/auth"
        "github.com/omniagent/omniagent/internal/log"
)

// ctxKey is a private type used to stash values on request contexts.
type ctxKey string

const (
        ctxPrincipal ctxKey = "principal"
)

// authMiddleware returns a chi-compatible middleware that validates either a
// Bearer token (web) or query ?token (WS). Failed auth returns 401.
func (d *Deps) authMiddleware(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
                p, err := d.authenticate(r)
                if err != nil {
                        writeJSON(w, http.StatusUnauthorized, map[string]any{"error": err.Error()})
                        return
                }
                r = r.WithContext(context.WithValue(r.Context(), ctxPrincipal, p))
                next.ServeHTTP(w, r)
        })
}

// authenticate extracts the bearer token and Turnstile token from the
// request, calls Auth.Verify, and returns the principal.
func (d *Deps) authenticate(r *http.Request) (*auth.Principal, error) {
        userToken := bearer(r)
        if userToken == "" {
                userToken = r.URL.Query().Get("token")
        }
        if userToken == "" {
                // try signed cookie
                if c, err := r.Cookie("omni_token"); err == nil {
                        userToken = verifyCookie(c.Value, d.Cfg.CookieSecret)
                }
        }
        if userToken == "" {
                return nil, errors.New("missing token")
        }
        turnstile := r.Header.Get("X-Turnstile-Token")
        ip := clientIP(r)
        p, err := d.Auth.Verify(r.Context(), userToken, turnstile, ip)
        if err != nil {
                log.Warn("auth failed", "ip", ip, "err", err)
                return nil, err
        }
        return p, nil
}

// principal returns the authenticated principal from context, or nil.
func principal(r *http.Request) *auth.Principal {
        if v := r.Context().Value(ctxPrincipal); v != nil {
                if p, ok := v.(*auth.Principal); ok {
                        return p
                }
        }
        return nil
}

// adminMiddleware checks OMNI_ADMIN_TOKEN via Bearer header.
func (d *Deps) adminMiddleware(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
                if d.Cfg.AdminToken == "" {
                        writeJSON(w, http.StatusForbidden, map[string]any{"error": "admin disabled"})
                        return
                }
                tok := bearer(r)
                if tok != d.Cfg.AdminToken {
                        writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "admin unauthorized"})
                        return
                }
                next.ServeHTTP(w, r)
        })
}

// rateLimitMiddleware enforces the per-user token bucket.
func (d *Deps) rateLimitMiddleware(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
                p := principal(r)
                if p == nil {
                        next.ServeHTTP(w, r)
                        return
                }
                if !d.Limiter.Allow(p.Token) {
                        writeJSON(w, http.StatusTooManyRequests, map[string]any{"error": "rate limit"})
                        return
                }
                next.ServeHTTP(w, r)
        })
}

// bearer extracts the Bearer token from the Authorization header.
func bearer(r *http.Request) string {
        h := r.Header.Get("Authorization")
        if h == "" {
                return ""
        }
        if !strings.HasPrefix(h, "Bearer ") {
                return ""
        }
        return strings.TrimSpace(strings.TrimPrefix(h, "Bearer "))
}

// clientIP extracts the client IP from request, honoring X-Forwarded-For.
func clientIP(r *http.Request) string {
        if ff := r.Header.Get("X-Forwarded-For"); ff != "" {
                return strings.TrimSpace(strings.Split(ff, ",")[0])
        }
        if r.RemoteAddr == "" {
                return ""
        }
        if i := strings.LastIndex(r.RemoteAddr, ":"); i > 0 {
                return r.RemoteAddr[:i]
        }
        return r.RemoteAddr
}

// writeJSON writes a JSON response.
func writeJSON(w http.ResponseWriter, status int, v any) {
        w.Header().Set("Content-Type", "application/json; charset=utf-8")
        w.WriteHeader(status)
        _ = json.NewEncoder(w).Encode(v)
}

// jsonLogger is a tiny request logger middleware.
func jsonLogger(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
                start := time.Now()
                ww := &statusWriter{ResponseWriter: w, status: 200}
                next.ServeHTTP(ww, r)
                log.Info("http",
                        "method", r.Method,
                        "path", r.URL.Path,
                        "status", ww.status,
                        "ip", clientIP(r),
                        "dur_ms", time.Since(start).Milliseconds())
        })
}

type statusWriter struct {
        http.ResponseWriter
        status int
}

// WriteHeader captures the status code.
func (w *statusWriter) WriteHeader(code int) {
        w.status = code
        w.ResponseWriter.WriteHeader(code)
}

// agentsList returns the public list of available adapters.
func (d *Deps) agentsList() []map[string]any {
        // We don't have a direct reference to the registry here; instead we
        // hard-code the agent IDs and let the frontend label them. The actual
        // adapter lookup happens in chat.go via the Manager's adapter registry.
        return []map[string]any{
                {"id": adapterClaude, "name": "Claude Code"},
                {"id": adapterCodex, "name": "OpenAI Codex"},
                {"id": adapterGemini, "name": "Gemini CLI"},
                {"id": adapterAntigravity, "name": "Antigravity CLI"},
                {"id": adapterZcode, "name": "ZCode"},
        }
}

// timeNow returns the current unix seconds.
func timeNow() int64 { return time.Now().Unix() }

// adapter ID constants used by both router.go and chat.go
const (
        adapterClaude      = "claude"
        adapterCodex       = "codex"
        adapterGemini      = "gemini"
        adapterAntigravity = "antigravity"
        adapterZcode       = "zcode"
)
