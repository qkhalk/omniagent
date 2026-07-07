// Package api exposes the HTTP+WS surface consumed by the web UI and
// (optionally) external integrations.
package api

import (
        "net/http"
        "path/filepath"
        "strings"

        "github.com/go-chi/chi/v5"
        "github.com/go-chi/chi/v5/middleware"
        "github.com/go-chi/cors"

        "github.com/omniagent/omniagent/internal/auth"
        "github.com/omniagent/omniagent/internal/config"
        "github.com/omniagent/omniagent/internal/ratelimit"
        "github.com/omniagent/omniagent/internal/session"
        "github.com/omniagent/omniagent/internal/store"
)

// Deps bundles the services a router needs.
type Deps struct {
        Cfg     *config.Config
        Store   *store.Store
        Auth    *auth.Service
        Limiter *ratelimit.Limiter
        Sessions *session.Manager
}

// New builds the http.Handler.
func New(d Deps) http.Handler {
        r := chi.NewRouter()
        r.Use(middleware.RealIP)
        r.Use(middleware.RequestID)
        r.Use(middleware.Recoverer)
        r.Use(cors.Handler(cors.Options{
                AllowedOrigins:   []string{"*"},
                AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
                AllowedHeaders:   []string{"Authorization", "Content-Type", "X-Requested-With"},
                ExposedHeaders:   []string{"X-Session-Id"},
                AllowCredentials: true,
                MaxAge:           300,
        }))
        r.Use(jsonLogger)

        // Health (no auth)
        r.Get("/api/health", func(w http.ResponseWriter, r *http.Request) {
                writeJSON(w, 200, map[string]any{"ok": true, "ts": timeNow()})
        })

        // Public: agent catalog + turnstile site key
        r.Get("/api/agents", func(w http.ResponseWriter, r *http.Request) {
                writeJSON(w, 200, map[string]any{
                        "agents":             d.agentsList(),
                        "turnstile_site_key": d.Cfg.TurnstileSiteKey,
                        "turnstile_enabled":  d.Cfg.TurnstileEnabled,
                })
        })

        // Auth
        r.Route("/api/auth", func(r chi.Router) {
                r.Post("/login", d.handleLogin)
                r.Post("/logout", d.handleLogout)
                r.With(d.authMiddleware).Get("/me", d.handleMe)
        })

        // Authenticated app endpoints
        r.Group(func(r chi.Router) {
                r.Use(d.authMiddleware)
                r.Use(d.rateLimitMiddleware)

                r.Get("/api/sessions", d.handleListSessions)
                r.Post("/api/sessions", d.handleCreateSession)
                r.Get("/api/sessions/{id}", d.handleGetSession)
                r.Delete("/api/sessions/{id}", d.handleCloseSession)
                r.Get("/api/sessions/{id}/messages", d.handleListMessages)
                r.Post("/api/sessions/{id}/send", d.handleSend)
                r.Post("/api/sessions/{id}/cancel", d.handleCancel)

                r.Get("/api/projects", d.handleListProjects)
                r.Get("/api/files", d.handleFileList)
                r.Get("/api/files/read", d.handleFileRead)
                r.Post("/api/files/write", d.handleFileWrite)
        })

        // WebSocket (auth via query string token; ban via IP)
        r.Get("/api/ws", d.handleWS)

        // Admin
        r.Group(func(r chi.Router) {
                r.Use(d.adminMiddleware)
                r.Get("/api/admin/bans", d.handleListBans)
                r.Delete("/api/admin/bans/{key}", d.handleUnban)
                r.Get("/api/admin/audit", d.handleListAudit)
                r.Get("/api/admin/usage", d.handleListUsage)
        })

        // Static SPA (if configured)
        if d.Cfg.WebDir != "" {
                staticDir := filepath.Clean(d.Cfg.WebDir)
                r.Get("/*", func(w http.ResponseWriter, r *http.Request) {
                        p := filepath.Join(staticDir, filepath.Clean("/"+r.URL.Path))
                        if !strings.HasPrefix(p, staticDir) {
                                http.NotFound(w, r)
                                return
                        }
                        http.FileServer(http.Dir(staticDir)).ServeHTTP(w, r)
                })
        }

        return r
}
