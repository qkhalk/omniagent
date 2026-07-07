// Package config loads OmniAgent configuration from environment.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds all runtime configuration.
type Config struct {
	Host        string
	Port        string
	BaseURL     string
	WebDir      string
	CookieSecret string
	AgentTimeout time.Duration

	DBPath     string
	Workspace  string

	UserTokens []UserToken

	TelegramToken          string
	TelegramWebhookURL     string
	TelegramWebhookSecret  string

	TurnstileSiteKey   string
	TurnstileSecretKey string
	TurnstileEnabled   bool

	BanMaxFails       int
	BanWindowSeconds  int
	BanDurationSeconds int

	RateLimitRPS   float64
	RateLimitBurst int

	ClaudeBin      string
	CodexBin       string
	GeminiBin      string
	AntigravityBin string
	ZcodeBin       string
	AdapterEnv     []string

	LogFormat string
	LogLevel  string

	AdminToken string
}

// UserToken pairs a token string with a human label.
type UserToken struct {
	Token string
	Label string
}

// Load reads configuration from environment.
func Load() (*Config, error) {
	c := &Config{
		Host:         getenv("OMNI_HOST", "0.0.0.0"),
		Port:         getenv("OMNI_PORT", "8787"),
		BaseURL:      getenv("OMNI_BASE_URL", "http://localhost:8787"),
		WebDir:       getenv("OMNI_WEB_DIR", ""),
		CookieSecret: getenv("OMNI_COOKIE_SECRET", ""),
		AgentTimeout: time.Duration(getenvInt("OMNI_AGENT_TIMEOUT", 600)) * time.Second,

		DBPath:    getenv("OMNI_DB_PATH", "./omniagent.db"),
		Workspace: getenv("OMNI_WORKSPACE", "/srv/omniagent/workspace"),

		TelegramToken:         getenv("OMNI_TELEGRAM_TOKEN", ""),
		TelegramWebhookURL:    getenv("OMNI_TELEGRAM_WEBHOOK_URL", ""),
		TelegramWebhookSecret: getenv("OMNI_TELEGRAM_WEBHOOK_SECRET", ""),

		TurnstileSiteKey:   getenv("OMNI_TURNSTILE_SITE_KEY", ""),
		TurnstileSecretKey: getenv("OMNI_TURNSTILE_SECRET_KEY", ""),
		TurnstileEnabled:   getenvBool("OMNI_TURNSTILE_ENABLED", true),

		BanMaxFails:        getenvInt("OMNI_BAN_MAX_FAILS", 5),
		BanWindowSeconds:   getenvInt("OMNI_BAN_WINDOW_SECONDS", 300),
		BanDurationSeconds: getenvInt("OMNI_BAN_DURATION_SECONDS", 900),

		RateLimitRPS:   getenvFloat("OMNI_RATE_LIMIT_RPS", 2),
		RateLimitBurst: getenvInt("OMNI_RATE_LIMIT_BURST", 10),

		ClaudeBin:      getenv("OMNI_CLAUDE_BIN", "claude"),
		CodexBin:       getenv("OMNI_CODEX_BIN", "codex"),
		GeminiBin:      getenv("OMNI_GEMINI_BIN", "gemini"),
		AntigravityBin: getenv("OMNI_ANTIGRAVITY_BIN", "antigravity"),
		ZcodeBin:       getenv("OMNI_ZCODE_BIN", "zcode"),

		LogFormat: getenv("OMNI_LOG_FORMAT", "text"),
		LogLevel:  getenv("OMNI_LOG_LEVEL", "info"),

		AdminToken: getenv("OMNI_ADMIN_TOKEN", ""),
	}

	c.UserTokens = parseUserTokens(getenv("OMNI_USER_TOKENS", ""))
	c.AdapterEnv = parseAdapterEnv(getenv("OMNI_ADAPTER_ENV", ""))

	if c.CookieSecret == "" {
		return nil, fmt.Errorf("OMNI_COOKIE_SECRET must be set (generate with `openssl rand -hex 32`)")
	}
	if len(c.UserTokens) == 0 {
		return nil, fmt.Errorf("OMNI_USER_TOKENS must list at least one token")
	}
	return c, nil
}

// Addr returns the listen address.
func (c *Config) Addr() string { return c.Host + ":" + c.Port }

// FindUserToken returns the UserToken whose Token matches the supplied value.
func (c *Config) FindUserToken(token string) (UserToken, bool) {
	for _, t := range c.UserTokens {
		if t.Token == token {
			return t, true
		}
	}
	return UserToken{}, false
}

func getenv(k, def string) string {
	if v, ok := os.LookupEnv(k); ok {
		return v
	}
	return def
}
func getenvInt(k string, def int) int {
	if v, ok := os.LookupEnv(k); ok {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}
func getenvFloat(k string, def float64) float64 {
	if v, ok := os.LookupEnv(k); ok {
		if n, err := strconv.ParseFloat(v, 64); err == nil {
			return n
		}
	}
	return def
}
func getenvBool(k string, def bool) bool {
	if v, ok := os.LookupEnv(k); ok {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return def
}

func parseUserTokens(s string) []UserToken {
	var out []UserToken
	for _, pair := range strings.Split(s, ",") {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		label := ""
		token := pair
		if i := strings.Index(pair, ":"); i >= 0 {
			token = strings.TrimSpace(pair[:i])
			label = strings.TrimSpace(pair[i+1:])
		}
		if token == "" {
			continue
		}
		out = append(out, UserToken{Token: token, Label: label})
	}
	return out
}

func parseAdapterEnv(s string) []string {
	var out []string
	for _, kv := range strings.Split(s, ";") {
		kv = strings.TrimSpace(kv)
		if kv == "" {
			continue
		}
		out = append(out, kv)
	}
	return out
}
