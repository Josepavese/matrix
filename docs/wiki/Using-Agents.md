# Using Agents

Matrix connects to real AI coding agents that you already have installed. This page shows you how to find, install, configure, and switch between them.

## Agent Basics

Each agent in Matrix has:

- A **name** (like `opencode`, `claude`, `gemini`)
- A **command** (the binary to run, like `claude-agent-acp`)
- A **protocol** (how Matrix talks to it -- ACP or A2A)
- An **active/inactive** status

Protocol behavior is capability-gated. For ACP, Matrix follows the Zed Agent
Client Protocol and reads provider-advertised capabilities before using
features such as `session/list`, `session/close`, `session/fork`, or
`additionalDirectories`.

## Listing Agents

See which agents Matrix knows about:

```bash
matrix agent list
```

Output shows each agent's name, command, protocol, and whether it is active.

Get detailed info about one agent:

```bash
matrix agent info opencode
```

## Discovering New Agents

Search the ACP Registry and A2A catalogs:

```bash
matrix agent search <query>
```

This looks for agents in the configured discovery sources (ACP Registry, A2A catalogs, and local vault).

## Installing Agents

Install an agent from the registry:

```bash
matrix install <agent-id>
```

Matrix downloads the agent binary (supports npm/npx, Python/uvx, and direct binary distributions) and registers it in the vault.

Uninstall:

```bash
matrix uninstall <agent-id>
```

## Configuring Agents

### Set a custom binary path

If the agent binary is in a non-standard location:

```bash
matrix agent set-binary claude /usr/local/bin/claude
```

### Set environment variables

Pass environment variables to an agent:

```bash
matrix agent env set claude ANTHROPIC_API_KEY sk-...
```

### Set the endpoint

For networked agents (WebSocket, HTTP):

```bash
matrix agent set-endpoint gemini ws://localhost:3000 --kind acp --transport ws
```

### Override agent settings

```bash
matrix agent set-binary opencode /custom/path/opencode --args acp
```

Append launch arguments without changing the seed definition:

```bash
matrix agent args set codex -- -c 'sandbox_mode="danger-full-access"' -c 'approval_policy="never"'
matrix agent args list codex
```

These launch arguments are global for the stored agent endpoint. For Codex
reasoning effort, HTTP clients can instead set a per-run override:

```bash
curl -X POST http://127.0.0.1:9091/v1/runs \
  -H "Content-Type: application/json" \
  -d '{
    "channel_id": "halfdesk.pm",
    "agent_id": "codex",
    "input": "Prepare the PM summary",
    "agent_config": {
      "model_reasoning_effort": "xhigh"
    }
  }'
```

Supported values are `low`, `medium`, `high`, and `xhigh`. Matrix rejects the
request before launch if the selected agent does not resolve to `codex`, if the
value is unsupported, or if `agent_config` and `codex_config` provide different
values. The run trace records the applied value on `routing.decision` under
`protocol_meta.agent_launch_policy.model_reasoning_effort`.

### Check agent health

Run diagnostics on an agent:

```bash
matrix agent doctor claude
```

This checks the binary path, protocol connectivity, and configuration.

## Switching the Default Agent

The default agent handles new conversations when no specific agent is requested.

```bash
matrix config set default_agent claude
```

Or for a specific workspace:

```bash
matrix workspace add my-project --default-agent gemini
```

## Enabling and Disabling Agents

Enable an agent so Matrix can route to it:

```bash
matrix agent enable claude
```

Disable without removing:

```bash
matrix agent disable claude
```

## Using Multiple Agents

### Per-prompt routing

Specify which agent should handle a particular prompt:

```bash
curl -X POST http://127.0.0.1:9091/v1/runs \
  -H "Content-Type: application/json" \
  -d '{
    "channel_id": "docs.http",
    "input": "Review this code for security issues",
    "agent_id": "claude"
  }'
```

### Handoff

Transfer work from one agent to another mid-session:

```
/handoff gemini
```

Matrix creates a handoff packet with full context and routes the next turn to Gemini.

Read more: [Handoff](Handoff.md)

### ACP fork vs Matrix sidecar

