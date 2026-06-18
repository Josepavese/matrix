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
checksum_url="$(printf '%s\n' "$json" \
  | grep -Eo '"browser_download_url":[[:space:]]*"[^"]+"' \
  | cut -d '"' -f 4 \
  | grep '/checksums\.txt$' \
  | head -n 1 || true)"

if [ -z "$asset_url" ]; then
  echo "no Matrix release asset found for ${goos}_${goarch} in $REPO $VERSION" >&2
  exit 1
fi

verify_checksum() {
  checksum_file="$1"
  archive_path="$2"
  asset_name="$3"

  expected="$(awk -v name="$asset_name" 'NF >= 2 { file=$2; sub(/^\*/, "", file); if (file == name) { print $1; exit } }' "$checksum_file")"
  if [ -z "$expected" ]; then
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
  if [ "$actual" != "$expected" ]; then
    echo "checksum verification failed for $asset_name" >&2
    exit 1
  fi

  echo "Verified checksum for $asset_name"
}

tmp_dir="$(mktemp -d)"
cleanup() {
  rm -rf "$tmp_dir"
}
trap cleanup EXIT INT TERM

archive="$tmp_dir/matrix.tar.gz"
asset_name="$(basename "$asset_url")"
curl -fL "$asset_url" -o "$archive"
if [ -n "$checksum_url" ]; then
  checksum_file="$tmp_dir/checksums.txt"
  curl -fL "$checksum_url" -o "$checksum_file"
  verify_checksum "$checksum_file" "$archive" "$asset_name"
else
  echo "No checksums.txt release asset found; skipping checksum verification."
fi
tar -xzf "$archive" -C "$tmp_dir"
if [ ! -f "$tmp_dir/matrix" ]; then
  echo "release asset $asset_name does not contain the matrix binary at archive root" >&2
  exit 1
fi

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
  if [ -f "$profile" ] && grep -F "$marker" "$profile" >/dev/null 2>&1; then
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
    if [ "$(basename "${SHELL:-}")" = "zsh" ]; then
      ensure_path_file "$HOME/.zshrc"
    fi
    ;;
esac

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
echo "Launcher: $launcher"
echo
echo "Run:"
echo "  matrix home"
echo "  matrix bootstrap doctor"
echo
if [ "$path_ready" -eq 0 ]; then
  echo "PATH was updated in your shell profile. Open a new shell if 'matrix' is not found in this one."
fi
