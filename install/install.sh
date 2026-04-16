#!/usr/bin/env sh
set -eu

REPO="${MATRIX_REPO:-Josepavese/matrix}"
VERSION="${MATRIX_VERSION:-latest}"

usage() {
  echo "Usage: MATRIX_VERSION=latest MATRIX_HOME=/custom/path sh install.sh"
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

need curl
need tar

os="$(uname -s | tr '[:upper:]' '[:lower:]')"
case "$os" in
  linux) goos="linux" ;;
  darwin) goos="darwin" ;;
  *)
    echo "unsupported OS for install.sh: $os" >&2
    exit 1
    ;;
esac

machine="$(uname -m)"
case "$machine" in
  x86_64|amd64) goarch="amd64" ;;
  arm64|aarch64) goarch="arm64" ;;
  *)
    echo "unsupported architecture: $machine" >&2
    exit 1
    ;;
esac

if [ -n "${MATRIX_HOME:-}" ]; then
  matrix_home="$MATRIX_HOME"
elif [ "$goos" = "darwin" ]; then
  matrix_home="$HOME/Library/Application Support/Matrix"
else
  matrix_home="${XDG_DATA_HOME:-$HOME/.local/share}/matrix"
fi

api="https://api.github.com/repos/$REPO/releases/latest"
if [ "$VERSION" != "latest" ]; then
  api="https://api.github.com/repos/$REPO/releases/tags/$VERSION"
fi

json="$(curl -fsSL "$api")"
asset_url="$(printf '%s\n' "$json" \
  | grep -Eo '"browser_download_url":[[:space:]]*"[^"]+"' \
  | cut -d '"' -f 4 \
  | grep "_${goos}_${goarch}\\.tar\\.gz$" \
  | head -n 1 || true)"

if [ -z "$asset_url" ]; then
  echo "no Matrix release asset found for ${goos}_${goarch} in $REPO $VERSION" >&2
  exit 1
fi

tmp_dir="$(mktemp -d)"
cleanup() {
  rm -rf "$tmp_dir"
}
trap cleanup EXIT INT TERM

archive="$tmp_dir/matrix.tar.gz"
curl -fL "$asset_url" -o "$archive"
tar -xzf "$archive" -C "$tmp_dir"

mkdir -p "$matrix_home/bin" "$matrix_home/configs" "$matrix_home/data" "$matrix_home/logs" "$matrix_home/artifacts" "$matrix_home/backups" "$matrix_home/tmp"
install -m 0755 "$tmp_dir/matrix" "$matrix_home/bin/matrix"

if [ -d "$tmp_dir/configs" ]; then
  find "$tmp_dir/configs" -type f | while IFS= read -r src; do
    rel="${src#$tmp_dir/configs/}"
    dest="$matrix_home/configs/$rel"
    mkdir -p "$(dirname "$dest")"
    if [ ! -e "$dest" ]; then
      cp "$src" "$dest"
    fi
  done
fi

echo "Matrix installed."
echo "PAL home: $matrix_home"
echo "Binary:   $matrix_home/bin/matrix"
echo
echo "Run:"
echo "  MATRIX_HOME=\"$matrix_home\" \"$matrix_home/bin/matrix\" home"
echo "  MATRIX_HOME=\"$matrix_home\" \"$matrix_home/bin/matrix\" bootstrap doctor"
echo
echo "Optional PATH:"
echo "  export MATRIX_HOME=\"$matrix_home\""
echo "  export PATH=\"\$MATRIX_HOME/bin:\$PATH\""