ACP does not expose a `side` or `session/side` method. If a workflow needs a
separate provider branch, Matrix uses real ACP `session/fork` only when the
agent advertises it. If a workflow needs auxiliary context, Matrix uses sidecar
capsules and projects them into the selected protocol without making them normal
chat text.

Read more: [Zed ACP Compliance](../matrix_zed_acp_compliance.md)

### Meta-agent

The `/action` command delegates system administration tasks to a designated meta-agent:

```
/action install the latest version of opencode
```

The meta-agent (configured via `action_agent`, defaults to `gemini`) has access to system tools for installing agents, changing configuration, and performing diagnostics.

```bash
matrix config set action_agent claude
```

## Pre-configured Agents

Matrix ships with these agents pre-configured:

| Agent | ID | Command | Notes |
|-------|----|---------|-------|
| OpenCode | `opencode` | `opencode acp` | Default agent for new sessions |
| Gemini CLI | `gemini` | `gemini --acp` | Default meta-agent for `/action` |
| Claude Code | `claude` | `claude-agent-acp` | Available but inactive by default |
| Kimi | `kimi` | `kimi acp` | Available but inactive by default |

You can modify these, add new ones, or remove them entirely.

## Agent Configuration File

Agent definitions are stored in `configs/agents.json` (and optionally `configs/agents.local.json` for local overrides):

```json
{
  "claude": {
    "command": "claude",
    "args": ["acp"],
    "kind": "acp",
    "transport": "stdio",
    "env_isolation": true,
    "active": false
  }
}
```

Fields:

| Field | Meaning |
|-------|---------|
| `command` | The binary to execute |
| `args` | Arguments passed to the command |
| `kind` | Agent protocol family, usually `acp` or `a2a` |
| `transport` | Wire transport, usually `stdio`, `ws`, or `http` |
| `env_isolation` | Whether to isolate the agent's environment |
| `active` | Whether Matrix will route to this agent |
| `healthcheck_path` | Optional health check endpoint |

## Trust Mode

Control whether agent tool requests are auto-approved:

```bash
matrix config set agent.trust_mode true
```

Options:

- `true` -- auto-approve all tool requests
- `false` -- deny direct agent tool requests by default

`agent.trust_mode` is Matrix-side trust. It controls Matrix's ACP file,
terminal, and permission request handler. It does not automatically change a
provider's own sandbox policy.

For Codex ACP trusted local workspace runs, configure both layers explicitly:

```bash
matrix config set agent.trust_mode true
matrix agent args set codex -- -c 'sandbox_mode="danger-full-access"' -c 'approval_policy="never"'
```

The appended Codex config is passed to `codex-acp` on launch. It is equivalent
to running Codex without its internal sandbox and without interactive approval
prompts for Matrix-routed HTTP runs. Run traces record the detected launch
policy under `routing.decision.protocol_meta.agent_launch_policy`, including
`trusted_terminal=true` when `sandbox_mode=danger-full-access` and
`approval_policy=never`.

If the daemon is already running as `matrix.service`, it may own the vault lock.
Stop the user service before applying local CLI config or agent overrides, then
start it again:

```bash
systemctl --user stop matrix.service
matrix config set agent.trust_mode true
matrix agent args set codex -- -c 'sandbox_mode="danger-full-access"' -c 'approval_policy="never"'
systemctl --user start matrix.service
```

## Troubleshooting

### Agent not found

```bash
matrix agent doctor <agent-id>
```

Check that the binary is in your PATH and the command is correct.

### Connection refused

For stdio agents, check that the command works standalone:

```bash
claude-agent-acp
```

For networked agents, check that the endpoint is reachable:

```bash
matrix agent show <agent-id>
```

### Agent keeps disconnecting

Matrix maintains a keepalive pool with 30-second health checks. If an agent repeatedly disconnects, check:

1. The agent binary is up to date
2. Sufficient system resources
3. The vault is not corrupted: `matrix vault doctor`

## Next

- [Handoff](Handoff.md) -- transfer work between agents
- [API Reference](API-Reference.md) -- run prompts programmatically
- [CLI Reference](CLI-Reference.md) -- all agent commands
