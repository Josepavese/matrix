# Getting Started

Install Matrix and run your first agent conversation in under 5 minutes.

## Prerequisites

Matrix talks to real coding agents. Before you start, make sure you have at least one of these installed and working on your machine:

- [OpenCode](https://github.com/opencode-ai/opencode) with ACP support (`opencode acp`)
- [Claude Code](https://docs.anthropic.com/en/docs/claude-code) (`claude acp`)
- [Gemini CLI](https://github.com/google-gemini/gemini-cli) (`gemini --experimental-acp`)

Matrix does not replace your agents. It routes communication to them. You need the agent binaries already installed.

## Installation

### Linux and macOS

```bash
curl -fsSL https://raw.githubusercontent.com/Josepavese/matrix/main/install/install.sh | sh
```

### Windows PowerShell

```powershell
irm https://raw.githubusercontent.com/Josepavese/matrix/main/install/install.ps1 | iex
```

### Install a specific version

```bash
curl -fsSL https://raw.githubusercontent.com/Josepavese/matrix/main/install/install.sh | MATRIX_VERSION=v0.1.2 sh
```

The installer downloads the right binary for your platform and sets up the `matrix` command. If your shell cannot find `matrix` after install, open a new terminal.

## First Run

### 1. Check your setup

```bash
matrix bootstrap doctor
```

This prints first-run setup guidance. It checks your environment, detects installed agents, and tells you what still needs to be configured.

### 2. Start the daemon

```bash
matrix run
```

Matrix starts a background daemon that exposes an HTTP API (default: `127.0.0.1:9091`) and optionally a Telegram bot. You will see the listening address in the output.

### 3. Verify it is running

```bash
curl http://127.0.0.1:9091/_matrix/runtime
```

You should get a JSON health report.

## Your First Prompt

Send a prompt to your default agent:

```bash
curl -X POST http://127.0.0.1:9091/v1/runs \
  -H "Content-Type: application/json" \
  -d '{
    "channel_id": "docs.http",
    "input": "What files are in the current directory?"
  }'
```

Matrix routes the prompt to your default agent (configured during bootstrap, typically OpenCode), runs it, and returns the result.

### With an API key

If you set an API key during bootstrap:

```bash
curl -X POST http://127.0.0.1:9091/v1/runs \
  -H "Content-Type: application/json" \
  -H "X-Matrix-Key: your-api-key" \
  -d '{
    "channel_id": "docs.http",
    "input": "List the top-level files in this project"
  }'
```

### Streaming response

For long-running prompts, use stream mode to get results as they arrive:

```bash
curl -X POST http://127.0.0.1:9091/v1/runs \
  -H "Content-Type: application/json" \
  -d '{
    "channel_id": "docs.http",
    "input": "Explain the architecture of this codebase",
    "execution_mode": "stream"
  }'
```

## Next Steps

- [Core Concepts](Core-Concepts.md) -- understand workspaces, sessions, and agents
- [Using Agents](Using-Agents.md) -- search for and install new agents
- [Channels](Channels.md) -- connect Telegram for chat-based access
- [Examples](Examples.md) -- walk through real workflows

## Where Matrix Lives

Matrix stores everything in one place:

| OS | Location |
|----|----------|
| Linux | `~/.local/share/matrix/` |
| macOS | `~/Library/Application Support/Matrix/` |
| Windows | `%LOCALAPPDATA%\Matrix\` |

The layout:

```
matrix/
  bin/          the matrix binary
  configs/      seed configuration files
  data/         vault database (matrix-vault.db)
  logs/         runtime logs
  artifacts/    generated artifacts
  backups/      vault backups
  tmp/          temporary workspace
```

You normally never need to touch these files directly. Use the CLI or API instead.

## Uninstall

Remove the Matrix home directory:

```bash
# Linux
rm -rf "${XDG_DATA_HOME:-$HOME/.local/share}/matrix"

# macOS
rm -rf "$HOME/Library/Application Support/Matrix"
```

```powershell
# Windows PowerShell
Remove-Item -Recurse -Force "$env:LOCALAPPDATA\Matrix"
```
