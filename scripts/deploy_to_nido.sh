#!/bin/bash
set -e

VM_NAME="matrix-e2e-validator"
DEPLOY_DIR="\$HOME/matrix-test-bot"

echo "Building matrix binary for Linux..."
# Ensure Linux build for Ubuntu VM
cd matrix-v2
GOOS=linux GOARCH=amd64 go build -o matrix ./cmd/matrix/*.go
echo "Building mock agent as opencode for Linux..."
GOOS=linux GOARCH=amd64 go build -o opencode ./cmd/mock-agent/*.go
cd ..

echo "Preparing deployment directory in $VM_NAME..."
nido ssh "$VM_NAME" "mkdir -p $DEPLOY_DIR/configs/locales"

echo "Deploying binary and mock agent..."
cat matrix-v2/matrix | nido ssh "$VM_NAME" "cat > $DEPLOY_DIR/matrix && chmod +x $DEPLOY_DIR/matrix"
cat matrix-v2/opencode | nido ssh "$VM_NAME" "cat > $DEPLOY_DIR/opencode && chmod +x $DEPLOY_DIR/opencode"

echo "Deploying configs (including telegram test bot)..."
cat matrix-v2/configs/agents.json | nido ssh "$VM_NAME" "cat > $DEPLOY_DIR/configs/agents.json"
cat matrix-v2/configs/telegram_test.json | nido ssh "$VM_NAME" "cat > $DEPLOY_DIR/configs/telegram.json"
cat matrix-v2/configs/locales/en.json | nido ssh "$VM_NAME" "cat > $DEPLOY_DIR/configs/locales/en.json"
cat matrix-v2/configs/locales/it.json | nido ssh "$VM_NAME" "cat > $DEPLOY_DIR/configs/locales/it.json"
echo "======================================"
echo "Deployment successful to $VM_NAME!"
echo "To start the Matrix test bot, run:"
echo "nido ssh $VM_NAME \"export NVM_DIR=\\\$HOME/.nvm && . \\\$NVM_DIR/nvm.sh && cd $DEPLOY_DIR && export PATH=\\\$NVM_DIR/versions/node/v20.20.0/bin:\\\$PATH:$DEPLOY_DIR && nohup ./matrix run > /tmp/matrix.log 2>&1 &\""
echo "======================================"
