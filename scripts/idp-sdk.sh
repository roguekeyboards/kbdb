#!/usr/bin/env bash
set -euo pipefail

# Copies the IdP browser SDK pinned in internal/consent/package-lock.json
# into internal/consent/idp-sdk.js, which the consent page embeds. Renovate
# runs this after bumping the SDK, so its PR carries the new file.
cd "$(dirname "$0")/../internal/consent"
npm ci --ignore-scripts --no-audit --no-fund --loglevel=error
cp node_modules/@stytch/vanilla-js/dist/iife/index.js idp-sdk.js
