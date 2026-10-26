#!/usr/bin/env bash
# Generates the account's image signing key pairs, once each, then deploys
# bootstrap/image-delivery.yaml, which registers their public halves with
# CloudFront. Acts on whichever account the active AWS credentials belong to.
set -euo pipefail

REGION="${AWS_REGION:-${AWS_DEFAULT_REGION:-us-east-2}}"

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

# ensure_key_pair <private param> <public param> <openssl genpkey args...>
# Rotation is a manual runbook, so an existing pair is never replaced.
ensure_key_pair() {
  local private_param=$1 public_param=$2
  shift 2

  local has_private=false has_public=false
  param_exists "$private_param" && has_private=true
  param_exists "$public_param" && has_public=true

  if [[ "$has_private" != "$has_public" ]]; then
    echo "Account $ACCOUNT has only one of $private_param and $public_param. Delete it and rerun." >&2
    exit 1
  fi

  if [[ "$has_private" == true ]]; then
    echo "Account $ACCOUNT already has $private_param."
    return
  fi

  # The key goes to SSM through stdin, never argv or disk.
  local private_key public_key
  private_key=$(openssl genpkey "$@" 2>/dev/null)
  public_key=$(openssl pkey -pubout <<<"$private_key")
  printf '%s\n' "$private_key" | aws ssm put-parameter --region "$REGION" \
    --name "$private_param" --type SecureString --value file:///dev/stdin >/dev/null
  printf '%s\n' "$public_key" | aws ssm put-parameter --region "$REGION" \
    --name "$public_param" --type String --value file:///dev/stdin >/dev/null
  echo "Generated $private_param in account $ACCOUNT."
}

# The RSA pair is retiring: the key group still trusts it so URLs signed
# with it before the switch to ECDSA keep working until they expire.
ensure_key_pair /kbdb/image-delivery/private-key /kbdb/image-delivery/public-key \
  -algorithm RSA -pkeyopt rsa_keygen_bits:2048
# P-256 is the only ECDSA curve CloudFront key groups accept. internal/
# repository/cloudfront signs deterministically (RFC 6979), so an image's
# URL is stable within its signing window.
ensure_key_pair /kbdb/image-delivery/ecdsa-private-key /kbdb/image-delivery/ecdsa-public-key \
  -algorithm EC -pkeyopt ec_paramgen_curve:P-256 -pkeyopt ec_param_enc:named_curve

aws cloudformation deploy --region "$REGION" \
  --template-file bootstrap/image-delivery.yaml \
  --stack-name kbdb-image-delivery \
  --no-fail-on-empty-changeset
