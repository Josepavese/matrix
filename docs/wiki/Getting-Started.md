# Getting Started

Install Matrix, select a working agent and send a task to an explicit project.

## Prerequisites

- Linux, macOS or Windows on a supported release architecture (amd64/arm64).
- An installed and authenticated ACP agent/adapter, or a configured remote A2A agent.
- An existing project directory accessible to that agent.

Matrix can discover/install registry agents, but provider authentication and
model access need their own setup. An enabled seed definition does not mean its
binary or account is ready. See [Using Agents](Using-Agents.md).

## Installation

### Linux and macOS

Resolve one release, download its installer, inspect it, then run it:

```bash
MATRIX_VERSION="$(curl -fsSL https://api.github.com/repos/Josepavese/matrix/releases/latest | sed -n 's/.*"tag_name": "\(v[^"]*\)".*/\1/p' | head -n 1)"
curl -fsSLO "https://github.com/Josepavese/matrix/releases/download/${MATRIX_VERSION}/install.sh"
less install.sh
env MATRIX_VERSION="$MATRIX_VERSION" sh install.sh
```

### Windows PowerShell

```powershell
$release = Invoke-RestMethod https://api.github.com/repos/Josepavese/matrix/releases/latest
$env:MATRIX_VERSION = $release.tag_name
Invoke-WebRequest "https://github.com/Josepavese/matrix/releases/download/$env:MATRIX_VERSION/install.ps1" -OutFile install.ps1
notepad install.ps1
.\install.ps1
```

### Install a specific version

Set `MATRIX_VERSION` (or `$env:MATRIX_VERSION`) to the release tag `vX.Y.Z`
before downloading its installer. The scripts verify the release archive and
install into the canonical PAL home, without a repository clone. Open a new
terminal if `matrix` is not on the current shell's PATH.

## First run

### 1. Check your setup

```bash
matrix home
matrix bootstrap doctor
matrix agent list
matrix agent doctor opencode
```

Bootstrap doctor reports readiness and setup guidance. It does not install
all agents or authenticate accounts. Here `opencode` is an example: substitute
a configured agent that passes its prerequisites and protocol check.

### 2. Register a project

```bash
matrix workspace add my-project --path /absolute/path/to/project --default-agent opencode
```

On Windows, use the real path, for example `--path "C:\Work\my-project"`.
Matrix resolves its runtime home on startup; do not assume the shell's current
directory becomes the task workspace.

### 3. Start the daemon

```bash
matrix run
```

This runs in the foreground. Keep that terminal open and use another for
requests. If a service already runs Matrix, use it instead of starting a second
listener. Default HTTP address: `127.0.0.1:9091`.

### 4. Verify authenticated access

On first daemon startup Matrix creates separate HTTP and JSON-RPC keys when
absent. In the second terminal, Bash:

```bash
MATRIX_API_KEY="$(matrix config get matrix_api_key)"
curl --fail-with-body -sS -H "X-Matrix-Key: $MATRIX_API_KEY" \
  http://127.0.0.1:9091/_matrix/runtime
```

Keep the key private. HTTP authentication is required by default. Provider
readiness failures in this report need inspection; an active HTTP server alone
does not prove an agent can execute a task.

## Your first prompt

### Bash

```bash
curl --fail-with-body -sS http://127.0.0.1:9091/v1/runs \
  -H "X-Matrix-Key: $MATRIX_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "channel_id": "docs.http",
    "agent_id": "opencode",
    "workspace_id": "my-project",
    "execution_mode": "sync",
    "input": "Explain this project. Read files as needed; do not modify them."
  }'
```

### PowerShell

```powershell
$matrixKey = matrix config get matrix_api_key
$matrixHeaders = @{ "X-Matrix-Key" = $matrixKey }
$matrixBody = @{
  channel_id = "docs.http"
  agent_id = "opencode"
  workspace_id = "my-project"
  execution_mode = "sync"
  input = "Explain this project. Read files as needed; do not modify them."
} | ConvertTo-Json
Invoke-RestMethod -Method Post -Uri http://127.0.0.1:9091/v1/runs -Headers $matrixHeaders -ContentType application/json -Body $matrixBody
```

Inspect the response status and retain `run_id`. The task's instruction to avoid
edits is not an enforced permission policy. For enforced restrictions, configure
[provider permissions or OS isolation](PAL-Execution-and-Observability.md#sandbox).

### Streaming response

Change `execution_mode` to `stream` for streamed events, or to `async` to return
an accepted run immediately. Async callers must inspect the eventual outcome;
see [Delegation and Notifications](Delegation-and-Notifications.md).

## Where Matrix lives

`MATRIX_HOME` overrides the OS default; `matrix home` prints the resolved path.

| OS | Default |
|---|---|
| Linux | `$XDG_DATA_HOME/matrix` or `~/.local/share/matrix` |
| macOS | `~/Library/Application Support/Matrix` |
| Windows | `%LOCALAPPDATA%\Matrix` |

`bin/` contains the binary, `configs/` seed files, `data/` the Vault and runtime
state, `logs/` local logs, `artifacts/` generated artifacts, `backups/` backups,
and `tmp/` runtime scratch. Configuration and state are Vault-backed; editing a
seed JSON file is not the normal way to update an existing installation.

Encryption requires a configured master key. Protect that key as well as the
Vault; keep project/provider backups separately. See [FAQ](FAQ.md#is-my-data-encrypted).

## Uninstall

Stop the foreground daemon or its service first. Back up the Vault and preserve
its encryption key before removing the application home: that directory holds
sessions, configuration and other useful state, not just the executable.
See [installation details](../matrix_installation.md) for platform layout.

## Next steps

- [Core Concepts](Core-Concepts.md)
- [Using Agents](Using-Agents.md)
- [Channels](Channels.md)
- [Examples](Examples.md)
