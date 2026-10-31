#!/usr/bin/env bash
set -euo pipefail

# shellcheck source=scripts/samconfig-env.sh
source "$(dirname "$0")/samconfig-env.sh"

# Never pass --parameter-overrides here: SAM replaces the section's
# parameter_overrides wholesale rather than merging.
sam build
sam deploy --config-env "$MYKEEBS_ENV"
