#!/usr/bin/env bash
# Verifies a published GitHub release end to end, so that a release captain runs
# one command instead of checking the asset set, the checksums, and the LICENSE
# inside every archive by hand.
#
# Why the by-tag fallback below is not paranoia: GitHub can serve the
# tag-addressed release document with an empty asset list while the assets are
# already downloadable (observed on v0.1.36 and v0.1.37). That stale index is
# what broke install.sh and install.ps1, and a manual verification that reads it
# signs off a release whose installers cannot resolve their own assets.
#
# Usage:
#   scripts/verify_release.sh [VERSION]
#
#   VERSION defaults to latest; a tag (v0.1.37) or a bare version (0.1.37) is
#   accepted.
#
# Flags:
#   --keep      keep the download directory and print its path
#   -h, --help  print this usage
#
# Environment:
#   MATRIX_REPO   GitHub owner/name. Default: Josepavese/matrix
#   GITHUB_TOKEN  optional; raises the GitHub API rate limit for CI use
#
# Exit status: 0 when every check passed, 1 when any check failed, 2 when the
# local environment cannot run the verification at all.

set -uo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
REPO="${MATRIX_REPO:-Josepavese/matrix}"
API="https://api.github.com/repos/$REPO"

VERSION="latest"
KEEP=0
tag_name=""
release_id=""
version=""

usage() {
  cat <<'USAGE'
Usage: scripts/verify_release.sh [VERSION]

Verifies a published GitHub release of this repository end to end: the asset
set, the sha256 of every archive against checksums.txt, and the LICENSE inside
every archive.

Arguments:
  VERSION        Release tag (v0.1.37) or bare version (0.1.37). Default: latest

Flags:
  --keep         Keep the download directory and print its path
  -h, --help     Print this help

Environment:
  MATRIX_REPO    GitHub owner/name. Default: Josepavese/matrix
  GITHUB_TOKEN   Optional token, for the GitHub API rate limit

Requires curl, tar, python3, and sha256sum (or shasum).

Exit status: 0 all checks passed, 1 a check failed, 2 usage or environment error.
USAGE
}

abort() {
  printf 'verify_release: %s\n' "$1" >&2
  exit 2
}

need() {
  command -v "$1" >/dev/null 2>&1 || abort "missing required command: $1"
}

# The check line shape mirrors tests/install_sh_asset_test.sh so the output of a
# release verification reads like the other gates in this repository.
check() {
  local what="$1" got="$2" want="$3"
  if [ "$got" = "$want" ]; then
    printf 'ok   %s -> %s\n' "$what" "$got"
    checks_ok=$((checks_ok + 1))
  else
    printf 'FAIL %s -> got %s, want %s\n' "$what" "$got" "$want"
    checks_failed=$((checks_failed + 1))
  fi
}

# Records a fact the surrounding control flow already established (a download
# that returned 0, a resolution that produced a tag), in the same ok shape.
note() {
  printf 'ok   %s -> %s\n' "$1" "$2"
  checks_ok=$((checks_ok + 1))
}

while [ $# -gt 0 ]; do
  case "$1" in
    --keep) KEEP=1 ;;
    -h | --help)
      usage
      exit 0
      ;;
    --)
      # Everything after -- is positional, so keep looping instead of stopping.
      shift
      continue
      ;;
    -*) abort "unknown flag: $1 (try --help)" ;;
    *)
      if [ "$VERSION" != "latest" ]; then
        abort "only one VERSION argument is accepted"
      fi
      VERSION="$1"
      ;;
  esac
  shift
done

case "$VERSION" in
  latest) ;;
  v*) ;;
  # A bare version is a common way to name a release, and the tag always carries
  # the v prefix in this repository.
  [0-9]*) VERSION="v$VERSION" ;;
  *) abort "invalid version: $VERSION (expected latest, v0.1.37, or 0.1.37)" ;;
