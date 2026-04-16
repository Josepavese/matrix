#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repo_root"

echo "== Matrix local deploy =="

echo
echo "== Preflight =="
bash scripts/deploy_preflight.sh

echo
echo "== Generate release artifacts =="
goreleaser release --snapshot --clean

echo
echo "== Install generated artifact into local PAL home =="
bash scripts/deploy_local_install.sh

echo
echo "LOCAL_DEPLOY_OK"
