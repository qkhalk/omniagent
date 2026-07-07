// Command omniagent launches the OmniAgent server (HTTP + WebSocket + Telegram bot).
package main

import (
        "context"
        "flag"
        "fmt"
        "net/http"
        "os"
        "os/signal"
        "syscall"
        "time"

        "github.com/omniagent/omniagent/internal/adapter"
        "github.com/omniagent/omniagent/internal/api"
        "github.com/omniagent/omniagent/internal/auth"
        "github.com/omniagent/omniagent/internal/bot"
        "github.com/omniagent/omniagent/internal/config"
        "github.com/omniagent/omniagent/internal/log"
        "github.com/omniagent/omniagent/internal/ratelimit"
        "github.com/omniagent/omniagent/internal/session"
        "github.com/omniagent/omniagent/internal/store"
)

func main() {
        envFile := flag.String("env", ".env", "path to .env file (loaded if it exists)")
        flag.Parse()

        // load .env into env (best-effort)
        if err := loadDotEnv(*envFile); err != nil && !os.IsNotExist(err) {
                fmt.Fprintln(os.Stderr, "load .env:", err)
        }

        cfg, err := config.Load()
        if err != nil {
                fmt.Fprintln(os.Stderr, "config:", err)
                os.Exit(1)
        }

        log.Init(cfg.LogFormat, cfg.LogLevel)
        log.Info("omniagent starting", "host", cfg.Host, "port", cfg.Port, "workspace", cfg.Workspace)

        // Ensure workspace exists
        if err := os.MkdirAll(cfg.Workspace, 0o755); err != nil {
                log.Error("workspace mkdir", "err", err)
                os.Exit(1)
        }

        st, err := store.Open(cfg.DBPath)
        if err != nil {
                log.Error("store open", "err", err)
                os.Exit(1)
        }
        defer st.Close()

        as := auth.New(cfg, st)
        sm := session.NewManager(st, cfg.Workspace, cfg.AgentTimeout)
        limiter := ratelimit.New(cfg.RateLimitRPS, cfg.RateLimitBurst)

        // Adapter registry
        registry := adapter.NewRegistry()
        registry.Register(adapter.NewClaudeAdapter(cfg.ClaudeBin, cfg.AdapterEnv))
        registry.Register(adapter.NewCodexAdapter(cfg.CodexBin, cfg.AdapterEnv))
        registry.Register(adapter.NewGeminiAdapter(cfg.GeminiBin, cfg.AdapterEnv))
        registry.Register(adapter.NewAntigravityAdapter(cfg.AntigravityBin, cfg.AdapterEnv))
        registry.Register(adapter.NewZCodeAdapter(cfg.ZcodeBin, cfg.AdapterEnv))

        // Wire registry into api and bot packages (avoids import cycle)
        api.SetAdapterRegistry(registry)
        bot.SetAdapterRegistry(registry)

        deps := api.Deps{Cfg: cfg, Store: st, Auth: as, Limiter: limiter, Sessions: sm}
        handler := api.New(deps)

        srv := &http.Server{
                Addr:              cfg.Addr(),
                Handler:           handler,
                ReadHeaderTimeout: 10 * time.Second,
                IdleTimeout:       120 * time.Second,
        }

        // Telegram bot (optional)
        tgBot, err := bot.New(cfg, st, as, sm)
        if err != nil {
                log.Error("telegram bot init", "err", err)
                os.Exit(1)
        }

        ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
        defer cancel()

        if tgBot != nil {
                go tgBot.Start(ctx)
                log.Info("telegram bot started")
        } else {
                log.Info("telegram bot disabled (OMNI_TELEGRAM_TOKEN not set)")
        }

        go func() {
                log.Info("http listen", "addr", cfg.Addr())
                if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
                        log.Error("http server", "err", err)
                        cancel()
                }
        }()

        <-ctx.Done()
        log.Info("shutting down")
        shutCtx, shutCancel := context.WithTimeout(context.Background(), 5*time.Second)
        defer shutCancel()
        _ = srv.Shutdown(shutCtx)
}

// loadDotEnv reads KEY=VAL lines from path into the process environment.
// Lines starting with # are comments. Existing env vars are NOT overridden.
func loadDotEnv(path string) error {
        b, err := os.ReadFile(path)
        if err != nil {
                return err
        }
        lines := splitLines(string(b))
        for _, ln := range lines {
                ln = trimSpace(ln)
                if ln == "" || ln[0] == '#' {
                        continue
                }
                i := indexByte(ln, '=')
                if i <= 0 {
                        continue
                }
                k := trimSpace(ln[:i])
                v := trimSpace(ln[i+1:])
                v = unquote(v)
                if _, ok := os.LookupEnv(k); !ok {
                        _ = os.Setenv(k, v)
                }
        }
        return nil
}

func splitLines(s string) []string {
        var out []string
        start := 0
        for i := 0; i < len(s); i++ {
                if s[i] == '\n' {
                        out = append(out, s[start:i])
                        start = i + 1
                }
        }
        if start < len(s) {
                out = append(out, s[start:])
        }
        return out
}
func trimSpace(s string) string {
        for len(s) > 0 && (s[0] == ' ' || s[0] == '\t' || s[0] == '\r') {
                s = s[1:]
        }
        for len(s) > 0 && (s[len(s)-1] == ' ' || s[len(s)-1] == '\t' || s[len(s)-1] == '\r') {
                s = s[:len(s)-1]
        }
        return s
}
func indexByte(s string, b byte) int {
        for i := 0; i < len(s); i++ {
                if s[i] == b {
                        return i
                }
        }
        return -1
}
func unquote(s string) string {
        if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
                return s[1 : len(s)-1]
        }
        if len(s) >= 2 && s[0] == '\'' && s[len(s)-1] == '\'' {
                return s[1 : len(s)-1]
        }
        return s
}
