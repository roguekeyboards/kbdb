#!/usr/bin/env bash
set -euo pipefail

# "Behind" means the live template matches an earlier version of the file.
# Matching no version means it was deployed from an unmerged branch, which
# only warns.

STACKS=bootstrap/stacks.yaml
env="${1:-}"
if [ -z "$env" ] || [ "$(yq ".accounts | has(\"$env\")" "$STACKS")" != true ]; then
  echo "usage: mise run bootstrap-check <dev|ci|prod>" >&2
  exit 2
fi
stacks=()
while IFS= read -r pair; do
  stacks+=("$pair")
done < <(yq ".stacks[] | select(.accounts | contains([\"$env\"])) | .name + \"=\" + .template" "$STACKS")

if [ "$(git rev-parse --is-shallow-repository)" = true ]; then
  echo "bootstrap-check needs full git history; this clone is shallow (git fetch --unshallow)." >&2
  exit 2
fi

# CloudFormation won't deploy a change to only comments, formatting or the
# template Description, so the live template can't pick one up.
normalize() { yq 'del(.Description) | ... comments=""'; }

status=0
for pair in "${stacks[@]}"; do
  stack=${pair%%=*}
  file=${pair#*=}
  if ! body=$(aws cloudformation get-template --stack-name "$stack" --template-stage Original \
    --query TemplateBody --output text 2>&1); then
    if [[ "$body" == *"does not exist"* ]]; then
      echo "$stack doesn't exist yet. Run mise run bootstrap $env with an admin profile first." >&2
      status=1
      continue
    fi
    echo "$body" >&2
    exit 1
  fi
  live=$(normalize <<<"$body")

  if [ "$live" = "$(normalize <"$file")" ]; then
    echo "$stack matches $file"
    continue
  fi

  deployed=
  for commit in $(git log --format=%H -- "$file"); do
    if [ "$live" = "$(git show "$commit:$file" | normalize)" ]; then
      deployed=$commit
      break
    fi
  done

  if [ -n "$deployed" ]; then
    echo "$stack is behind $file (it was deployed from ${deployed:0:7}). Run mise run bootstrap $env with an admin profile first." >&2
    status=1
  else
    echo "$stack matches no version of $file, so it was deployed from an unmerged branch. Continuing." >&2
  fi
done
exit "$status"
