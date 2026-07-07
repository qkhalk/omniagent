// Package auth implements token validation, Cloudflare Turnstile verification
// and progressive (IP / Telegram-user-id) banning.
package auth

import (
        "context"
        "database/sql"
        "encoding/json"
        "errors"
        "fmt"
        "io"
        "net/http"
        "net/url"
        "strings"
        "time"

        "github.com/omniagent/omniagent/internal/config"
        "github.com/omniagent/omniagent/internal/store"
)

// Service wires token validation, Turnstile and ban logic.
type Service struct {
        cfg   *config.Config
        store *store.Store
        hc    *http.Client
}

// New returns a configured *Service.
func New(cfg *config.Config, st *store.Store) *Service {
        return &Service{
                cfg:   cfg,
                store: st,
                hc:    &http.Client{Timeout: 8 * time.Second},
        }
}

// Principal is the authenticated identity.
type Principal struct {
        Token string
        Label string
        IP    string
}

// Verify verifies the user token + (optional) Turnstile token from the web.
// remoteKey is the identifier used for ban tracking: IP for web, "tg:<user_id>" for Telegram.
func (s *Service) Verify(ctx context.Context, userToken, turnstileToken, remoteKey string) (*Principal, error) {
        if err := s.checkBan(ctx, remoteKey); err != nil {
                return nil, err
        }

        ut, ok := s.cfg.FindUserToken(userToken)
        if !ok {
                s.recordFail(ctx, remoteKey, "token", userToken)
                return nil, ErrBadToken
        }

        if s.cfg.TurnstileEnabled && turnstileToken != "" {
                if err := s.verifyTurnstile(ctx, turnstileToken, remoteKey); err != nil {
                        s.recordFail(ctx, remoteKey, "turnstile", "")
                        return nil, fmt.Errorf("turnstile: %w", err)
                }
        }

        return &Principal{Token: ut.Token, Label: ut.Label, IP: remoteKey}, nil
}

// VerifyTelegram authenticates a Telegram user with the token sent via /start <token>.
// No Turnstile is needed for Telegram (the bot enforces it via the user-id ban).
func (s *Service) VerifyTelegram(ctx context.Context, userToken string, tgUserID int64) (*Principal, error) {
        remoteKey := fmt.Sprintf("tg:%d", tgUserID)
        if err := s.checkBan(ctx, remoteKey); err != nil {
                return nil, err
        }
        ut, ok := s.cfg.FindUserToken(userToken)
        if !ok {
                s.recordFail(ctx, remoteKey, "token", userToken)
                return nil, ErrBadToken
        }
        return &Principal{Token: ut.Token, Label: ut.Label, IP: remoteKey}, nil
}

// checkBan returns ErrBanned if the key is currently banned.
func (s *Service) checkBan(ctx context.Context, key string) error {
        b, err := s.store.GetBan(ctx, key)
        if err != nil {
                if errors.Is(err, sql.ErrNoRows) {
                        return nil
                }
                // tolerate DB errors
                return nil
        }
        if b.BannedUntil > time.Now().Unix() {
                return fmt.Errorf("%w: still %ds", ErrBanned, b.BannedUntil-time.Now().Unix())
        }
        return nil
}

// recordFail bumps the fail counter and possibly triggers a ban.
func (s *Service) recordFail(ctx context.Context, key, kind, detail string) {
        b, err := s.store.UpsertBan(ctx, key, s.cfg.BanMaxFails, s.cfg.BanWindowSeconds, s.cfg.BanDurationSeconds)
        if err != nil {
                return
        }
        if b.BannedUntil > time.Now().Unix() {
                _ = s.store.AddAudit(ctx, "", "ban", key, "banned", detail, false)
        } else {
                _ = s.store.AddAudit(ctx, "", "auth", key, "fail:"+kind, detail, false)
        }
}

// ClearBan resets the ban for a key.
func (s *Service) ClearBan(ctx context.Context, key string) error {
        return s.store.ClearBan(ctx, key)
}

func (s *Service) verifyTurnstile(ctx context.Context, token, remoteIP string) error {
        if s.cfg.TurnstileSecretKey == "" {
                return errors.New("turnstile secret key not configured")
        }
        form := url.Values{}
        form.Set("secret", s.cfg.TurnstileSecretKey)
        form.Set("response", token)
        if remoteIP != "" {
                form.Set("remoteip", remoteIP)
        }
        req, _ := http.NewRequestWithContext(ctx, http.MethodPost,
                "https://challenges.cloudflare.com/turnstile/v0/siteverify", strings.NewReader(form.Encode()))
        req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
        resp, err := s.hc.Do(req)
        if err != nil {
                return err
        }
        defer resp.Body.Close()
        body, _ := io.ReadAll(resp.Body)
        var r struct {
                Success     bool     `json:"success"`
                ErrorCodes  []string `json:"error-codes"`
        }
        if err := json.Unmarshal(body, &r); err != nil {
                return err
        }
        if !r.Success {
                return fmt.Errorf("turnstile rejected: %v", r.ErrorCodes)
        }
        return nil
}

// Sentinel errors.
var (
        ErrBadToken = errors.New("invalid token")
        ErrBanned   = errors.New("banned")
)
