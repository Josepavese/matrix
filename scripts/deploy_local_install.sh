#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
dist_dir="${MATRIX_DIST_DIR:-$repo_root/dist}"
version="${MATRIX_VERSION:-}"

usage() {
  cat <<'USAGE'
Usage: scripts/deploy_local_install.sh

Installs the host-matching Matrix artifact from dist/ into the local PAL home.

Environment:
  MATRIX_HOME      Override target PAL home.
  MATRIX_DIST_DIR  Override artifact directory. Default: ./dist
  MATRIX_VERSION   Optional artifact version prefix. Auto-detected when unset.
  MATRIX_ALLOW_MISSING_CHECKSUM
                  Allow a local smoke install without dist/checksums.txt.
  HOME             Override user home for isolated launcher/profile smoke tests.
USAGE
}

case "${1:-}" in
  -h|--help)
    usage
    exit 0
    ;;
esac

need() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "missing required command: $1" >&2
    exit 1
  }
}

need uname
need find
need tar

home_dir="${HOME:-}"
if [[ -z "$home_dir" ]]; then
  echo "HOME must be set; use an isolated HOME for smoke installs that should not touch your real profile." >&2
  exit 1
fi

os="$(uname -s | tr '[:upper:]' '[:lower:]')"
case "$os" in
  linux) goos="linux" ;;
  darwin) goos="darwin" ;;
  *)
    echo "unsupported OS for local deploy install: $os" >&2
    exit 1
    ;;
esac

machine="$(uname -m)"
case "$machine" in
  x86_64|amd64) goarch="amd64" ;;
  arm64|aarch64) goarch="arm64" ;;
  *)
    echo "unsupported architecture for local deploy install: $machine" >&2
    exit 1
    ;;
esac

if [[ -n "${MATRIX_HOME:-}" ]]; then
  matrix_home="$MATRIX_HOME"
elif [[ "$goos" == "darwin" ]]; then
  matrix_home="$home_dir/Library/Application Support/Matrix"
else
  matrix_home="${XDG_DATA_HOME:-$home_dir/.local/share}/matrix"
fi

if [[ -n "$version" ]]; then
  archive="$dist_dir/matrix_${version}_${goos}_${goarch}.tar.gz"
else
  shopt -s nullglob
  matches=("$dist_dir"/matrix_*_"${goos}_${goarch}".tar.gz)
  shopt -u nullglob
  if [[ "${#matches[@]}" -eq 0 ]]; then
    archive=""
  else
    IFS=$'\n' read -r -d '' -a sorted_matches < <(printf '%s\n' "${matches[@]}" | sort -V && printf '\0')
    archive="${sorted_matches[-1]}"
  fi
fi
if [[ ! -f "$archive" ]]; then
  if [[ -n "$version" ]]; then
    echo "missing local deploy artifact: $archive" >&2
  else
    echo "missing local deploy artifact for ${goos}_${goarch} in $dist_dir" >&2
  fi
  echo "run: goreleaser release --snapshot --clean" >&2
  exit 1
fi

verify_checksum() {
  local checksum_file="$1"
  local archive_path="$2"
  local asset_name="$3"
  local expected actual

  expected="$(awk -v name="$asset_name" 'NF >= 2 { file=$2; sub(/^\*/, "", file); if (file == name) { print $1; exit } }' "$checksum_file")"
  if [[ -z "$expected" ]]; then
    echo "checksums.txt is available but has no entry for $asset_name" >&2
    exit 1
  fi

  if command -v sha256sum >/dev/null 2>&1; then
    actual="$(sha256sum "$archive_path" | awk '{ print $1 }')"
  elif command -v shasum >/dev/null 2>&1; then
    actual="$(shasum -a 256 "$archive_path" | awk '{ print $1 }')"
  else
    echo "checksums.txt is available but neither sha256sum nor shasum is installed" >&2
    exit 1
  fi

  actual="$(printf '%s' "$actual" | tr '[:upper:]' '[:lower:]')"
  expected="$(printf '%s' "$expected" | tr '[:upper:]' '[:lower:]')"
  if [[ "$actual" != "$expected" ]]; then
    echo "checksum verification failed for $asset_name" >&2
    exit 1
  fi

  echo "Verified checksum for $asset_name"
}

validate_tar_archive() {
  local archive_path="$1"
  tar -tzf "$archive_path" | while IFS= read -r entry; do
    case "$entry" in
      ""|/*|../*|*/../*|..|*/..)
        echo "unsafe archive entry: $entry" >&2
        exit 1
      ;;
    esac
  done
  tar -tzvf "$archive_path" | while IFS= read -r entry; do
    local type
    type="$(printf '%s' "$entry" | cut -c1)"
    case "$type" in
      -|d) ;;
      *)
        echo "unsafe archive entry type: $entry" >&2
        exit 1
        ;;
    esac
  done
}

