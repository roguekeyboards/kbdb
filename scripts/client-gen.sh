#!/usr/bin/env bash
set -euo pipefail

# Generates a TypeScript client from api/openapi.yaml. Locally (mise run
# client-gen), output goes to a gitignored ts-client/ scratch dir (named
# per-client since other language clients may be generated alongside it
# later) with no version stamp, for sanity-checking generator output/config
# by hand. In CI (.github/workflows/publish-clients.yml), CLIENT_GEN_VERSION
# and CLIENT_GEN_OUT_DIR are set to stamp the package.json version from the
# release tag before `npm publish`.

cd "$(dirname "${BASH_SOURCE[0]}")/.."

OUT_DIR="${CLIENT_GEN_OUT_DIR:-ts-client}"
GENERATOR_IMAGE="openapitools/openapi-generator-cli:v7.24.0"
ADDITIONAL_PROPS="npmName=@roguekeyboards/kbdb-api-client,supportsES6=true"
if [ -n "${CLIENT_GEN_VERSION:-}" ]; then
  ADDITIONAL_PROPS="${ADDITIONAL_PROPS},npmVersion=${CLIENT_GEN_VERSION}"
fi

rm -rf "$OUT_DIR"
mkdir -p "$OUT_DIR"

docker run --rm \
  --user "$(id -u):$(id -g)" \
  -v "$PWD:/local" \
  "$GENERATOR_IMAGE" generate \
  -i "/local/api/openapi.yaml" \
  -g typescript-fetch \
  -o "/local/$OUT_DIR" \
  --git-user-id roguekeyboards \
  --git-repo-id kbdb \
  --additional-properties="$ADDITIONAL_PROPS"

echo "Generated TypeScript client at $OUT_DIR/"
