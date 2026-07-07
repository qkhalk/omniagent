.PHONY: build-backend build-web build run dev clean

build: build-backend build-web

build-backend:
	cd backend && go build -trimpath -ldflags="-s -w" -o ../bin/omniagent ./cmd/omniagent

build-web:
	cd web && npm install --no-audit --no-fund && npm run build

run: build
	OMNI_WEB_DIR=$(PWD)/web/out ./bin/omniagent -env backend/.env

dev:
	cd web && npm run dev

clean:
	rm -rf bin web/out web/.next web/node_modules backend/omniagent backend/omniagent.db*

# Generate per-user tokens
tokens:
	@for i in 1 2 3; do \
	  printf "user%d-%s:User %d\n" "$$i" "$$(openssl rand -hex 16)" "$$i"; \
	done

# Generate a strong cookie secret
secret:
	@openssl rand -hex 32
