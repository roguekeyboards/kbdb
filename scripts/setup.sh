#!/usr/bin/env bash
set -euo pipefail

# shellcheck source=scripts/samconfig-env.sh
source "$(dirname "$0")/samconfig-env.sh"

aws cloudformation deploy --template-file bootstrap/ecr-repo.yaml \
  --stack-name "kbdb-ecr-bootstrap-${STACK_NAME}" \
  --region "$REGION" \
  --parameter-overrides "RepositoryName=${REPO_NAME}" ExpireBy=count \
  --no-fail-on-empty-changeset

"$(dirname "$0")/deploy.sh" "$KBDB_ENV"
"$(dirname "$0")/dns.sh" "$KBDB_ENV"
