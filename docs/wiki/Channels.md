# Channels

Channels are how you communicate with Matrix. All channels share the same sessions, workspaces, and agents.

## Telegram

Telegram gives you a chat-based interface to your agents from any device.

### Setup

1. Create a Telegram bot via [@BotFather](https://t.me/BotFather)
2. Get the bot token
3. Configure Matrix:

```bash
matrix channel set telegram.token "123456:ABC-DEF..."
matrix channel set telegram.enabled true
matrix channel set telegram.admins "your-telegram-user-id"
```

Or via environment variables:

```bash
export MATRIX_TELEGRAM_TOKEN="123456:ABC-DEF..."
export MATRIX_TELEGRAM_ENABLED=true
export MATRIX_TELEGRAM_ADMINS="your-telegram-user-id"
```

4. Restart Matrix:

```bash
matrix run
```

### Using Telegram

Once configured, talk to your bot directly:

```
What files are in the current project?
```

Matrix routes it to your default agent and replies with the result.

### Commands

Chat commands are parsed by exact first token. For example, `/session list` is a
command, while `/session-list` is treated as an unknown command and will not
accidentally trigger `/session`.

| Command | What It Does |
|---------|-------------|
| `/status` | Current workspace summary |
| `/now` | Current operating state |
| `/timeline` | Recent workspace events |
| `/decisions` | Orchestration decisions |
| `/why` | Latest routing decision |
| `/memory` | Workspace memory |
| `/snapshots` | List snapshots |
| `/snapshot <note>` | Create a snapshot |
| `/use <workspace>` | Switch workspace |
| `/workspaces` | List workspaces |
| `/continue` | Continue current work |
| `/resume` | Resume workspace context |
| `/review` | Enter review mode |
| `/explain` | Enter explain mode |
| `/triage` | Enter triage mode |
| `/handoff <agent>` | Hand off to another agent |
| `/session new [agent]` | Create a new session |
| `/stop` | Cancel current session |
| `/action <instruction>` | Delegate to meta-agent |
| `/help` | Show help |

### Real-time indicators

Telegram shows live "thinking" updates while an agent is processing your request. You will see tool calls and progress in real time.

### Advanced commands

```
/session list
/session inspect
/workspace bind <workspace>
/workspace switch <workspace>
```

These give you lower-level control when you need it.

## HTTP API

The HTTP API is the primary programmatic interface. Matrix listens on `127.0.0.1:9091` by default.

### Authentication

Optional API key via the `X-Matrix-Key` header:

```bash
matrix config set matrix_api_key my-secret-key
```

Then include it in requests:

```bash
curl -H "X-Matrix-Key: my-secret-key" http://127.0.0.1:9091/_matrix/runtime
```

### Run a prompt

```bash
curl -X POST http://127.0.0.1:9091/v1/runs \
  -H "Content-Type: application/json" \
  -d '{
    "channel_id": "docs.http",
    "input": "Explain this function",
    "execution_mode": "sync"
  }'
```

Modes:
- `sync` -- wait for the full result (default)
- `async` -- return immediately, poll for results
- `stream` -- stream results as they arrive

### Attach sidecar context

Programmatic callers can keep the human task body separate from machine-trackable context:

```bash
curl -X POST http://127.0.0.1:9091/v1/runs \
  -H "Content-Type: application/json" \
  -d '{
    "channel_id": "supervisor.noema",
    "agent_id": "opencode",
    "input": {
      "text": "Update the parser without breaking existing config keys."
    },
    "sidecar_capsules": [
      {
        "provider": "noema",
        "id": "caps_parser",
        "visibility": "llm_visible",
        "format": "noema_xml",
        "content": "<noema id=\"caps_parser\">avoid: do not rename config keys</noema>"
      }
    ]
  }'
```

Matrix projects the capsule into ACP/A2A and records `sidecar.capsule.delivered` in the run trace. Chat frontends should not render raw capsule content as normal user text.

### Check run status

```bash
curl http://127.0.0.1:9091/v1/runs/{run_id}/trace
```

### Session management

```bash
curl -X POST http://127.0.0.1:9091/v1/session-actions \
  -H "Content-Type: application/json" \
  -d '{
    "channel_id": "docs.http",
    "action": "list"
  }'
```

Actions: `new`, `list`, `status`, `switch`, `cancel`, `delete`, `cleanup`,
`name`, `capabilities`, `fork`, `fork_status`, `reconcile`

`capabilities`, `fork`, `fork_status`, and `reconcile` use the same
channel-neutral contract as Telegram and future ingress adapters. `fork` is
provider-gated: Matrix calls a real provider fork when available and otherwise
returns a typed `unsupported=true` response. Fork capability descriptors include
`active_parent_safe`, `requires_idle_parent`, `artifact_turn`,
`async_supported`, `blocking`, `artifact_streaming`, and
`live_intervention_suitable`. `active_parent_safe` means state safety only, not
fast live delivery. For live sidecar use, callers can set `async=true` and poll
`fork_status` with the returned `fork.job_id`.

Text channels expose the same surface through `/session`:

```text
/session capabilities [agent]
/session fork [target] [--async --restore-parent --ephemeral --cleanup-policy delete_remote_or_cancel_and_forget_local --input prompt]
/session fork-status <forkjob-id>
/session reconcile
```

### Workspace management

```bash
curl -X POST http://127.0.0.1:9091/v1/workspace-actions \
  -H "Content-Type: application/json" \
  -d '{
    "channel_id": "docs.http",
    "action": "list"
  }'
```

Actions: `list`, `status`, `snapshot`, `switch`, `bind`

Full documentation: [API Reference](API-Reference.md)

## CLI

The CLI is for quick commands, configuration, and inspection.

### Start Matrix

```bash
matrix run
```

### Health check

```bash
matrix doctor
```

### Configuration

```bash
matrix config list
matrix config get default_agent
matrix config set default_agent claude
```

### Logs

```bash
matrix logs show
matrix logs tail
matrix logs doctor
```

### Vault

```bash
matrix vault list
matrix vault get session.meta.sess-123
matrix vault backup
matrix vault restore
matrix vault doctor
```

Full documentation: [CLI Reference](CLI-Reference.md)

## Channel Neutrality

All channels expose the same semantics:

- Same session lifecycle (create, list, switch, cancel, delete)
- Same workspace operations (list, status, switch, bind, snapshot)
- Same intents (continue, resume, review, explain, triage, handoff)
- Same agent routing

The channel is just an access surface. The work stays the same.

## Next

- [API Reference](API-Reference.md) -- complete HTTP endpoint documentation
- [CLI Reference](CLI-Reference.md) -- all `matrix` commands
- [Getting Started](Getting-Started.md) -- set up your first channel
