# SecurePatrol operations shortcuts (run from infra/). See docs/runbook.md.
# COMPOSE_FILE in .env selects the full stack, so plain `docker compose` works.
COMPOSE       = docker compose
COMPOSE_INFRA = $(COMPOSE) -f docker-compose.yml

# `make env SERVER_HOST=...` / `make certs SERVER_HOST=...` (certs falls back
# to SERVER_HOST from .env).
SERVER_HOST ?=

.PHONY: help env certs certs-renew preflight up up-infra down logs ps build restart \
        keycloak-config backup-db offsite-backup offsite-check offsite-snapshots

help:
	@echo "make env SERVER_HOST=<dns|ip> - create .env with random secrets (once)"
	@echo "make certs [SERVER_HOST=...]  - private CA + server cert for SERVER_HOST (nginx/certs)"
	@echo "make certs-renew              - new server cert from the existing CA"
	@echo "make preflight                - check .env, URLs, secrets and certificate"
	@echo "make up                       - preflight, start the full stack, configure Keycloak"
	@echo "make up-infra                 - infrastructure only (local backend/web dev)"
	@echo "make keycloak-config          - sync redirect URIs + OTP policy into running realms"
	@echo "make backup-db                - dump app + Keycloak DBs to infra/backups/"
	@echo "make offsite-backup           - run the off-site (restic) backup now"
	@echo "make offsite-check            - verify the off-site repository (reads 10 % of data)"
	@echo "make down / logs / ps / restart"

env:
	@test -n "$(SERVER_HOST)" || (echo "usage: make env SERVER_HOST=<dns-name-or-ip>" && exit 1)
	sh scripts/gen-env.sh "$(SERVER_HOST)"

certs:
	sh scripts/certs.sh $(SERVER_HOST)

certs-renew:
	sh scripts/certs.sh --renew $(SERVER_HOST)
	-$(COMPOSE) exec nginx nginx -s reload

preflight:
	sh scripts/preflight.sh

# Local builds (VERSION=dev) build the images; a server deployed from CI images
# (scripts/deploy.sh pins VERSION/IMAGE_REPO in .env) never rebuilds them.
up: preflight
	@if grep -q '^VERSION=dev$$' .env; then \
	  $(COMPOSE) build api web backup-offsite && \
	  $(COMPOSE) up -d --remove-orphans --wait --wait-timeout 600; \
	else \
	  $(COMPOSE) up -d --no-build --remove-orphans --wait --wait-timeout 600; \
	fi
	sh scripts/keycloak-config.sh

up-infra:
	@test -f .env || (echo "missing infra/.env — run: make env SERVER_HOST=localhost" && exit 1)
	$(COMPOSE_INFRA) up -d

keycloak-config:
	sh scripts/keycloak-config.sh

backup-db:
	sh scripts/backup-db.sh manual

offsite-backup:
	$(COMPOSE) exec backup-offsite /usr/local/bin/entrypoint.sh run

offsite-check:
	$(COMPOSE) exec backup-offsite /usr/local/bin/entrypoint.sh check 10%

offsite-snapshots:
	$(COMPOSE) exec backup-offsite /usr/local/bin/entrypoint.sh snapshots

down:
	$(COMPOSE) down

logs:
	$(COMPOSE) logs -f --tail=200 api notification-worker

ps:
	$(COMPOSE) ps

build:
	$(COMPOSE) build api web backup-offsite

restart:
	$(COMPOSE) restart api notification-worker web
