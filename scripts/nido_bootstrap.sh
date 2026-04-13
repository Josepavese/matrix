#!/bin/bash
# nido_bootstrap.sh - Idempotent environment setup for Matrix E2E testing
set -e

VM_NAME="matrix-e2e-validator"
TEMPLATE_NAME="matrix-validator-base"
IMAGE_TAG="ubuntu:22.04"

# 1. Verifica se la VM esiste già
VM_EXISTS=$(nido ls --json | jq -r '.data.vms[]? | select(.name == "'$VM_NAME'") | .name')

if [ "$VM_EXISTS" == "$VM_NAME" ]; then
    echo "✅ VM '$VM_NAME' già presente nel nido."
    STATE=$(nido ls --json | jq -r '.data.vms[]? | select(.name == "'$VM_NAME'") | .state')
    if [ "$STATE" == "stopped" ]; then
        echo "⚡ Risveglio della VM..."
        nido start "$VM_NAME"
    fi
    exit 0
fi

# 2. Se la VM non esiste, prova a crearla dal template
TEMPLATE_EXISTS=$(nido template list --json | jq -r '.data.templates[]? | select(.name == "'$TEMPLATE_NAME'") | .name')

if [ "$TEMPLATE_EXISTS" == "$TEMPLATE_NAME" ]; then
    echo "🐣 Creazione VM dal template '$TEMPLATE_NAME'..."
    # nido spawn <name> --template <template>
    nido spawn "$VM_NAME" --template "$TEMPLATE_NAME" || nido spawn "$VM_NAME" --image "$TEMPLATE_NAME"
    nido start "$VM_NAME"
    exit 0
fi

# 3. Se neanche il template esiste, creazione da zero
echo "🌱 Nessun template trovato. Inizio creazione da zero ($IMAGE_TAG)..."
nido spawn "$VM_NAME" --image "$IMAGE_TAG"
nido start "$VM_NAME"

echo "🛠️ Installazione dipendenze nella VM (APM Dependencies)..."
# Attendiamo che SSH sia pronto
sleep 10
nido ssh "$VM_NAME" "sudo apt-get update && sudo apt-get install -y python3 python3-pip git curl"

echo "📦 Installazione NVM e Node.js (v22)..."
# Install NVM
nido ssh "$VM_NAME" "curl -o- https://raw.githubusercontent.com/nvm-sh/nvm/v0.39.7/install.sh | bash"

# Use nvm to install node (version from SSOT)
NODE_VER=$(jq -r '.node_version' matrix-v2/configs/node_env.json)
nido ssh "$VM_NAME" "export NVM_DIR=\"\$HOME/.nvm\" && [ -s \"\$NVM_DIR/nvm.sh\" ] && \\. \"\$NVM_DIR/nvm.sh\" && nvm install $NODE_VER && nvm use $NODE_VER && nvm alias default $NODE_VER"

echo "🤖 Installazione Agenti Globali..."
nido ssh "$VM_NAME" "export NVM_DIR=\"\$HOME/.nvm\" && [ -s \"\$NVM_DIR/nvm.sh\" ] && \\. \"\$NVM_DIR/nvm.sh\" && npm install -g @openai/codex"

echo "❄️ Creazione template per i prossimi test..."
nido stop "$VM_NAME"
# Assumo la sintassi 'nido template create <vm> <template>' basata su pattern nido
nido template create "$VM_NAME" "$TEMPLATE_NAME" || nido template archive "$VM_NAME" "$TEMPLATE_NAME" || echo "⚠️ Errore creazione template, procedo comunque."

echo "🚀 Riavvio VM pronta per i test."
nido start "$VM_NAME"
