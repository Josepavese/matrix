#!/usr/bin/env bash
# Exercises install.sh's release-asset resolution without touching the network.
#
# Why this exists: the by-tag release document on GitHub can be served with an
# empty asset list while the assets are already downloadable (observed on
# v0.1.36 and v0.1.37). install.sh falls back to the id-addressed endpoint in
# that case, and the fallback cannot be tested against the live API because the
# condition is not reproducible on demand. curl is stubbed here so all three
# outcomes are deterministic.
set -uo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
work="$(mktemp -d)"
if [ "${KEEP_WORK:-0}" = "1" ]; then
  echo "work dir: $work"
else
  trap 'rm -rf "$work"' EXIT
fi

failures=0
check() {
  if [ "$2" = "$3" ]; then
    printf 'ok   %s -> %s\n' "$1" "$2"
  else
    printf 'FAIL %s -> got %s, want %s\n' "$1" "$2" "$3"
    failures=$((failures + 1))
  fi
}

goos="$(uname -s | tr '[:upper:]' '[:lower:]')"
case "$(uname -m)" in
  x86_64 | amd64) goarch="amd64" ;;
  aarch64 | arm64) goarch="arm64" ;;
  *) echo "unsupported test architecture" >&2; exit 1 ;;
esac

# A real archive with a real checksum, so checksum verification runs for real.
# GoReleaser puts the binary at the archive root, and install.sh requires it there.
mkdir -p "$work/payload"
cat >"$work/payload/matrix" <<'EOF'
#!/bin/sh
echo "matrix 9.9.9"
EOF
chmod 0755 "$work/payload/matrix"
tar -czf "$work/archive.tgz" -C "$work/payload" matrix
archive_name="matrix_9.9.9_${goos}_${goarch}.tar.gz"
archive_sha="$(sha256sum "$work/archive.tgz" | cut -d ' ' -f 1)"
printf '%s  %s\n' "$archive_sha" "$archive_name" >"$work/checksums.txt"

base="https://api.github.com/repos/Josepavese/matrix/releases"
asset_url="$base/download/v9.9.9/$archive_name"
checksum_url="$base/download/v9.9.9/checksums.txt"

mkdir -p "$work/fixtures" "$work/stub" "$work/home"
# The stub answers three URLs; anything else is a 404 to catch surprises.
cat >"$work/fixtures/by-tag-stale.json" <<EOF
{"id": 4242, "tag_name": "v9.9.9", "assets": []}
EOF
cat >"$work/fixtures/by-tag-fresh.json" <<EOF
{"id": 4242, "tag_name": "v9.9.9", "assets": [
  {"name": "$archive_name", "browser_download_url": "$asset_url"},
  {"name": "checksums.txt", "browser_download_url": "$checksum_url"}
]}
EOF
cat >"$work/fixtures/by-id-assets.json" <<EOF
[
  {"name": "$archive_name", "browser_download_url": "$asset_url"},
  {"name": "checksums.txt", "browser_download_url": "$checksum_url"}
]
EOF
cat >"$work/fixtures/by-id-empty.json" <<EOF
[]
EOF
cp "$work/archive.tgz" "$work/fixtures/archive.tgz"
cp "$work/checksums.txt" "$work/fixtures/checksums.txt"

# stub curl: prints the fixture for a URL, or copies it when -o is used.
cat >"$work/stub/curl" <<EOF
#!/usr/bin/env bash
out=""
url=""
while [ \$# -gt 0 ]; do
  case "\$1" in
    -o) out="\$2"; shift 2 ;;
    -*) shift ;;
    *) url="\$1"; shift ;;
  esac
done
fixture=""
case "\$url" in
  */releases/latest|*/releases/tags/*) fixture="$work/fixtures/by-tag.json" ;;
  */releases/4242/assets) fixture="$work/fixtures/by-id-assets.json" ;;
  "$asset_url") fixture="$work/fixtures/archive.tgz" ;;
  "$checksum_url") fixture="$work/fixtures/checksums.txt" ;;
esac
if [ -z "\$fixture" ]; then
  echo "stub curl: unexpected url \$url" >&2
  exit 22
fi
if [ -n "\$out" ]; then
  cp "\$fixture" "\$out"
else
  cat "\$fixture"
fi
EOF
chmod 0755 "$work/stub/curl"

run_install() {
  # HOME is redirected so the launcher lands inside the fixture, not in the
  # developer's own ~/.local/bin.
  PATH="$work/stub:$PATH" HOME="$work/home" MATRIX_HOME="$work/matrix-home" \
    MATRIX_VERSION=v9.9.9 bash "$ROOT/install/install.sh" >"$work/out.log" 2>"$work/err.log"
  return $?
}

# 1. Stale by-tag view: the fallback must find the assets and say so.
cp "$work/fixtures/by-tag-stale.json" "$work/fixtures/by-tag.json"
rm -rf "$work/matrix-home"
run_install
check "stale by-tag install exit" "$?" "0"
check "stale by-tag reads by id" "$(grep -c 'stale' "$work/err.log")" "1"
check "stale by-tag installed binary" "$(test -x "$work/matrix-home/bin/matrix" && echo yes || echo no)" "yes"

# 2. Fresh by-tag view: no second call, no warning.
cp "$work/fixtures/by-tag-fresh.json" "$work/fixtures/by-tag.json"
rm -rf "$work/matrix-home"
run_install
check "fresh by-tag install exit" "$?" "0"
check "fresh by-tag has no warning" "$(grep -c 'stale' "$work/err.log")" "0"
check "fresh by-tag installed binary" "$(test -x "$work/matrix-home/bin/matrix" && echo yes || echo no)" "yes"

# 3. Neither view has assets: fail closed instead of installing nothing.
cp "$work/fixtures/by-tag-stale.json" "$work/fixtures/by-tag.json"
cp "$work/fixtures/by-id-empty.json" "$work/fixtures/by-id-assets.json"
rm -rf "$work/matrix-home"
run_install
check "empty release exit" "$?" "1"
check "empty release names the count" "$(grep -c 'found 0' "$work/err.log")" "1"

if [ "$failures" -ne 0 ]; then
  echo "INSTALL_SH_ASSET_TESTS_FAILED ($failures)"
  exit 1
fi
echo "INSTALL_SH_ASSET_TESTS_OK"