validate_extracted_tree() {
  local root="$1"
  local unsafe_link unsafe_hardlink
  unsafe_link="$(find "$root" -type l -print -quit)"
  if [[ -n "$unsafe_link" ]]; then
    echo "unsafe extracted symlink: $unsafe_link" >&2
    exit 1
  fi
  unsafe_hardlink="$(find "$root" -type f -links +1 -print -quit)"
  if [[ -n "$unsafe_hardlink" ]]; then
    echo "unsafe extracted hardlink: $unsafe_hardlink" >&2
    exit 1
  fi
}

prepare_pal_home() {
  mkdir -p "$matrix_home"
  chmod 0700 "$matrix_home"
  for dir in bin configs data logs artifacts backups tmp; do
    mkdir -p "$matrix_home/$dir"
    chmod 0700 "$matrix_home/$dir"
  done
}

tmp_dir="$(mktemp -d)"
cleanup() {
  rm -rf "$tmp_dir"
}
trap cleanup EXIT INT TERM

checksum_file="$dist_dir/checksums.txt"
if [[ -f "$checksum_file" ]]; then
  verify_checksum "$checksum_file" "$archive" "$(basename "$archive")"
else
  if [[ "${MATRIX_ALLOW_MISSING_CHECKSUM:-}" == "1" || "${MATRIX_ALLOW_MISSING_CHECKSUM:-}" == "true" ]]; then
    echo "No checksums.txt found in $dist_dir; continuing because MATRIX_ALLOW_MISSING_CHECKSUM=$MATRIX_ALLOW_MISSING_CHECKSUM."
  else
    echo "missing checksums.txt in $dist_dir" >&2
    exit 1
  fi
fi
validate_tar_archive "$archive"
tar -xzf "$archive" -C "$tmp_dir"
validate_extracted_tree "$tmp_dir"
if [[ ! -f "$tmp_dir/matrix" || -L "$tmp_dir/matrix" ]]; then
  echo "local deploy artifact $(basename "$archive") does not contain the matrix binary at archive root" >&2
  exit 1
fi

prepare_pal_home
install -m 0755 "$tmp_dir/matrix" "$matrix_home/bin/matrix"

user_bin="$home_dir/.local/bin"
launcher="$user_bin/matrix"
mkdir -p "$user_bin"
if ln -sfn "$matrix_home/bin/matrix" "$launcher" 2>/dev/null; then
  :
else
  cp "$matrix_home/bin/matrix" "$launcher"
  chmod 0755 "$launcher"
fi

ensure_path_file() {
  profile="$1"
  marker="# Matrix PAL PATH"
  if [[ -f "$profile" ]] && grep -F "$marker" "$profile" >/dev/null 2>&1; then
    return
  fi
  {
    echo
    echo "$marker"
    echo "case \":\$PATH:\" in"
    echo "  *\":$user_bin:\"*) ;;"
    echo "  *) export PATH=\"$user_bin:\$PATH\" ;;"
    echo "esac"
  } >> "$profile"
}

case ":$PATH:" in
  *":$user_bin:"*) path_ready=1 ;;
  *)
    path_ready=0
    ensure_path_file "$HOME/.profile"
    if [[ "$(basename "${SHELL:-}")" == "zsh" ]]; then
      ensure_path_file "$HOME/.zshrc"
    fi
    ;;
esac

if [[ -d "$tmp_dir/configs" ]]; then
  while IFS= read -r src; do
    rel="${src#$tmp_dir/configs/}"
    dest="$matrix_home/configs/$rel"
    mkdir -p "$(dirname "$dest")"
    chmod 0700 "$(dirname "$dest")"
    if [[ ! -e "$dest" ]]; then
      cp "$src" "$dest"
      chmod 0600 "$dest"
    fi
  done < <(find "$tmp_dir/configs" -type f)
  find "$matrix_home/configs" -type d -exec chmod 0700 {} +
fi

echo "Matrix local deploy install complete."
echo "PAL home: $matrix_home"
echo "Binary:   $matrix_home/bin/matrix"
echo "Launcher: $launcher"

smoke_index=0
run_smoke() {
  smoke_index=$((smoke_index + 1))
  smoke_log="$tmp_dir/smoke-$smoke_index.log"
  echo "Smoke: matrix $*"
  if ! MATRIX_HOME="$matrix_home" "$matrix_home/bin/matrix" "$@" >"$smoke_log" 2>&1; then
    echo "smoke failed: matrix $*" >&2
    cat "$smoke_log" >&2
    exit 1
  fi
}

echo
echo "== Post-install smoke =="
run_smoke home
run_smoke vault migrate
run_smoke bootstrap doctor
run_smoke doctor
run_smoke readiness
echo "Post-install smoke OK: home, vault migrate, bootstrap doctor, doctor, readiness"

if [[ "$path_ready" -eq 0 ]]; then
  echo "PATH was updated in your shell profile. Open a new shell if 'matrix' is not found in this one."
fi
