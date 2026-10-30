#!/usr/bin/env bash
set -euo pipefail

mockery
go tool oapi-codegen -config .oapi-codegen.yml api/openapi.yaml
go run ./api/gen
go run ./brand/gen
scripts/idp-sdk.sh
go mod tidy
