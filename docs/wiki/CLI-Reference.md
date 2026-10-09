# CLI Reference

Common `matrix` commands with examples. Use `matrix <command> --help` for the installed version's full flag inventory.

## Global Commands

### `matrix run`

Start the Matrix daemon. This starts the HTTP API server, Telegram bot (if configured), and JSON-RPC listener.

```bash
matrix run
```

### `matrix run submit`, `wait` and `ack`

```bash
# Linux/macOS only:
matrix run wait <run-id> --timeout 10m --after <cursor> --json
matrix run ack --run-id <run-id> --sequence <sequence> --idempotency-key <stable-key> --json
```

`run submit` exists, but the v0.1.53 client omits required `channel_id` and is
refused with HTTP 400; see the [open correction](../../issues/run-submit-missing-channel.md).
Use HTTP for explicit project-bound async submission. Wait reads Unix outcome notifications. It shows
input questions interactively but keeps waiting; JSON output accumulates them
until return. Use the direct socket SSE listener for immediate input intervention.
`--on-attention` also returns on a nonterminal inactivity notice.

Persist the reported cursor and keep acknowledgement keys stable on retry.
Windows uses HTTP run events/elicitations instead of wait/ack.
See [Delegation and Notifications](Delegation-and-Notifications.md).

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

Report first-run readiness and setup guidance. This command does not authenticate model accounts.

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

Configured launch policy adds requested/effective policy, application
mechanism, and verification status. Inapplicable policy reports
`launch_policy_invalid` and blocks dispatch.

### `matrix agent set-endpoint <agent-id> <address>`

Set the network endpoint for an agent.

```bash
matrix agent set-endpoint gemini ws://localhost:3000 --kind acp --transport ws
```

### `matrix agent set-binary <agent-id> <path>`

Set a custom binary path for an agent.

```bash
matrix agent set-binary claude /usr/local/bin/claude-agent-acp
```

### `matrix agent args`

Manage argument overrides appended to an agent command at launch time.

```bash
matrix agent args list codex
matrix agent args set codex -- -c 'sandbox_mode="danger-full-access"' -c 'approval_policy="never"'
matrix agent args append codex -- --verbose
matrix agent args clear codex
```

`set` replaces only the appended override arguments. It does not modify the
seed command or seed args stored for the agent.

Provider-policy adapters may translate recognized governed args to verified
provider surfaces. Canonical Codex uses `codex-acp-env-v1`; unrecognized args
remain ordinary wrapper argv.

### `matrix agent override`

Inspect or clear raw SSOT overrides.

```bash
matrix agent override show opencode
```

`show` reports the names the override sets and how many, never the values:
stdout reaches logs, shell history and shared screens. Add `--reveal-values` to
print the values, as `NAME=Value` entries.

### `matrix agent env set <agent-id> <key> <value>`

Set environment variables for an agent.

```bash
matrix agent env set claude ANTHROPIC_API_KEY sk-...
```

### `matrix agent env list <agent-id>`

List the environment variables an agent's override sets: the names by default,
one per line. `matrix agent env list <agent-id> --reveal-values` prints the
entries themselves, `NAME=Value`, because only the operator can decide that
stdout is not going into a log.

### `matrix agent show <agent-id>`

Show the full agent definition.

```bash
matrix agent show claude
```

The report carries the configuration, the environment it will apply and the
override: the environment variables and the endpoint headers are named and
counted (`env_names`, `env_count`, `header_names`, `header_count`) and their
values are not printed. `matrix agent show <agent-id> --reveal-values` prints the
values too. This is the same rule `matrix agent doctor` follows with
`effective_env_count` and `matrix agent set-endpoint` with `header_names`.

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

Switch an explicit channel binding to a different workspace (`--channel` is required).

