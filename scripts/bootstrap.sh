#!/usr/bin/env bash
# Deploys an account's bootstrap stacks from bootstrap/stacks.yaml, in
# order: mise run bootstrap <dev|ci|prod>. Needs that account's admin
# credentials. func-setup.sh runs it for floci.
set -euo pipefail

STACKS=bootstrap/stacks.yaml
env="${1:-}"
want=$(yq ".accounts.\"$env\" // \"\"" "$STACKS")
if [ -z "$env" ] || [ -z "$want" ]; then
  echo "usage: mise run bootstrap <dev|ci|prod>" >&2
  exit 2
fi
have=$(aws sts get-caller-identity --query Account --output text)
if [ "$have" != "$want" ]; then
  echo "These credentials are for account $have, not $env's $want." >&2
  exit 1
fi

region="${AWS_REGION:-${AWS_DEFAULT_REGION:-us-east-2}}"
count=$(yq '.stacks | length' "$STACKS")
for ((i = 0; i < count; i++)); do
  stack=".stacks[$i]"
  if [ "$(yq "$stack.accounts | contains([\"$env\"])" "$STACKS")" != true ]; then
    continue
  fi
  name=$(yq "$stack.name" "$STACKS")
  echo "Deploying $name"

  script=$(yq "$stack.script // \"\"" "$STACKS")
  if [ -n "$script" ]; then
    AWS_REGION="$region" "$script"
    continue
  fi

  args=(--region "$region" --stack-name "$name" --template-file "$(yq "$stack.template" "$STACKS")" --no-fail-on-empty-changeset)
  capabilities=$(yq "$stack.capabilities // \"\"" "$STACKS")
  if [ -n "$capabilities" ]; then
    args+=(--capabilities "$capabilities")
  fi
  overrides=()
  while IFS= read -r kv; do
    overrides+=("$kv")
  done < <(yq "$stack.parameters.\"$env\" // {} | to_entries | .[] | .key + \"=\" + .value" "$STACKS")
  if [ ${#overrides[@]} -gt 0 ]; then
    args+=(--parameter-overrides "${overrides[@]}")
  fi
  aws cloudformation deploy "${args[@]}"
done
