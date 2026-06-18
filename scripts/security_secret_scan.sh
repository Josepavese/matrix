#!/usr/bin/env bash
set -euo pipefail

declare -A patterns=(
  [telegram_bot_token]='[0-9]{8,10}:[A-Za-z0-9_-]{35,}'
  [github_token]='gh[pousr]_[A-Za-z0-9_]{36,}'
  [openai_api_key]='(sk-[A-Za-z0-9]{32,}|sk-proj-[A-Za-z0-9_-]{40,})'
  [anthropic_api_key]='sk-ant-[A-Za-z0-9_-]{32,}'
  [google_api_key]='AIza[0-9A-Za-z_-]{35}'
  [slack_token]='xox[baprs]-[0-9A-Za-z-]{20,}'
  [stripe_secret_key]='sk_(live|test)_[0-9A-Za-z]{24,}'
  [jwt_token]='eyJ[A-Za-z0-9_-]{10,}\\.[A-Za-z0-9_-]{10,}\\.[A-Za-z0-9_-]{10,}'
  [aws_access_key]='AKIA[0-9A-Z]{16}'
  [private_key]='-----BEGIN (RSA |EC |OPENSSH |DSA |)?PRIVATE KEY-----'
)

failed=0
for name in "${!patterns[@]}"; do
  matches="$(
    git grep -I -n -E -e "${patterns[$name]}" -- \
      . \
      ':!go.sum' \
      ':!*.sum' \
      ':!dist/**' \
      ':!scripts/security_secret_scan.sh' || true
  )"
  if [[ -n "$matches" ]]; then
    echo "::error title=Potential secret detected::$name"
    printf '%s\n' "$matches" | sed -E 's/^([^:]+:[0-9]+:).*/\1<redacted>/'
    failed=1
  fi
done

if [[ "$failed" -ne 0 ]]; then
  echo "Potential secrets detected. Revoke exposed credentials, remove them from the tree, and rewrite history if already pushed." >&2
  exit 1
fi

echo "No high-confidence secrets detected in tracked files."
