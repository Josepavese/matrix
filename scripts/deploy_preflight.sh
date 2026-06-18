#!/usr/bin/env bash
set -euo pipefail

echo "== Matrix deploy preflight =="

echo
echo "== Git state =="
git status --short

echo
echo "== Required tools =="
command -v golangci-lint >/dev/null
golangci-lint --version
command -v goreleaser >/dev/null
goreleaser --version

echo
echo "== Governance manifest =="
go run ./scripts/governance_check --manifest governance/manifest.toml

echo
echo "== Go formatting =="
unformatted="$(gofmt -l cmd internal pkg scripts tests)"
if [[ -n "$unformatted" ]]; then
  echo "Go files need formatting. Format the listed files before deploying." >&2
  printf '%s\n' "$unformatted" >&2
  exit 1
fi

echo
echo "== Code governance =="
go run ./scripts/code_governance.go --config code-governance.toml

echo
echo "== Lint =="
golangci-lint run

echo
echo "== Tests =="
go test -race -v ./...

echo
echo "== Build =="
go build ./cmd/matrix

echo
echo "== Orchestration profile =="
preflight_home="$(mktemp -d)"
cleanup_preflight_home() {
  rm -rf "$preflight_home"
}
trap cleanup_preflight_home EXIT
mkdir -p "$preflight_home/configs"
cp -R configs/. "$preflight_home/configs/"
MATRIX_HOME="$preflight_home" go run ./cmd/matrix orchestration capabilities >/tmp/matrix-orchestration-capabilities.json
cleanup_preflight_home
trap - EXIT

echo
echo "== GoReleaser config =="
goreleaser check

echo
echo "DEPLOY_PREFLIGHT_OK"
