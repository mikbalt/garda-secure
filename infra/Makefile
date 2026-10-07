# SecurePatrol operations shortcuts (run from infra/). See docs/runbook.md.
COMPOSE      = docker compose
COMPOSE_FULL = $(COMPOSE) -f docker-compose.yml -f docker-compose.app.yml

.PHONY: help certs up up-infra down logs ps build restart

help:
	@echo "make certs     - create a self-signed TLS cert in nginx/certs (dev/LAN)"
	@echo "make up        - build and start the full stack"
	@echo "make up-infra  - start infrastructure only (for local backend/web dev)"
	@echo "make down      - stop everything (data volumes are kept)"
	@echo "make logs      - follow api + worker logs"
	@echo "make ps        - service status"

certs:
	mkdir -p nginx/certs
	openssl req -x509 -nodes -days 825 -newkey rsa:2048 \
	  -keyout nginx/certs/key.pem -out nginx/certs/cert.pem \
	  -subj "/CN=localhost" -addext "subjectAltName=DNS:localhost,IP:127.0.0.1"

up:
	@test -f .env || (echo "missing infra/.env — copy .env.example first" && exit 1)
	@test -f nginx/certs/cert.pem || (echo "missing TLS cert — run 'make certs'" && exit 1)
	$(COMPOSE_FULL) up -d --build

up-infra:
	@test -f .env || (echo "missing infra/.env — copy .env.example first" && exit 1)
	$(COMPOSE) up -d

down:
	$(COMPOSE_FULL) down

logs:
	$(COMPOSE_FULL) logs -f --tail=200 api notification-worker

ps:
	$(COMPOSE_FULL) ps

build:
	$(COMPOSE_FULL) build

restart:
	$(COMPOSE_FULL) restart api notification-worker web
