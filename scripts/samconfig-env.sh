# shellcheck shell=bash disable=SC2034
# Sourced by setup.sh, deploy.sh and teardown.sh. Resolves the <env> argument
# to its samconfig.toml section and exits unless the active AWS credentials
# belong to the account that section deploys to.
#
# Sets MYKEEBS_ENV, STACK_NAME, REGION, S3_BUCKET, REPO_URI and REPO_NAME for the
# caller (hence SC2034 disabled above).

MYKEEBS_ENV="${1:-dev-$(whoami)}"

samconfig() {
  yq -p toml -o yaml ".[\"$MYKEEBS_ENV\"].deploy.parameters.$1 // \"\"" samconfig.toml
}

STACK_NAME="$(samconfig stack_name)"
REGION="$(samconfig region)"
S3_BUCKET="$(samconfig s3_bucket)"
REPO_URI="$(samconfig 'image_repositories[0]')"
REPO_URI="${REPO_URI#ApiFunction=}"
REPO_NAME="${REPO_URI##*/}"

if [ -z "$STACK_NAME" ] || [ -z "$REGION" ] || [ -z "$S3_BUCKET" ] || [ -z "$REPO_URI" ]; then
  echo "samconfig.toml has no complete [$MYKEEBS_ENV.deploy.parameters] section (needs stack_name, region, s3_bucket, image_repositories)." >&2
  exit 1
fi

EXPECTED_ACCOUNT="${S3_BUCKET#kbdb-sam-artifacts-}"
ACCOUNT="$(aws sts get-caller-identity --query Account --output text)"
if [ "$ACCOUNT" != "$EXPECTED_ACCOUNT" ]; then
  echo "$MYKEEBS_ENV deploys to account $EXPECTED_ACCOUNT, but the active AWS credentials are for $ACCOUNT." >&2
  exit 1
fi
