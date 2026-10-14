#!/usr/bin/env bash
# Runs a kbdb-moderation subcommand against an environment's stack:
#   moderation.sh <subcommand> [env] [args...]
# env defaults to dev-$(whoami). An image key always contains a "/" and a
# flag starts with "-", so a first argument that's neither is the env.
set -euo pipefail

subcommand="$1"
shift
env=""
if [ $# -gt 0 ] && [[ "$1" != -* && "$1" != */* ]]; then
  env="$1"
  shift
fi

# shellcheck source=scripts/samconfig-env.sh
source "$(dirname "$0")/samconfig-env.sh" "$env"

go run ./cmd/kbdb-moderation --stack "$STACK_NAME" --region "$REGION" "$subcommand" "$@"
