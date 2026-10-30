#!/usr/bin/env bash
set -euo pipefail

scripts/gen.sh

if ! git diff --exit-code --stat -- go.mod go.sum '**/mocks/*.go' internal/handlers/api/api.gen.go api/gateway.gen.yaml ; then
  echo
  echo "Generated files are out of date - run the commands above locally and commit the result." >&2
  exit 1
fi

