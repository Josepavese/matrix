#!/usr/bin/env bash
set -euo pipefail

echo "== Matrix deploy preflight =="

echo
echo "== Git state =="
git status --short

echo
echo "== Go formatting =="
gofmt -w cmd internal pkg scripts tests

echo
echo "== Code governance =="
go run ./scripts/code_governance.go --config code-governance.toml

echo
echo "== Tests =="
go test -race -v ./...

echo
echo "== Build =="
go build ./cmd/matrix

echo
echo "== Orchestration profile =="
go run ./cmd/matrix orchestration capabilities >/tmp/matrix-orchestration-capabilities.json

echo
echo "== GoReleaser config =="
if command -v goreleaser >/dev/null 2>&1; then
  goreleaser check
else
  echo "goreleaser not found; skipping local goreleaser check"
fi

echo
echo "DEPLOY_PREFLIGHT_OK"
