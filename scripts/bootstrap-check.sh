#!/usr/bin/env bash
set -euo pipefail

# "Behind" means the live template matches an earlier version of the file.
# Matching no version means it was deployed from an unmerged branch, which
# only warns.

case "${1:-}" in
  ci) stacks=(kbdb-stack-boundary=bootstrap/stack-boundary.yaml kbdb-ci-oidc=bootstrap/ci-oidc-role.yaml) ;;
  prod) stacks=(kbdb-stack-boundary=bootstrap/stack-boundary.yaml kbdb-prod-oidc=bootstrap/prod-oidc-role.yaml) ;;
  *)
    echo "usage: mise run bootstrap-check <ci|prod>" >&2
    exit 2
    ;;
esac

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
  live=$(aws cloudformation get-template --stack-name "$stack" --template-stage Original \
    --query TemplateBody --output text | normalize)

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
    echo "$stack is behind $file (it was deployed from ${deployed:0:7}). Deploy $file to $stack with an admin profile first (CONTRIBUTING.md, First-time account bootstrap)." >&2
    status=1
  else
    echo "$stack matches no version of $file, so it was deployed from an unmerged branch. Continuing." >&2
  fi
done
exit "$status"
