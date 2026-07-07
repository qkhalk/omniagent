# OmniAgent

- `backend/` — Go backend (HTTP + WebSocket + Telegram bot)
- `web/` — Next.js 16 PWA frontend (static export)
- `Dockerfile` + `docker-compose.yml` — single-container deployment
- `README.md` / `ARCHITECTURE.md` / `SECURITY.md` — docs
- `backend/.env.example` — full env reference
- `web/.env.example` — frontend env reference

## Quickstart

```bash
# 1. Configure
cp backend/.env.example backend/.env
# edit OMNI_COOKIE_SECRET, OMNI_USER_TOKENS, OMNI_WORKSPACE, OMNI_TELEGRAM_TOKEN…

# 2. Build (backend + frontend)
cd backend && go build -o ../bin/omniagent ./cmd/omniagent && cd ..
cd web && npm install && npm run build && cd ..

# 3. Run
OMNI_WEB_DIR=$(pwd)/web/out ./bin/omniagent -env backend/.env
```

Open http://localhost:8787/

## Telegram

Message your bot with `/start <token>` where `<token>` is one of the tokens
listed in `OMNI_USER_TOKENS`. Then chat naturally.

## Docker

```bash
docker compose up --build -d
```
