#!/usr/bin/env bash
# Points an environment's custom domains at its stack: a DNS-only CNAME in
# Cloudflare for CustomDomainName and ImagesDomainName, whichever its
# samconfig.toml section sets. Needs cf auth login. Run by setup.sh after
# the first deploy; safe to rerun.
set -euo pipefail

# shellcheck source=scripts/samconfig-env.sh
source "$(dirname "$0")/samconfig-env.sh"

overrides="$(samconfig parameter_overrides)"
param() {
  grep -o "$1=[^ \\]*" <<<"$overrides" | head -1 | cut -d= -f2- || true
}
output() {
  local value
  value=$(aws cloudformation describe-stacks --region "$REGION" --stack-name "$STACK_NAME" \
    --query "Stacks[0].Outputs[?OutputKey=='$1'].OutputValue" --output text)
  if [ -z "$value" ] || [ "$value" = None ]; then
    echo "$STACK_NAME has no $1 output; deploy it first." >&2
    exit 1
  fi
  echo "$value"
}

point() {
  local name="$1" target="$2"
  local zone="${name#"${name%.*.*}".}"
  local existing
  existing=$(cf dns records list -z "$zone" --name "$name" --type CNAME)
  if [ "$(yq -p json 'length' <<<"$existing")" -gt 0 ]; then
    if [ "$(yq -p json '.[0].content + " " + (.[0].proxied | tostring)' <<<"$existing")" = "$target false" ]; then
      echo "$name already points at $target"
      return
    fi
    cf dns records edit -z "$zone" "$(yq -p json '.[0].id' <<<"$existing")" \
      --body "{\"type\":\"CNAME\",\"name\":\"$name\",\"content\":\"$target\",\"proxied\":false}" >/dev/null
    echo "Repointed $name at $target"
    return
  fi
  cf dns records create -z "$zone" \
    --body "{\"type\":\"CNAME\",\"name\":\"$name\",\"content\":\"$target\",\"proxied\":false,\"ttl\":1,\"comment\":\"$STACK_NAME\"}" >/dev/null
  echo "Pointed $name at $target"
}

api="$(param CustomDomainName)"
if [ -n "$api" ]; then
  point "$api" "$(output ApiCustomDomainTarget)"
fi
images="$(param ImagesDomainName)"
if [ -n "$images" ]; then
  point "$images" "$(output ImagesDomainTarget)"
fi
