# OmniAgent — Architecture

This document describes the internal structure of OmniAgent so that
contributors can navigate the codebase and extend it.

## Module map

```
backend/
├── cmd/omniagent/main.go        # Entry point: load .env → wire services → start HTTP + bot
├── go.mod
├── internal/
│   ├── config/config.go         # Env loader, validation, types
│   ├── log/log.go               # slog wrapper (text or JSON)
│   ├── store/
│   │   ├── store.go             # SQLite open + schema migration
│   │   └── queries.go           # Sessions / messages / ban / audit / usage
│   ├── auth/auth.go             # Token verify, Turnstile verify, ban check, ban upsert
│   ├── ratelimit/limiter.go     # Per-key token bucket
│   ├── adapter/
│   │   ├── adapter.go           # Adapter interface, Event, Spec, Registry
│   │   ├── base.go              # baseAdapter: subprocess spawn + line scanner
│   │   └── adapters.go          # 5 concrete adapters + SafeJoin
│   ├── session/manager.go       # In-memory run registry + subscriber broadcast
│   ├── api/
│   │   ├── router.go            # chi router, route table, SPA fallback
│   │   ├── middleware.go        # authMiddleware, adminMiddleware, rateLimitMiddleware
│   │   ├── auth.go              # /api/auth/{login,logout,me}, sessions CRUD
│   │   ├── chat.go              # /api/sessions/{id}/{send,cancel}, files, admin
│   │   └── helpers.go           # JSON, cookie signing, WebSocket, file IO, adapters list
│   └── bot/
│       ├── bot.go               # telebot wiring, command handlers, chat pump
│       └── util.go              # randHex, osReadDir helpers
└── .env.example

web/
├── next.config.ts               # output: "export" (static SPA)
├── tailwind.config.ts           # Minimal mono palette via CSS vars
├── src/
│   ├── app/
│   │   ├── layout.tsx           # Root layout: fonts, I18nProvider, SW register
│   │   ├── globals.css          # Theme tokens, dark mode
│   │   ├── page.tsx             # /  — login (token + Turnstile)
│   │   └── app/
│   │       ├── layout.tsx       # App shell: sidebar nav, auth guard
│   │       ├── page.tsx         # /app — chat terminal + multi-session
│   │       ├── files/page.tsx   # /app/files — file explorer + editor
│   │       └── admin/page.tsx   # /app/admin — bans / audit / usage
│   ├── components/
│   │   ├── ui/                  # Button, Input, Card, Select (shadcn-style)
│   │   ├── turnstile.tsx        # Cloudflare Turnstile loader + widget
│   │   ├── lang-toggle.tsx      # VI/EN toggle
│   │   ├── theme-toggle.tsx     # dark/light
│   │   └── sw-register.tsx      # PWA service worker registration
│   ├── i18n/
│   │   ├── index.ts             # Dictionary (vi / en)
│   │   └── provider.tsx         # React context provider + useI18n hook
│   └── lib/
│       ├── api.ts               # Fetch client for /api/*, WebSocket URL builder
│       └── utils.ts             # cn, formatBytes, formatTime
└── public/                      # manifest.json, sw.js, favicon.svg, icon PNGs
```

## Request lifecycle

### Web login

1. User opens `/` → React fetches `/api/agents` (public) to learn the
   Turnstile site key and the list of available agents.
2. User enters token, Turnstile widget produces a Turnstile token.
3. `POST /api/auth/login` with `{token, turnstile_token}`.
4. `auth.Service.Verify` checks the ban table, looks up the token in
   `OMNI_USER_TOKENS`, verifies Turnstile with Cloudflare's
   `siteverify` endpoint, then returns the principal.
5. Server sets an `HttpOnly` cookie `omni_token = token.hmac[:16]`
   and returns the raw token in the JSON body. The browser stores the
   token in `localStorage` for `Authorization: Bearer` headers and the
   cookie is used for the WebSocket upgrade (which can't set headers).

### Send a message (web)

1. `POST /api/sessions` → create row in `sessions` table.
2. Open `GET /api/ws?session=<id>&token=<token>` → upgrade to WebSocket,
   authorize session ownership, replay last 100 messages.
3. `POST /api/sessions/{id}/send` → `session.Manager.Start` spawns the
   adapter subprocess, attaches a `storeWriter` that both persists
   assistant text and broadcasts events to all subscribers of the
   session.
4. Subscriber goroutine pumps events into the WebSocket; the React
   chat terminal renders them as text / tool / error / done.

### Send a message (Telegram)

1. User sends `/start <token>` → `auth.Service.VerifyTelegram` checks
   ban + token; on success the bot caches `tg_user_id → token` in
   memory.
2. User sends plain text → bot auto-creates a session if needed,
   subscribes to the session's event channel, then calls
   `session.Manager.Start`.
3. A goroutine `telegramPump` reads from the subscriber channel and
   pushes messages back to the Telegram chat (chunked to 4000 chars).

## Adapter contract

Each adapter is a single subprocess per session. The base adapter
provides:

- `exec.CommandContext` with the configured binary and args.
- stdin pipe receives the user prompt as plain text (or JSON for
  adapters that prefer it).
- stdout is line-scanned; each line goes through a per-adapter parser
  that returns zero or more `Event` values.
- stderr is forwarded as `Event{Type:"error"}`.
- On context cancellation the subprocess is killed.
- A final `Event{Type:"done"}` is always emitted.

Adapters are intentionally thin — most of the intelligence lives in
the CLI binary itself. We only standardize the event stream.

## Adding a new adapter

1. Add a struct embedding `baseAdapter` in `adapters.go`.
2. In the constructor set `id`, `display`, `bin`, `args`, `env`.
3. Implement `Run(ctx, spec, w)` by calling `b.run(ctx, spec, w, parser)`
   where `parser` is a `func(line string) []Event`. Use `jsonEvent` for
   JSON-line protocols, `plainLine` for free-text protocols.
4. Register in `main.go` and add the agent id to `agentsList()` in
   `middleware.go`.

## Persistence

A single SQLite file (`OMNI_DB_PATH`, default `./omniagent.db`) holds
five tables:

- `sessions` — one row per chat session.
- `messages` — append-only log of user/assistant/tool messages.
- `ban` — fail counters + ban windows keyed by IP or `tg:<user_id>`.
- `audit` — every auth / send / file event with ok/fail flag.
- `usage` — per-request token counts and durations.

Schema is auto-migrated on startup; no manual SQL needed.

## WebSocket message format

Server → client JSON:

```jsonc
{ "type": "replay",   "role": "user|assistant|tool", "content": "...", "tool_name": "Read" }
{ "type": "ready" }                                          // end of replay
{ "type": "text",     "text": "..." }                        // assistant stream chunk
{ "type": "tool",     "tool_name": "Edit", "tool_input": "{…}" }
{ "type": "error",    "text": "..." }
{ "type": "done" }                                           // run finished
```

Client → server: nothing is sent over WS; new prompts go via
`POST /api/sessions/{id}/send`.

## Failure modes

- **Adapter binary missing** → `Available()` returns false, hidden from
  `/api/agents` if you wire that up (currently all 5 are listed; the
  registry can be extended to filter).
- **Subprocess hangs** → `OMNI_AGENT_TIMEOUT` (default 600s) cancels the
  context, kills the process, emits `done`.
- **DB locked** → SQLite WAL mode + 5s busy timeout; concurrent writes
  are rare (one per session).
- **Telegram API down** → bot poller retries with backoff built into
  `telebot.v3`.
- **Turnstile down** → login fails; users see the error. Disable via
  `OMNI_TURNSTILE_ENABLED=false` for dev.
