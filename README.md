# OmniAgent

> Một cổng — mọi CLI agent.
> One gateway — every CLI agent.

OmniAgent is a lightweight, single-binary gateway that lets you drive
multiple AI coding CLIs — **Claude Code**, **OpenAI Codex**, **Gemini CLI**,
**Google Antigravity**, **Z.ai ZCode** — from a single **Telegram bot** and
**Web UI** (PWA). It is inspired by
[`claude-code-telegram`](https://github.com/RichardAtCT/claude-code-telegram)
but rewritten in **Go** for a tiny memory footprint (~30 MB RSS), fast
cold-start, and a uniform adapter layer that any new CLI can plug into.

---

## Tính năng · Features

| Khu vực · Area        | Mô tả · Description                                                                                          |
| --------------------- | ------------------------------------------------------------------------------------------------------------- |
| Multi-CLI adapters    | One `Adapter` interface, 5 implementations (Claude/Codex/Gemini/Antigravity/ZCode). Add a new CLI in <100 LoC. |
| Web UI (PWA)          | Next.js 16 static export — chat terminal, multi-session sidebar, file explorer, admin panel, dark mode, installable. |
| Telegram bot          | `/start <token>` to authenticate, then chat naturally. Switch agent / project on the fly.                    |
| Auth — token + Turnstile | Each user gets a token in `.env`. Web login is gated by Cloudflare Turnstile.                                |
| Progressive ban       | N failed attempts within a sliding window → automatic time-ban (default 5 fails / 5 min → 15 min ban).       |
| Rate limit            | Per-user token bucket (default 2 rps, burst 10).                                                             |
| Streaming             | WebSocket for web, async push for Telegram. Tool calls, text, errors streamed live.                          |
| SQLite storage        | Sessions, messages, audit log, usage, ban table — zero-ops, single file.                                     |
| Workspace sandbox     | All agents run under `OMNI_WORKSPACE`; path traversal (`..`) is rejected at the adapter layer.               |
| Audit log             | Every login / send / file write is recorded. Viewable in admin panel.                                        |
| Single binary         | Go backend compiles to ~15 MB. Web UI ships as static files served by the same binary (optional).            |

---

## Kiến trúc · Architecture

```
┌───────────────────────────────────────────────────────────────────┐
│                       OmniAgent (Go binary)                        │
│                                                                    │
│   ┌──────────────┐  ┌──────────────┐  ┌──────────────────────┐    │
│   │  HTTP + WS   │  │ Telegram bot │  │  Adapter Registry    │    │
│   │  (chi)       │  │ (telebot v3) │  │  ─────────────────── │    │
│   │              │  │              │  │  claude  codex       │    │
│   │  /api/auth   │  │  /start <t>  │  │  gemini  antigravity │    │
│   │  /api/sess   │  │  /new /agent │  │  zcode  ...          │    │
│   │  /api/files  │  │  /repo /stat │  │                      │    │
│   │  /api/admin  │  │  /cancel     │  │  Each: subprocess +  │    │
│   │  /api/ws     │  │  chat text   │  │  stream-json parser  │    │
│   └──────┬───────┘  └──────┬───────┘  └──────────┬───────────┘    │
│          │                 │                     │                │
│          └──────────┬──────┴─────────────────────┘                │
│                     ▼                                              │
│            ┌─────────────────┐    ┌────────────────┐              │
│            │ Session Manager │ ─► │ SQLite Store   │              │
│            │  +  WS hub      │    │  sessions,     │              │
│            └─────────────────┘    │  messages,     │              │
│                     │              │  audit, usage, │              │
│                     ▼              │  ban           │              │
│            ┌─────────────────┐    └────────────────┘              │
│            │ Auth + Ban +    │                                    │
│            │ Rate limit      │                                    │
│            │ Turnstile verify│                                    │
│            └─────────────────┘                                    │
└───────────────────────────────────────────────────────────────────┘
                  ▲                                    ▲
                  │ HTTPS / WSS                        │ Telegram polling
                  │                                    │
        ┌─────────┴─────────┐                ┌─────────┴─────────┐
        │  Next.js 16 PWA   │                │  Telegram app     │
        │  (static export)  │                │  (any device)     │
        │  · login          │                │                   │
        │  · chat terminal  │                │                   │
        │  · file explorer  │                │                   │
        │  · admin panel    │                │                   │
        └───────────────────┘                └───────────────────┘
```

See [`ARCHITECTURE.md`](./ARCHITECTURE.md) for the full module map and
data-flow diagrams, and [`SECURITY.md`](./SECURITY.md) for the threat model.

---

## Quickstart

### Prerequisites

- Go 1.23+ (only needed to build from source)
- Node 20+ (only needed to build the web UI from source)
- At least one CLI agent installed and on `PATH` (`claude`, `codex`, `gemini`,
  `antigravity`, or `zcode`). OmniAgent auto-detects availability per adapter.
- A Telegram bot token from [@BotFather](https://t.me/botfather) (optional —
  set `OMNI_TELEGRAM_TOKEN` empty to disable the bot).
- Cloudflare Turnstile site + secret keys (optional, but recommended for any
  internet-facing deployment).

### 1. Build

```bash
# Backend (Go)
cd backend
go build -o ../bin/omniagent ./cmd/omniagent

# Frontend (Next.js)
cd ../web
npm install && npm run build
# produces web/out/ (static export)
```

### 2. Configure

```bash
cp backend/.env.example backend/.env
# edit .env — at minimum set:
#   OMNI_COOKIE_SECRET       (openssl rand -hex 32)
#   OMNI_USER_TOKENS         (token1:Label 1,token2:Label 2)
#   OMNI_WORKSPACE           (absolute path to your projects)
#   OMNI_TELEGRAM_TOKEN      (from BotFather, or empty)
#   OMNI_TURNSTILE_SITE_KEY / OMNI_TURNSTILE_SECRET_KEY  (Cloudflare)

cp web/.env.example web/.env.local
# edit .env.local — set:
#   NEXT_PUBLIC_API_BASE          (URL where OmniAgent is reachable)
#   NEXT_PUBLIC_TURNSTILE_SITE_KEY (same as backend)
```

Generate per-user tokens:

```bash
for i in 1 2 3; do
  echo "user$i-$(openssl rand -hex 16)"
done
# paste them into OMNI_USER_TOKENS as "user1-…:Dev 1,user2-…:Dev 2,…"
```

### 3. Run

```bash
# Option A — serve everything from the Go binary
OMNI_WEB_DIR=$(pwd)/web/out ./bin/omniagent -env backend/.env

# Option B — run web separately (e.g. on Vercel) and point it at the Go API
./bin/omniagent -env backend/.env
cd web && npm start
```

Open `http://localhost:8787/` (or the URL you set in `OMNI_BASE_URL`).
On Telegram, message your bot with `/start <token>`.

### 4. Docker (optional)

```bash
docker compose up --build
```

`docker-compose.yml` builds the Go binary and the Next.js static export and
runs them in a single container on port 8787. Adjust the env file mounted
into the container.

---

## Commands

### Web UI

| Route         | Mục đích · Purpose                                       |
| ------------- | -------------------------------------------------------- |
| `/`           | Login (token + Turnstile).                               |
| `/app`        | Chat terminal — multi-session sidebar, streaming output. |
| `/app/files`  | File explorer + inline editor for the workspace.         |
| `/app/admin`  | Admin panel (bans, audit log, usage). Needs `OMNI_ADMIN_TOKEN`. |

### Telegram

| Command               | Mô tả                                              |
| --------------------- | -------------------------------------------------- |
| `/start <token>`      | Authenticate (one-time per Telegram user).         |
| `/new`                | Create a new chat session.                         |
| `/agent <id>`         | Switch agent: `claude`, `codex`, `gemini`, `antigravity`, `zcode`. |
| `/agents`             | List available agents.                             |
| `/repo <name>`        | Switch project (subdirectory under `OMNI_WORKSPACE`). |
| `/repo`               | List projects in the workspace.                    |
| `/status`             | Show current agent / project / session.            |
| `/cancel`             | Cancel the running agent.                          |
| `/help`               | Show this list.                                    |
| _(plain text)_        | Send a message to the current agent.               |

---

## Configuration reference

See [`backend/.env.example`](./backend/.env.example) for the full list with
inline comments. The most important ones:

| Variable                     | Default                                   | Notes                                                |
| ---------------------------- | ----------------------------------------- | ---------------------------------------------------- |
| `OMNI_HOST` / `OMNI_PORT`    | `0.0.0.0` / `8787`                        | Listen address.                                      |
| `OMNI_BASE_URL`              | `http://localhost:8787`                   | Public URL (used for Telegram webhook, CORS, etc.).  |
| `OMNI_COOKIE_SECRET`         | _(required)_                              | HMAC key for session cookie. `openssl rand -hex 32`. |
| `OMNI_USER_TOKENS`           | _(required)_                              | `token1:Label 1,token2:Label 2,…`.                   |
| `OMNI_WORKSPACE`             | `/srv/omniagent/workspace`                | All agent subprocesses run here.                     |
| `OMNI_DB_PATH`               | `./omniagent.db`                          | SQLite path.                                         |
| `OMNI_TELEGRAM_TOKEN`        | _(empty = disabled)_                      | Bot token from BotFather.                            |
| `OMNI_TURNSTILE_ENABLED`     | `true`                                    | Set `false` to skip Turnstile verification.          |
| `OMNI_TURNSTILE_SITE_KEY`    | _(empty = skipped)_                       | Cloudflare site key.                                 |
| `OMNI_TURNSTILE_SECRET_KEY`  | _(empty = skipped)_                       | Cloudflare secret key.                               |
| `OMNI_BAN_MAX_FAILS`         | `5`                                       | Failed attempts before ban.                          |
| `OMNI_BAN_WINDOW_SECONDS`    | `300`                                     | Sliding window for fail count.                       |
| `OMNI_BAN_DURATION_SECONDS`  | `900`                                     | Ban duration.                                        |
| `OMNI_RATE_LIMIT_RPS`        | `2`                                       | Per-user sustained requests/sec.                     |
| `OMNI_RATE_LIMIT_BURST`      | `10`                                      | Per-user burst.                                      |
| `OMNI_AGENT_TIMEOUT`         | `600`                                     | Max agent subprocess run time (seconds).             |
| `OMNI_CLAUDE_BIN`            | `claude`                                  | Override binary path for the Claude adapter.         |
| `OMNI_CODEX_BIN`             | `codex`                                   | Same for Codex.                                      |
| `OMNI_GEMINI_BIN`            | `gemini`                                  | Same for Gemini.                                     |
| `OMNI_ANTIGRAVITY_BIN`       | `antigravity`                             | Same for Antigravity.                                |
| `OMNI_ZCODE_BIN`             | `zcode`                                   | Same for ZCode.                                      |
| `OMNI_ADMIN_TOKEN`           | _(empty = admin disabled)_                | Bearer token for `/api/admin/*`.                     |

---

## Adding a new CLI agent

Implement the `adapter.Adapter` interface (`backend/internal/adapter/adapter.go`):

```go
type Adapter interface {
    ID() string
    DisplayName() string
    Available(ctx context.Context) bool
    Run(ctx context.Context, spec Spec, w EventWriter) error
}
```

Embed `baseAdapter` for free subprocess plumbing — you only write the
line parser. Register in `cmd/omniagent/main.go`:

```go
registry.Register(adapter.NewFooAdapter(cfg.FooBin, cfg.AdapterEnv))
```

Add the agent id to the `agentsList()` in
`backend/internal/api/middleware.go` and the frontend will pick it up
automatically via `/api/agents`.

---

## Security

- **Token-only auth** — no passwords, no email. Each user is a 24+ byte
  random hex string stored in `.env`. Tokens never travel over plain HTTP
  in production (terminate TLS at Cloudflare/Caddy/nginx).
- **Cloudflare Turnstile** — first line of defense against bots on the web.
  Telegram is gated by the user-id ban instead.
- **Progressive ban** — `OMNI_BAN_MAX_FAILS` failures inside
  `OMNI_BAN_WINDOW_SECONDS` triggers a ban of `OMNI_BAN_DURATION_SECONDS`.
  The ban key is the client IP for web, `tg:<user_id>` for Telegram.
- **Workspace sandbox** — agents can only read/write inside `OMNI_WORKSPACE`.
  Path traversal (`../`) is rejected by `adapter.SafeJoin`.
- **Signed cookies** — the web session cookie is `token.hmac[:16]`, so
  tampering with the value invalidates the session.
- **Per-user rate limit** — token bucket per user token, independent of IP.
- **Audit log** — every login attempt, message send, and file write is
  recorded with timestamp, channel, remote id, action, and ok/fail flag.
- **Admin isolation** — `/api/admin/*` requires a separate
  `OMNI_ADMIN_TOKEN` bearer, never the user token.

See [`SECURITY.md`](./SECURITY.md) for the full threat model.

---

## Roadmap

- [ ] Webhook API (GitHub / generic) — like the original repo
- [ ] Cron scheduler for recurring agent runs
- [ ] Voice messages (Telegram) → transcription → agent
- [ ] Image upload + analysis
- [ ] Per-user cost cap (Anthropic / OpenAI billing APIs)
- [ ] Plugin system

---

## Credits

- Inspired by [`claude-code-telegram`](https://github.com/RichardAtCT/claude-code-telegram) by RichardAtCT.
- Built with [chi](https://github.com/go-chi/chi),
  [telebot v3](https://gopkg.in/telebot.v3),
  [gorilla/websocket](https://github.com/gorilla/websocket),
  [mattn/go-sqlite3](https://github.com/mattn/go-sqlite3),
  [Next.js 16](https://nextjs.org),
  [Tailwind CSS](https://tailwindcss.com),
  [lucide-react](https://lucide.dev).

## License

MIT — see [`LICENSE`](./LICENSE).
