#!/usr/bin/env bash
set -euo pipefail
packages=()
while IFS= read -r pkg; do
  packages+=("$pkg")
done < <(go list ./... | grep -v /test/functional)
go test "${packages[@]}"
