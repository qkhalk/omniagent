# OmniAgent — Security

## Threat model

OmniAgent exposes a multi-CLI agent gateway over HTTP and Telegram. The
highest-value assets are:

1. **User tokens** — bearer credentials that grant full agent access.
2. **Workspace files** — agents can read/edit/delete files under
   `OMNI_WORKSPACE`.
3. **CLI credentials** — `ANTHROPIC_API_KEY`, `OPENAI_API_KEY`, etc.
   These live in the process environment, never in the database.
4. **Server CPU / spend** — agent runs cost real money; an attacker
   who can send prompts can drain your API budgets.

## Layers

### 1. Token + Cloudflare Turnstile

- Each user is issued a 24+ byte hex token stored in `.env`
  (`OMNI_USER_TOKENS`).
- Web login: token + Turnstile token → `auth.Service.Verify`.
- Telegram login: `/start <token>` → `auth.Service.VerifyTelegram` (no
  Turnstile — Telegram is its own identity provider; we rely on the
  user-id ban instead).
- Tokens are compared in constant time (Go's `==` on strings of equal
  length is constant-time; tokens are fixed-length hex).

### 2. Progressive ban

- Each failed auth attempt bumps a counter for the ban key (client IP
  for web, `tg:<user_id>` for Telegram).
- `OMNI_BAN_MAX_FAILS` failures inside `OMNI_BAN_WINDOW_SECONDS`
  triggers `OMNI_BAN_DURATION_SECONDS` ban.
- While banned, every request returns `401 banned` without even
  checking the token.
- Bans are stored in SQLite; survive restarts.
- Admin can unban via `/api/admin/bans/{key}` DELETE.

### 3. Workspace sandbox

- All file APIs go through `adapter.SafeJoin(workspace, rel)` which
  rejects paths containing `..` and forces the result to stay under
  `workspace`.
- Adapters receive `ProjectDir` already validated; they do not parse
  paths themselves.
- `OMNI_WORKSPACE` should be a dedicated directory with no symlinks
  pointing outside. The admin is responsible for setting this up.

### 4. Signed session cookie

- Web sessions use an `HttpOnly` cookie `omni_token = token.hmac[:16]`.
- `hmac` is HMAC-SHA256(`OMNI_COOKIE_SECRET`, token).
- Tampering with the cookie value invalidates verification.
- The cookie is `SameSite=Lax`, `Secure` when TLS is detected.

### 5. Per-user rate limit

- Token bucket per user token: `OMNI_RATE_LIMIT_RPS` sustained,
  `OMNI_RATE_LIMIT_BURST` burst.
- Independent of IP — a single user can't multiply their quota by
  rotating IPs.

### 6. Subprocess isolation

- Each agent run is its own `exec.CommandContext` with `cmd.Dir`
  constrained to the validated project directory.
- The subprocess inherits only `OMNI_ADAPTER_ENV` env vars (which the
- admin populates with API keys), not the entire OmniAgent env.
- `OMNI_AGENT_TIMEOUT` (default 600s) is a hard ceiling; the context
  cancellation kills the process tree.
-stdin/stdout/stderr are pipes; the subprocess cannot write to
  OmniAgent's own files.

### 7. Admin isolation

- `/api/admin/*` requires `Authorization: Bearer <OMNI_ADMIN_TOKEN>`.
- The admin token is separate from user tokens — leaking a user token
  does not expose the admin panel.
- Admin endpoints never return raw user tokens (always masked in audit
  log, e.g. `dev1-d…aaa`).

### 8. Audit log

- Every login attempt (success or failure), every `session.send`,
  every `file.write`, and every ban event is recorded in the `audit`
  table with timestamp, channel, remote id, action, and ok/fail flag.
- The admin panel renders the last 200 entries.

## Operational hardening (recommended)

1. **Terminate TLS at the edge** — Cloudflare, Caddy, or nginx in
   front of OmniAgent. Never expose port 8787 directly to the
   internet over plain HTTP.
2. **Restrict `OMNI_WORKSPACE`** — use a dedicated directory, no
   symlinks, owned by a non-root user. Do NOT point it at `$HOME` or
   `/`.
3. **Rotate tokens** — change `OMNI_USER_TOKENS` periodically; rotate
   `OMNI_COOKIE_SECRET` (which invalidates all web sessions).
4. **Set `OMNI_ADMIN_TOKEN`** — without it the admin panel is fully
   disabled.
5. **Cap adapter spend externally** — set per-key spending limits at
   Anthropic / OpenAI / Google so a compromised token can't drain
   your account. OmniAgent's per-user rate limit slows but does not
   prevent runaway spend.
6. **Run as non-root** — the Go binary does not need root.
7. **Firewall the bot** — if running Telegram polling, the bot only
   needs outbound HTTPS to `api.telegram.org`. No inbound ports
   beyond the HTTP listener.

## Known limitations

- The in-memory `tg_user_id → token` cache in the bot is lost on
  restart; Telegram users must `/start <token>` again after a deploy.
  (A future version will persist this in SQLite.)
- The admin token is sent as a Bearer header and stored in
  `localStorage` in the browser. For higher security, use a
  short-lived admin session instead.
- WebSocket auth uses `?token=<raw_token>` in the URL, which can leak
  via server access logs. Terminate TLS at the edge and do not log
  query strings.
- No CSRF token — the session cookie is `SameSite=Lax` which mitigates
  the common CSRF vectors, but state-changing endpoints should be
  audited if you embed the UI in another origin.

## Reporting a vulnerability

Please open a private GitHub Security Advisory or email the maintainer.
Do not open a public issue for security problems.
