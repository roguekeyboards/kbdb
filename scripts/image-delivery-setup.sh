#!/usr/bin/env bash
# Generates the account's image signing key pair, once, then deploys
# bootstrap/image-delivery.yaml, which registers its public half with
# CloudFront. Acts on whichever account the active AWS credentials belong to.
set -euo pipefail

REGION="${AWS_REGION:-${AWS_DEFAULT_REGION:-us-east-2}}"
PRIVATE_KEY_PARAM=/mykeebs-api/image-delivery/private-key
PUBLIC_KEY_PARAM=/mykeebs-api/image-delivery/public-key

param_exists() {
  local err
  if err=$(aws ssm get-parameter --region "$REGION" --name "$1" 2>&1 >/dev/null); then
    return 0
  fi
  if [[ "$err" == *ParameterNotFound* ]]; then
    return 1
  fi
  echo "$err" >&2
  exit 1
}

ACCOUNT=$(aws sts get-caller-identity --query Account --output text)

has_private=false
has_public=false
param_exists "$PRIVATE_KEY_PARAM" && has_private=true
param_exists "$PUBLIC_KEY_PARAM" && has_public=true

if [[ "$has_private" != "$has_public" ]]; then
  echo "Account $ACCOUNT has only one of $PRIVATE_KEY_PARAM and $PUBLIC_KEY_PARAM. Delete it and rerun." >&2
  exit 1
fi

# Rotation is a manual runbook, so an existing pair is never replaced.
if [[ "$has_private" == true ]]; then
  echo "Account $ACCOUNT already has an image signing key pair."
else
  # ECDSA P-256, signed deterministically (RFC 6979) by
  # internal/repository/cloudfront so an image's URL is stable within its
  # signing window. CloudFront rejects explicit curve parameters, hence
  # named_curve. The key goes to SSM through stdin, never argv or disk.
  private_key=$(openssl genpkey -algorithm EC -pkeyopt ec_paramgen_curve:P-256 \
    -pkeyopt ec_param_enc:named_curve 2>/dev/null)
  public_key=$(openssl pkey -pubout <<<"$private_key")
  printf '%s\n' "$private_key" | aws ssm put-parameter --region "$REGION" \
    --name "$PRIVATE_KEY_PARAM" --type SecureString --value file:///dev/stdin >/dev/null
  printf '%s\n' "$public_key" | aws ssm put-parameter --region "$REGION" \
    --name "$PUBLIC_KEY_PARAM" --type String --value file:///dev/stdin >/dev/null
  echo "Generated an image signing key pair in account $ACCOUNT."
fi

aws cloudformation deploy --region "$REGION" \
  --template-file bootstrap/image-delivery.yaml \
  --stack-name mykeebs-api-image-delivery \
  --no-fail-on-empty-changeset
