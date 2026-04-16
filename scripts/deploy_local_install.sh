#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
dist_dir="${MATRIX_DIST_DIR:-$repo_root/dist}"
version="${MATRIX_VERSION:-0.0.0-snapshot}"

usage() {
  cat <<'USAGE'
Usage: scripts/deploy_local_install.sh

Installs the host-matching Matrix artifact from dist/ into the local PAL home.

Environment:
  MATRIX_HOME      Override target PAL home.
  MATRIX_DIST_DIR  Override artifact directory. Default: ./dist
  MATRIX_VERSION   Artifact version prefix. Default: 0.0.0-snapshot
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
need tar

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
  matrix_home="$HOME/Library/Application Support/Matrix"
else
  matrix_home="${XDG_DATA_HOME:-$HOME/.local/share}/matrix"
fi

archive="$dist_dir/matrix_${version}_${goos}_${goarch}.tar.gz"
if [[ ! -f "$archive" ]]; then
  echo "missing local deploy artifact: $archive" >&2
  echo "run: goreleaser release --snapshot --clean" >&2
  exit 1
fi

tmp_dir="$(mktemp -d)"
cleanup() {
  rm -rf "$tmp_dir"
}
trap cleanup EXIT INT TERM

tar -xzf "$archive" -C "$tmp_dir"

mkdir -p "$matrix_home/bin" "$matrix_home/configs" "$matrix_home/data" "$matrix_home/logs" "$matrix_home/artifacts" "$matrix_home/backups" "$matrix_home/tmp"
install -m 0755 "$tmp_dir/matrix" "$matrix_home/bin/matrix"

user_bin="$HOME/.local/bin"
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
    if [[ ! -e "$dest" ]]; then
      cp "$src" "$dest"
    fi
  done < <(find "$tmp_dir/configs" -type f)
fi

echo "Matrix local deploy install complete."
echo "PAL home: $matrix_home"
echo "Binary:   $matrix_home/bin/matrix"
echo "Launcher: $launcher"
MATRIX_HOME="$matrix_home" "$matrix_home/bin/matrix" home
if [[ "$path_ready" -eq 0 ]]; then
  echo "PATH was updated in your shell profile. Open a new shell if 'matrix' is not found in this one."
fi
