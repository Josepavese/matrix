# Frequently Asked Questions

## General

### What is Matrix?

Matrix is a local-first communication hub for AI coding agents. It runs on your machine, connects to the coding agents you already use (Claude, Gemini, OpenCode, Codex), and gives you one place to interact with all of them.

### Is Matrix an AI agent?

No. Matrix does not generate code or make decisions. It routes your prompts to real agents and manages sessions, workspaces, and handoffs between them.

### Which agents are supported?

Any agent that speaks ACP (Agent Client Protocol) or A2A (Agent-to-Agent). Out of the box: OpenCode, Claude Code, Gemini CLI, and Kimi.

### Does Matrix replace my agents?

No. You keep using the same agents. Matrix is the communication layer between you and them.

### Is Matrix a cloud service?

No. Matrix runs entirely on your machine. All state is stored locally in an encrypted vault. There is no cloud dependency.

---

## Setup

### How do I install Matrix?

```bash
curl -fsSL https://raw.githubusercontent.com/Josepavese/matrix/main/install/install.sh | sh
```

Then run `matrix bootstrap doctor` to set it up. See [Getting Started](Getting-Started.md) for details.

### What are the prerequisites?

You need at least one AI coding agent installed (OpenCode, Claude Code, or Gemini CLI). Matrix routes to these agents, so they need to be available on your machine.

### How do I install from source?

Clone the repository and build with Go:

```bash
git clone https://github.com/Josepavese/matrix.git
cd matrix
go build -o matrix ./cmd/matrix
```

### Does Matrix work on Windows?

Yes. PowerShell install:

```powershell
irm https://raw.githubusercontent.com/Josepavese/matrix/main/install/install.ps1 | iex
```

---

## Usage

### How do I send a prompt to an agent?

Start the daemon (`matrix run`), then use the HTTP API:

```bash
curl -X POST http://127.0.0.1:9091/v1/runs \
  -H "Content-Type: application/json" \
  -d '{
    "channel_id": "docs.http",
    "input": "Explain this code"
  }'
```

Or use Telegram if configured. See [Channels](Channels.md).

### How do I switch between agents?

Use handoff to transfer work between agents:

```
/handoff claude
```

Or set a different default agent:

```bash
matrix config set default_agent claude
```

See [Using Agents](Using-Agents.md) and [Handoff](Handoff.md).

### How do I use Telegram?

Configure the bot token and admin ID, then restart:

```bash
matrix channel set telegram.token "your-bot-token"
matrix channel set telegram.enabled true
matrix channel set telegram.admins "your-user-id"
matrix run
```

See [Channels](Channels.md#telegram) for full instructions.

### Can I use Matrix from my phone?

Yes, via Telegram. Set up the Telegram channel and talk to your agents from any device.

---

## Architecture

### Where is my data stored?

In the PAL home directory:

- Linux: `~/.local/share/matrix/`
- macOS: `~/Library/Application Support/Matrix/`
- Windows: `%LOCALAPPDATA%\Matrix\`

The vault database is `data/matrix-vault.db`.

### Is my data encrypted?

The vault supports optional AES encryption via `MATRIX_VAULT_MASTER_KEY`. Without a master key, data is stored unencrypted locally.

```bash
matrix vault seal
```

### What protocols does Matrix support?

ACP (Agent Client Protocol) is the operational default. A2A (Agent-to-Agent) is implemented and ready. The routing core is protocol-agnostic.

### How does Matrix connect to agents?

Agents communicate via stdio (standard input/output), WebSocket, or HTTP. The default mode for most agents is stdio. Matrix spawns the agent process and communicates through its standard streams.

---

## Troubleshooting

### `matrix` command not found

Open a new terminal. The installer adds the binary to your PATH, but existing terminals may not see it.

### Agent not responding

```bash
matrix agent doctor <agent-id>
```

Check that the agent binary is in your PATH and the command is correct.

### Telegram bot not working

1. Verify the token: `matrix channel`
2. Check that `telegram.enabled` is `true`
3. Make sure your Telegram user ID is in the admin list
4. Restart Matrix: `matrix run`

### Vault corrupted

```bash
matrix vault doctor
```

If needed, restore from a backup:

```bash
matrix vault restore
```

### Port already in use

Change the HTTP address:

```bash
matrix config set matrix_http_addr 127.0.0.1:9092
```

### How do I see logs?

```bash
matrix logs show
matrix logs tail    # live tail
```

### How do I reset everything?

Remove the PAL home directory and reinstall:

```bash
rm -rf "${XDG_DATA_HOME:-$HOME/.local/share}/matrix"
curl -fsSL https://raw.githubusercontent.com/Josepavese/matrix/main/install/install.sh | sh
```

---

## More Help

- [Getting Started](Getting-Started.md) -- full setup guide
- [Core Concepts](Core-Concepts.md) -- understand how Matrix works
- [CLI Reference](CLI-Reference.md) -- every command documented
- [Report an issue](https://github.com/Josepavese/matrix/issues) -- bugs and feature requests
