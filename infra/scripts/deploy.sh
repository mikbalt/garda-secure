#!/bin/sh
# Production deploy of a released version (INF-C1/C5, INF-M4). Called by the
# CD workflow over SSH after it checked out the tag; can be run by hand:
#   cd /opt/securepatrol && git fetch --tags && git checkout --detach refs/tags/v1.2.3
#   infra/scripts/deploy.sh v1.2.3
# Steps: verify tag == HEAD and a clean work tree, preflight, pull the CI-built
# images (IMAGE_REPO, default from .env), optionally verify their digests,
# dump both databases (pre-migration backup), `up --wait`, keycloak-config,
# record the version, keep the last 3 image versions.
# Environment (optional):
#   API_DIGEST / WEB_DIGEST / OFFSITE_DIGEST  sha256:… expected image digests
#   GHCR_USER / GHCR_TOKEN                    short-lived registry login
#   SKIP_PRE_DEPLOY_BACKUP=1                  (not recommended)
# Rollback: docs/runbook.md §7.
set -eu
. "$(dirname -- "$0")/lib.sh"

version=${1:-}
printf '%s' "$version" | grep -Eq '^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.]+)?$' \
  || die "usage: $0 vMAJOR.MINOR.PATCH[-pre] (got '$version')"

repo_root=$(cd "$INFRA_DIR/.." && pwd)
cd "$repo_root"
[ -z "$(git status --porcelain --untracked-files=no)" ] \
  || die "the work tree has local changes — refusing to deploy over them (git status)"
head=$(git rev-parse HEAD)
tag_commit=$(git rev-parse "refs/tags/$version^{commit}" 2>/dev/null) || die "tag $version not found (git fetch --tags)"
[ "$head" = "$tag_commit" ] || die "HEAD ($head) is not tag $version ($tag_commit)"

cd "$INFRA_DIR"
sh scripts/preflight.sh

load_env "$INFRA_DIR/.env"
IMAGE_REPO=${IMAGE_REPO:-securepatrol}
case "$IMAGE_REPO" in securepatrol) die "IMAGE_REPO must point at the registry (e.g. ghcr.io/<owner>/securepatrol) for deploys" ;; esac
VERSION=$version
export IMAGE_REPO VERSION

state="$INFRA_DIR/.deploy"
mkdir -p "$state"
previous=$(cat "$state/current" 2>/dev/null || echo none)
info "deploying $version (previous: $previous) from $IMAGE_REPO"

if [ -n "${GHCR_TOKEN:-}" ]; then
  registry=${IMAGE_REPO%%/*}
  printf '%s' "$GHCR_TOKEN" | docker login "$registry" -u "${GHCR_USER:-github-actions}" --password-stdin >/dev/null
  trap 'docker logout "$registry" >/dev/null 2>&1 || true' EXIT
fi

info "pulling images"
compose pull --quiet

verify_digest() { # image expected
  [ -n "$2" ] || return 0
  if ! docker image inspect --format '{{range .RepoDigests}}{{println .}}{{end}}' "$1" | grep -q "@$2\$"; then
    die "$1 does not have the expected digest $2"
  fi
  info "digest ok: $1@$2"
}
verify_digest "$IMAGE_REPO/api:$VERSION" "${API_DIGEST:-}"
verify_digest "$IMAGE_REPO/web:$VERSION" "${WEB_DIGEST:-}"
verify_digest "$IMAGE_REPO/backup-offsite:$VERSION" "${OFFSITE_DIGEST:-}"

if [ "${SKIP_PRE_DEPLOY_BACKUP:-0}" != 1 ]; then
  info "pre-deploy database backup"
  sh scripts/backup-db.sh "pre-$version-from-$previous"
fi

info "starting $version"
if ! compose up -d --no-build --remove-orphans --wait --wait-timeout 600; then
  compose ps -a || true
  echo
  echo "DEPLOY FAILED. To roll back:" >&2
  echo "  1. git checkout --detach refs/tags/$previous && infra/scripts/deploy.sh $previous" >&2
  echo "  2. if $version ran DB migrations, restore the pre-deploy dump first:" >&2
  echo "     infra/scripts/restore-db.sh app infra/backups/<newest>-pre-$version-*/$POSTGRES_DB.dump" >&2
  exit 1
fi

sh scripts/keycloak-config.sh

# Pin the running version in .env so manual `docker compose ...` commands use
# the same images (.env is not tracked by git).
set_env() { # key value
  if grep -q "^$1=" .env; then
    sed -i "s|^$1=.*|$1=$2|" .env
  else
    printf '%s=%s\n' "$1" "$2" >> .env
  fi
}
set_env VERSION "$version"
set_env IMAGE_REPO "$IMAGE_REPO"

echo "$previous" > "$state/previous"
echo "$version" > "$state/current"
echo "$(date -u +%Y-%m-%dT%H:%M:%SZ) $version $(git -C "$repo_root" rev-parse --short HEAD) from $previous" >> "$state/history"

# Keep the images of the 3 newest versions (current + 2 for rollback); never
# the previous one. Only dangling layers are pruned otherwise.
for name in api web backup-offsite; do
  docker image ls --format '{{.CreatedAt}}\t{{.Repository}}:{{.Tag}}' "$IMAGE_REPO/$name" \
    | sort -r | cut -f2 | tail -n +4 | while IFS= read -r img; do
      case "$img" in *":$version"|*":$previous"|*":<none>") continue ;; esac
      docker image rm "$img" >/dev/null 2>&1 && info "removed old image $img" || true
    done
done
docker image prune -f >/dev/null

info "deploy of $version complete"