esac
case "$VERSION" in
  */* | *' '*) abort "invalid version: $VERSION" ;;
esac

need curl
need tar
need python3

if command -v sha256sum >/dev/null 2>&1; then
  sha256_cmd=(sha256sum)
elif command -v shasum >/dev/null 2>&1; then
  # macOS ships shasum but not sha256sum; this mirrors install.sh's fallback.
  sha256_cmd=(shasum -a 256)
else
  abort "missing required command: sha256sum (or shasum)"
fi

sha256_of() {
  "${sha256_cmd[@]}" "$1" | awk '{ print $1 }' | tr 'A-Z' 'a-z'
}

license_path="$ROOT/LICENSE"
[ -f "$license_path" ] || abort "repository LICENSE not found at $license_path"

auth=()
if [ -n "${GITHUB_TOKEN:-}" ]; then
  auth=(-H "Authorization: Bearer $GITHUB_TOKEN")
fi

work="$(mktemp -d)" || abort "could not create a temporary directory"
if [ "$KEEP" -eq 1 ]; then
  echo "download dir kept: $work"
else
  # A trap is used rather than a single cleanup call so an early exit (a 404, a
  # failed check, Ctrl-C) still removes the multi-megabyte downloads.
  trap 'rm -rf "$work"' EXIT INT TERM
fi

checks_ok=0
checks_failed=0

finish() {
  echo
  echo "== summary =="
  echo "repo:     $REPO"
  echo "release:  ${tag_name:-$VERSION}"
  echo "version:  ${version:-unknown}"
  if [ "$KEEP" -eq 1 ]; then
    echo "kept:     $work"
  fi
  echo "checks:   $checks_ok ok, $checks_failed failed"
  check "checks failed" "$checks_failed" "0"
  if [ "$checks_failed" -ne 0 ]; then
    echo "RELEASE_VERIFY_FAILED"
    exit 1
  fi
  echo "RELEASE_VERIFY_OK"
  exit 0
}

# Prints the HTTP status on stdout and stores the body in $2.
api_get() {
  curl -sS -o "$2" -w '%{http_code}' \
    "${auth[@]+"${auth[@]}"}" \
    -H 'Accept: application/vnd.github+json' \
    "$1" 2>"$work/api.err" || true
}

fetch() {
  curl -fsSL --retry 3 --retry-delay 1 -o "$2" "$1"
}

if [ "$VERSION" = "latest" ]; then
  release_url="$API/releases/latest"
else
  release_url="$API/releases/tags/$VERSION"
fi

release_json="$work/release.json"
status="$(api_get "$release_url" "$release_json")"
case "$status" in
  200) ;;
  000) abort "cannot reach the GitHub API at $release_url: $(cat "$work/api.err")" ;;
  401 | 403)
    abort "GitHub API refused the request (HTTP $status): $(cat "$work/api.err"); set GITHUB_TOKEN to raise the rate limit"
    ;;
  404)
    printf 'verify_release: no release %s in %s\n' "$VERSION" "$REPO" >&2
    check "release $VERSION resolvable" "HTTP 404" "HTTP 200"
    finish
    ;;
  *) abort "unexpected HTTP $status from $release_url: $(cat "$work/api.err")" ;;
esac

if ! python3 - "$release_json" >"$work/meta.txt" 2>"$work/meta.err" <<'PY'
import json
import sys

doc = json.load(open(sys.argv[1]))
print(doc.get("id") or "")
print(doc.get("tag_name") or "")
PY
then
  abort "cannot parse the release document: $(cat "$work/meta.err")"
fi
release_id="$(sed -n '1p' "$work/meta.txt")"
tag_name="$(sed -n '2p' "$work/meta.txt")"

if [ -z "$tag_name" ]; then
  api_message="$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1])).get("message") or "")' "$release_json" 2>/dev/null || true)"
  if [ -n "$api_message" ]; then
    printf 'verify_release: GitHub API said: %s\n' "$api_message" >&2
  fi
  check "release metadata for $VERSION" "no tag_name" "tag_name"
  finish
fi

version="${tag_name#v}"
if [ "$VERSION" = "latest" ]; then
  note "release latest" "$tag_name (id $release_id)"
else
  check "release $VERSION tag" "$tag_name" "$VERSION"
fi

assets_doc="$release_json"
metadata_state="assets listed in the $tag_name document"

# The tag-addressed document can carry no browser_download_url at all while the
# assets exist, so treat that as a stale index and read releases/<id>/assets
# instead of failing a healthy release. install.sh and install.ps1 do the same.
if ! grep -q '"browser_download_url"' "$release_json"; then
  if [ -n "$release_id" ]; then
    assets_status="$(api_get "$API/releases/$release_id/assets" "$work/assets.json")"
    if [ "$assets_status" = "200" ] && [ -s "$work/assets.json" ] \
      && grep -q '"browser_download_url"' "$work/assets.json"; then
      echo "release metadata was stale for $VERSION; read assets by release id $release_id" >&2
      assets_doc="$work/assets.json"
      metadata_state="stale $tag_name document; assets read by release id $release_id"
    fi
  fi
fi

if ! python3 - "$assets_doc" "$work/assets.tsv" 2>"$work/assets.err" <<'PY'
import json
import sys

doc = json.load(open(sys.argv[1]))
assets = doc if isinstance(doc, list) else (doc.get("assets") or [])
with open(sys.argv[2], "w") as out:
    for asset in assets:
        name = asset.get("name") or ""
        url = asset.get("browser_download_url") or ""
        if name and url:
            out.write("%s\t%s\n" % (name, url))
PY
then
  abort "cannot read the asset list: $(cat "$work/assets.err")"
fi

asset_count="$(wc -l <"$work/assets.tsv" | tr -d ' ')"
if [ "$asset_count" -eq 0 ]; then
  metadata_state="no assets in the $tag_name document or in the id-addressed view"
fi
note "asset metadata" "$metadata_state"
check "assets resolvable from release metadata" "$([ "$asset_count" -gt 0 ] && echo yes || echo no)" "yes"
if [ "$asset_count" -eq 0 ]; then
  finish
fi

asset_url_for() {
  awk -F '\t' -v name="$1" '$1 == name { print $2; exit }' "$work/assets.tsv"
}

expected_assets=(
  "checksums.txt"
  "install.sh"
  "install.ps1"
  "matrix_${version}_linux_amd64.tar.gz"
  "matrix_${version}_linux_arm64.tar.gz"
  "matrix_${version}_darwin_amd64.tar.gz"
  "matrix_${version}_darwin_arm64.tar.gz"
  "matrix_${version}_windows_amd64.zip"
  "matrix_${version}_windows_arm64.zip"
)
expected_joined=" ${expected_assets[*]} "

for name in "${expected_assets[@]}"; do
  if [ -n "$(asset_url_for "$name")" ]; then
    check "asset present" "$name" "$name"
  else
    check "asset present" "missing" "$name"
  fi
done

unexpected=""
while IFS= read -r name; do
  [ -n "$name" ] || continue
  case "$expected_joined" in
    *" $name "*) ;;
    *) unexpected="${unexpected:+$unexpected, }$name" ;;
  esac
done <<<"$(cut -f1 "$work/assets.tsv")"
check "unexpected assets" "${unexpected:-none}" "none"
check "asset count" "$asset_count" "${#expected_assets[@]}"

duplicate_names="$(cut -f1 "$work/assets.tsv" | sort | uniq -d | tr '\n' ',')"
duplicate_names="${duplicate_names%,}"
check "duplicate asset names" "${duplicate_names:-none}" "none"

mkdir -p "$work/dl"
checksums_file="$work/dl/checksums.txt"
checksums_url="$(asset_url_for "checksums.txt")"
if [ -z "$checksums_url" ]; then
  # Already reported as a missing asset; no archive can be verified without it.
  check "download checksums.txt" "asset missing" "checksums.txt bytes"
  finish
fi
if fetch "$checksums_url" "$checksums_file"; then
  note "download checksums.txt" "$(wc -c <"$checksums_file" | tr -d ' ') bytes"
else
  check "download checksums.txt" "curl failed" "checksums.txt bytes"
  finish
fi

archive_names=()
for name in "${expected_assets[@]}"; do
  case "$name" in
    *.tar.gz | *.zip) archive_names+=("$name") ;;
  esac
done

# Prints "<entry path or status><TAB><sha256 or detail>" for the LICENSE member.
license_probe() {
  case "$1" in
    *.zip)
      # zipfile is used because unzip is not guaranteed to be installed.
      python3 - "$1" <<'PY'
import hashlib
import sys
import zipfile

with zipfile.ZipFile(sys.argv[1]) as archive:
    entries = [n for n in archive.namelist() if n == "LICENSE" or n.endswith("/LICENSE")]
    if not entries:
        print("missing\tno LICENSE entry")
    elif len(entries) > 1:
        print("multiple\t%s" % ",".join(entries))
    else:
        print("%s\t%s" % (entries[0], hashlib.sha256(archive.read(entries[0])).hexdigest()))
PY
      ;;
    *)
      entry="$(tar -tzf "$1" 2>/dev/null | grep -E '(^|/)LICENSE$' | head -n 1 || true)"
      if [ -z "$entry" ]; then
        printf 'missing\tno LICENSE entry\n'
      else
        printf '%s\t%s\n' "$entry" "$(tar -xzOf "$1" "$entry" 2>/dev/null | "${sha256_cmd[@]}" | awk '{ print $1 }')"
      fi
      ;;
  esac
}

repo_license_sha="$(sha256_of "$license_path")"

for name in "${archive_names[@]}"; do
  dest="$work/dl/$name"
  url="$(asset_url_for "$name")"
  if [ -z "$url" ]; then
    # Reported once by the asset-present check; there is nothing to download.
    continue
  fi
  if ! fetch "$url" "$dest"; then
    check "download $name" "curl failed" "archive bytes"
    continue
  fi
  note "download $name" "$(wc -c <"$dest" | tr -d ' ') bytes"

  # GoReleaser writes "<sha256>  <name>"; an archive with no entry, or with the
  # same name twice, is a broken checksum document even when the bytes are fine.
  entries="$(awk -v name="$name" 'NF >= 2 { file = $2; sub(/^\*/, "", file); if (file == name) print $1 }' "$checksums_file")"
  entry_count="$(printf '%s\n' "$entries" | sed '/^$/d' | wc -l | tr -d ' ')"
  check "checksums.txt entries for $name" "$entry_count" "1"
  if [ "$entry_count" -ne 1 ]; then
    continue
  fi
  check "sha256 $name" "$(sha256_of "$dest")" "$(printf '%s' "$entries" | tr 'A-Z' 'a-z')"

  # Apache-2.0 requires the licence text to travel with a redistributed binary.
  # GoReleaser's archives.files replaces its default file list, so a release can
  # ship archives without LICENSE (v0.1.34 did) unless something checks it.
  probe="$(license_probe "$dest")"
  present="yes"
  inner_sha=""
  case "$probe" in
    missing*) present="no (no LICENSE entry)" ;;
    multiple*) present="no (multiple LICENSE entries: $(printf '%s' "$probe" | cut -f2))" ;;
    "") present="no (archive unreadable)" ;;
    *) inner_sha="$(printf '%s' "$probe" | cut -f2)" ;;
  esac
  check "LICENSE present in $name" "$present" "yes"
  if [ "$present" = "yes" ]; then
    check "LICENSE matches repository in $name" "$inner_sha" "$repo_license_sha"
  fi
done

finish
