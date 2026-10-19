#!/usr/bin/env bash
set -euo pipefail

scripts/gen.sh

# brand/dist's images aren't compared here: rasterizing can differ by a shade
# on a pixel or two between CPU architectures, so a Mac-generated PNG never
# matches a CI-generated one byte for byte. brand/internal/assets' tests
# compare them pixel by pixel within a tolerance instead. Its text files
# (SVG, CSS) are deterministic and checked here like everything else.
if ! git diff --exit-code --stat -- go.mod go.sum '**/mocks/*.go' internal/handlers/api/api.gen.go \
  'brand/dist/*.svg' 'brand/dist/*.css'; then
  echo
  echo "Generated files are out of date - run the commands above locally and commit the result." >&2
  exit 1
fi

added=$(git ls-files --others --exclude-standard -- brand/dist)
removed=$(git diff --name-only --diff-filter=D -- brand/dist)
if [ -n "$added$removed" ]; then
  echo "brand/dist gained or lost files - run mise run gen locally and commit the result:" >&2
  printf '%s\n' "$added" "$removed" | sed '/^$/d' >&2
  exit 1
fi
