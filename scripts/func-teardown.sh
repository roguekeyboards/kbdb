#!/usr/bin/env bash
set -uo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ENV_FILE="$ROOT/.env.floci"

if [[ -f "$ENV_FILE" ]]; then
  # shellcheck disable=SC1090
  source "$ENV_FILE"
  [[ -n "${KBDB_OIDC_GEN_DIR:-}" ]] && rm -rf "$KBDB_OIDC_GEN_DIR"
fi

# floci starts its own ECR registry container and volume through the mounted
# Docker socket, outside compose, and leaves them behind on shutdown. Stop
# floci first so it can't start another, remove them, then let compose remove
# the network they were holding, so every func-setup starts from a clean slate.
docker compose -f docker-compose.floci.yml stop
CONTAINERS=$(docker ps -aq --filter label=floci=true)
# shellcheck disable=SC2086 # IDs, split on purpose
[[ -n "$CONTAINERS" ]] && docker rm -f $CONTAINERS >/dev/null
VOLUMES=$(docker volume ls -q --filter label=floci=true)
# shellcheck disable=SC2086
[[ -n "$VOLUMES" ]] && docker volume rm $VOLUMES >/dev/null
docker compose -f docker-compose.floci.yml down
rm -f "$ENV_FILE"
