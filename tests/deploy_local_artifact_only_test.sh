#!/usr/bin/env bash
set -euo pipefail
repo="$(cd "$(dirname "$0")/.." && pwd)"
scratch="$(mktemp -d)"
trap 'rm -rf "$scratch"' EXIT
mkdir -p "$scratch/archive/configs" "$scratch/dist" "$scratch/user" "$scratch/pal/data" "$scratch/pal/configs"
cat > "$scratch/archive/matrix" <<'BIN'
#!/usr/bin/env sh
case "$*" in
  version) echo 'matrix fixture' ;;
  home) echo "$MATRIX_HOME" ;;
  'run submit --help') echo 'fixture submission help' ;;
  *) echo "Installer invoked a storage/runtime command: $*" >&2; exit 77 ;;
esac
BIN
chmod +x "$scratch/archive/matrix"
printf '%s\n' 'Apache-2.0 fixture' > "$scratch/archive/LICENSE"
printf '%s\n' 'release seed' > "$scratch/archive/configs/system.json"
printf '%s\n' 'existing user configuration' > "$scratch/pal/configs/system.json"
printf '%s\n' 'existing user Vault sentinel' > "$scratch/pal/data/matrix-vault.db"
cp "$scratch/pal/data/matrix-vault.db" "$scratch/original-vault"
cp "$scratch/pal/configs/system.json" "$scratch/original-config"
os="$(uname -s | tr '[:upper:]' '[:lower:]')"
case "$(uname -m)" in x86_64|amd64) arch=amd64 ;; arm64|aarch64) arch=arm64 ;; *) exit 2 ;; esac
archive="matrix_fixture_${os}_${arch}.tar.gz"
tar -czf "$scratch/dist/$archive" -C "$scratch/archive" .
(cd "$scratch/dist" && sha256sum "$archive" > checksums.txt)
for iteration in 1 2; do
  HOME="$scratch/user" MATRIX_HOME="$scratch/pal" MATRIX_DIST_DIR="$scratch/dist" MATRIX_VERSION=fixture \
    bash "$repo/scripts/deploy_local_install.sh" > "$scratch/install-$iteration.log" 2>&1
  cmp "$scratch/original-vault" "$scratch/pal/data/matrix-vault.db"
  cmp "$scratch/original-config" "$scratch/pal/configs/system.json"
done
echo 'LOCAL_DEPLOY_ARTIFACT_ONLY_OK: install and rerun preserved Vault/configuration; no storage command invoked'