```bash
matrix workspace switch my-project --channel docs.http
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
| `matrix_api_key` | HTTP API authentication key; generated on first daemon startup if absent |
| `jsonrpc_addr` | JSON-RPC daemon address |
| `daemon_api_key` | JSON-RPC daemon authentication; generated separately on first daemon startup if absent |
| `agent.trust_mode` | Auto-approve tool requests (`true` or `false`, default: `false`) |

When `matrix run` owns the bbolt vault, config and agent commands use the
authenticated runtime storage broker published inside the PAL. The CLI does
not open a second bbolt writer, and the service does not need to be stopped:

```bash
matrix config set agent.trust_mode true
matrix agent args set codex -- -c 'sandbox_mode="danger-full-access"' -c 'approval_policy="never"'
```

The ephemeral broker descriptor is `data/runtime-broker.json` under
`MATRIX_HOME`, has mode `0600` on Unix, contains a per-run random token, and is
removed on graceful shutdown. When no daemon is active, the CLI opens the Vault
directly as before. Read-only commands such as `matrix agent show`,
`matrix logs tail`, and `matrix doctor` follow the same broker-first policy.
The short-lived `data/runtime-broker.starting.json` ownership claim prevents a
CLI command from taking the bbolt lock while the daemon is starting.

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

`telegram.admins` is the enforced allow-list of Telegram user ids that may
drive the bot. A comma-separated list is accepted (`"111111111,222222222"`), as
is a JSON array. Matrix checks the sender before dispatching any update, so
messages, group messages, edited messages and inline-button presses from any
other user are dropped and logged with the sender's user id. An enabled channel
with an empty admin list refuses to start rather than accepting everyone.

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

The key is written as given, and that includes the retention keys that are not
`config.*`. One of them bounds how long the daemon keeps an acknowledgement
record before a replay of that key counts as a first claim again:

```bash
matrix vault set retention.notification_ack_max_age 3600
```

Unset means no expiry. See `POST /v1/run-notifications/ack` in the API reference
for what a configured window declares and what it does not.

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

Set `MATRIX_AGENT_REGISTRY_URL` to an HTTPS mirror containing a full ACP index
with mirrored artifact URLs for offline deployments. Without the override,
Matrix uses the public ACP registry. A matching artifact SHA-256 remains
required for verified binary installation.

A binary distribution whose registry entry publishes no `sha256` is refused:
there is nothing to verify the download against. To accept such an artifact
anyway, opt in explicitly. The override is logged, recorded in the install
evidence, and the agent keeps reporting as not verified:

```bash
matrix install <agent-id> --allow-unverified
```

`--allow-unverified` applies only to a missing digest: a digest that mismatches
or is malformed always refuses the install.

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

**Provider endpoints in logs.** What Matrix writes into its own logs withholds
the provider endpoint: the resolved address is recorded as `***`. That is the
default because a log is written without anyone asking for it. Set
`MATRIX_LOG_REVEAL_ENDPOINTS=1` to record the real address instead; that is the
only value that turns it on, and any other value keeps the redaction. Header
values are never logged at all, in either mode, with no override; the only place
that lists header names is the agent configuration display, and it is not a log.

This covers logging. A surface an operator queries on purpose, such as
`matrix agent doctor`, still shows the endpoint it was asked about.

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

### `matrix capacity`, `sandbox`, `fs` and `fuse`

Native capacity observation, explicit execution-policy prerequisites and read-only semantic state. Mounting additionally needs an installed PAL driver.

```bash
matrix fs list
matrix fs path runs <run-id>
matrix fs read runs/<encoded-id>/status.json
matrix fuse mount /existing/empty/mountpoint --driver-path /path/to/rclone
matrix capacity /existing/workspace
matrix sandbox doctor <agent-id> --workspace /existing/workspace
matrix sandbox command <agent-id> /guest/agent --arg=acp
```

See [PAL execution and observability](PAL-Execution-and-Observability.md) for
sandbox contracts, native prerequisites, admission and the optional collector.

---

## Next

- [API Reference](API-Reference.md) -- the same operations via HTTP
- [Channels](Channels.md) -- set up your channels
- [Getting Started](Getting-Started.md) -- back to the beginning
