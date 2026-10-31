#!/usr/bin/env bash
set -euo pipefail

# shellcheck source=scripts/samconfig-env.sh
source "$(dirname "$0")/samconfig-env.sh"

if [ "$MYKEEBS_ENV" = "prod" ]; then
  echo "Refusing to tear down prod." >&2
  exit 1
fi

BOOTSTRAP_STACK="kbdb-ecr-bootstrap-${STACK_NAME}"

# CloudFormation can't delete a non-empty bucket.
BUCKET=$(aws cloudformation describe-stacks --stack-name "$STACK_NAME" --region "$REGION" \
  --query "Stacks[0].Outputs[?OutputKey=='ImagesBucketName'].OutputValue" \
  --output text 2>/dev/null || true)
if [ -n "$BUCKET" ] && [ "$BUCKET" != "None" ]; then
  aws s3 rm "s3://$BUCKET" --recursive --region "$REGION"
fi

echo "Deleting $STACK_NAME..."
aws cloudformation delete-stack --stack-name "$STACK_NAME" --region "$REGION"
aws cloudformation wait stack-delete-complete --stack-name "$STACK_NAME" --region "$REGION"

# The repo has DeletionPolicy: Retain, so deleting its stack leaves it behind.
echo "Deleting $BOOTSTRAP_STACK and $REPO_NAME..."
aws cloudformation delete-stack --stack-name "$BOOTSTRAP_STACK" --region "$REGION"
aws cloudformation wait stack-delete-complete --stack-name "$BOOTSTRAP_STACK" --region "$REGION"
if aws ecr describe-repositories --repository-names "$REPO_NAME" --region "$REGION" >/dev/null 2>&1; then
  aws ecr delete-repository --repository-name "$REPO_NAME" --force --region "$REGION" >/dev/null
fi

echo "$MYKEEBS_ENV torn down."
