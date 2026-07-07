# ---- Build stage: Go backend ----
FROM golang:1.23-alpine AS go-builder
WORKDIR /src
RUN apk add --no-cache git gcc musl-dev
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
RUN CGO_ENABLED=1 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/omniagent ./cmd/omniagent

# ---- Build stage: Next.js static export ----
FROM node:20-alpine AS web-builder
WORKDIR /web
COPY web/package.json web/package-lock.json* ./
RUN npm ci --no-audit --no-fund || npm install --no-audit --no-fund
COPY web/ ./
RUN npm run build

# ---- Runtime stage ----
FROM alpine:3.20
RUN apk add --no-cache ca-certificates tini sqlite-libs && \
    addgroup -S omni && adduser -S omni -G omni && \
    mkdir -p /srv/omniagent/workspace /data && chown -R omni:omni /srv/omniagent /data

COPY --from=go-builder /out/omniagent /usr/local/bin/omniagent
COPY --from=web-builder /web/out /srv/omniagent/web

USER omni
WORKDIR /srv/omniagent
ENV OMNI_HOST=0.0.0.0 \
    OMNI_PORT=8787 \
    OMNI_DB_PATH=/data/omniagent.db \
    OMNI_WORKSPACE=/srv/omniagent/workspace \
    OMNI_WEB_DIR=/srv/omniagent/web

EXPOSE 8787
ENTRYPOINT ["/sbin/tini", "--"]
CMD ["omniagent", "-env", "/srv/omniagent/.env"]
