#!/usr/bin/env bash
# Builds the mykeebs-migrate CLI into bin/. sam build only packages the Lambda, so
# this standalone operational tool needs its own build step.
set -euo pipefail

version="$(git describe --tags --always --dirty 2>/dev/null || echo dev)"
mkdir -p bin
go build -ldflags "-X main.Version=${version}" -o bin/mykeebs-migrate ./cmd/mykeebs-migrate
echo "built bin/mykeebs-migrate (${version})"
