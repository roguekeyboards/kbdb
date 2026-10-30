#!/usr/bin/env bash
set -euo pipefail

mockery
go tool oapi-codegen -config .oapi-codegen.yml api/openapi.yaml
go run ./api/gen
go mod tidy
