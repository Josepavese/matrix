# CLI Reference

Every `matrix` command with examples.

## Global Commands

### `matrix run`

Start the Matrix daemon. This starts the HTTP API server, Telegram bot (if configured), and JSON-RPC listener.

```bash
matrix run
```

### `matrix home`

Show the PAL home directory.

```bash
matrix home
```

### `matrix doctor`

Run a full system health check.

```bash
matrix doctor
```

### `matrix bootstrap doctor`

Run the first-time setup wizard. Checks environment, detects agents, writes initial configuration.

```bash
matrix bootstrap doctor
```

### `matrix readiness`

Run pre-flight checks before starting the daemon.

```bash
matrix readiness
```

---

## Agent Commands

### `matrix agent list`

List all configured agents.

```bash
matrix agent list
```

### `matrix agent info <agent-id>`

Show detailed information about one agent.

```bash
matrix agent info claude
```

### `matrix agent search <query>`

Search the ACP Registry and A2A catalogs for available agents.

```bash
matrix agent search code-review
```

### `matrix agent enable <agent-id>`

Enable an agent so Matrix will route to it.

```bash
matrix agent enable claude
```

### `matrix agent disable <agent-id>`

Disable an agent without removing it.

```bash
matrix agent disable kimi
```

### `matrix agent doctor <agent-id>`

Run diagnostics on an agent. Checks binary, protocol, and configuration.

```bash
matrix agent doctor opencode
```

### `matrix agent set-endpoint <agent-id> <address>`

Set the network endpoint for an agent.

```bash
matrix agent set-endpoint gemini ws://localhost:3000 --kind acp --transport ws
```

### `matrix agent set-binary <agent-id> <path>`

Set a custom binary path for an agent.

```bash
matrix agent set-binary claude /usr/local/bin/claude
```

### `matrix agent override`

Inspect or clear raw SSOT overrides.

```bash
matrix agent override show opencode
```

### `matrix agent env set <agent-id> <key> <value>`

Set environment variables for an agent.

```bash
matrix agent env set claude ANTHROPIC_API_KEY sk-...
```

### `matrix agent show <agent-id>`

Show the full agent definition.

```bash
matrix agent show claude
```

---

## Session Commands

### `matrix session attach <channel-id> <session-id>`

Attach a physical channel to an existing logical session.

```bash
matrix session attach telegram_123 sess-abc123
```

### `matrix session inspect <session-id>`

Show detailed session information.

```bash
matrix session inspect sess-abc123
```

---

## Workspace Commands

### `matrix workspace add <workspace-id>`

Create a new workspace and optionally bind it to a directory path.

```bash
matrix workspace add my-project --path /home/user/my-project
```

### `matrix workspace list`

List all workspaces.

```bash
matrix workspace list
```

### `matrix workspace state <workspace-id>`

Show current materialized workspace state.

```bash
matrix workspace state my-project
```

### `matrix workspace switch <name>`

Switch to a different workspace.

```bash
matrix workspace switch my-project
```

### `matrix workspace snapshots <workspace-id>`

List workspace snapshots.

```bash
matrix workspace snapshots my-project
```

### `matrix workspace timeline <workspace-id>`

Show the workspace event timeline.

```bash
matrix workspace timeline my-project
```

### `matrix workspace decisions <workspace-id>`

Show the orchestration decision trace.

```bash
matrix workspace decisions my-project
```

### `matrix workspace memory <workspace-id>`

Show workspace memory (turn summaries).

```bash
matrix workspace memory my-project
```

### `matrix workspace retention`

Manage workspace retention policies.

```bash
matrix workspace retention
```

---

## Configuration Commands

### `matrix config list`

List all configuration values.

```bash
matrix config list
```

### `matrix config get <key>`

Get a configuration value.

```bash
matrix config get default_agent
```

### `matrix config set <key> <value>`

Set a configuration value.

```bash
matrix config set default_agent claude
matrix config set matrix_http_addr 127.0.0.1:9092
```

Common configuration keys:

| Key | Description |
|-----|-------------|
| `default_agent` | Agent for new sessions (default: `opencode`) |
| `action_agent` | Meta-agent for `/action` (default: `gemini`) |
| `matrix_http_addr` | HTTP API address |
| `matrix_api_key` | HTTP API authentication key |
| `jsonrpc_addr` | JSON-RPC daemon address |
| `daemon_api_key` | JSON-RPC daemon authentication |
| `agent.trust_mode` | Auto-approve tool requests (`auto` or `manual`) |

### `matrix config delete <key>`

Delete a configuration value.

```bash
matrix config delete matrix_api_key
```

---

## Channel Commands

### `matrix channel list`

List supported channel providers.

```bash
matrix channel list
```

### `matrix channel show <provider>`

Show effective and override configuration for a channel provider.

```bash
matrix channel show telegram
```

### `matrix channel set <provider> <key> <value>`

Set a channel override in the SSOT vault.

```bash
matrix channel set telegram token "123456:ABC..."
matrix channel set telegram enabled true
matrix channel set telegram admins "123456789"
```

### `matrix channel delete <provider> <key>`

Delete a channel override from the SSOT vault.

```bash
matrix channel delete telegram token
```

---

## Vault Commands

### `matrix vault get <key>`

Get a vault entry.

```bash
matrix vault get session.meta.sess-123
```

### `matrix vault set <key> <value>`

Set a vault entry.

```bash
matrix vault set config.custom-key my-value
```

### `matrix vault backup`

Create a vault backup.

```bash
matrix vault backup
```

### `matrix vault restore <backup_path>`

Restore from a vault backup.

```bash
matrix vault restore ./backups/<backup-file>
```

### `matrix vault migrate`

Run vault schema migrations.

```bash
matrix vault migrate
```

### `matrix vault seal`

Seal the vault (enable encryption).

```bash
matrix vault seal
```

### `matrix vault doctor`

Run vault diagnostics.

```bash
matrix vault doctor
```

---

## Install Commands

### `matrix install <agent-id>`

Install an agent from the registry.

```bash
matrix install claude
```

### `matrix uninstall <agent-id>`

Uninstall an agent.

```bash
matrix uninstall claude
```

---

## Logs Commands

### `matrix logs tail`

Tail live logs.

```bash
matrix logs tail
```

### `matrix logs show-config`

Show effective logging configuration.

```bash
matrix logs show-config
```

### `matrix logs doctor`

Run log diagnostics.

```bash
matrix logs doctor
```

---

## Other Commands

### `matrix orchestration capabilities`

View orchestration capabilities.

```bash
matrix orchestration capabilities
```

### `matrix fuse`

FUSE filesystem operations (experimental).

```bash
matrix fuse
```

---

## Next

- [API Reference](API-Reference.md) -- the same operations via HTTP
- [Channels](Channels.md) -- set up your channels
- [Getting Started](Getting-Started.md) -- back to the beginning
