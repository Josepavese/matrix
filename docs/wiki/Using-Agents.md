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

The [README agent grid](../../README.md#agents-on-acp) follows Zed's ACP
ecosystem directory. Some agents require an adapter; the grid is not a list of
Matrix end-to-end certifications. Inspect capabilities and your local setup.

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

For an installation without GitHub access, mirror the complete ACP registry
index and the artifact URLs it contains to an administrator controlled HTTPS
endpoint, then select that index for discovery and installation:

```bash
MATRIX_AGENT_REGISTRY_URL=https://mirror.example.org/acp/registry.json matrix install <agent-id>
```

The override also applies to `matrix agent search` and registry backed agent
information. Without it Matrix uses the public ACP registry. The override URL
must use HTTPS. Each binary artifact still needs a valid `sha256` in the index;
the existing digest, extraction, and command path checks remain active. The
mirror operator is responsible for verifying and pinning the index itself.
An index that still points an artifact at GitHub will still need GitHub access.

Matrix downloads the agent binary (supports npm/npx, Python/uvx, and direct binary distributions) and registers it in the vault.

For a binary distribution Matrix verifies the downloaded archive against the
`sha256` the registry index publishes for this platform, and extracts it only
after the match. When the index publishes no digest there is nothing to verify,
so the install is refused and says which artifact is missing what. If you have
checked such an entry yourself and accept an unverified artifact, opt in
explicitly:

```bash
matrix install <agent-id> --allow-unverified
```

The override is printed during the install and recorded in the install evidence,
which `matrix agent doctor <agent-id>` and
`matrix agent info <agent-id> --source=local` keep reporting as not verified.
The launcher path a binary distribution names must resolve inside the agent's own
directory, and the package identifier of an npx/uvx distribution must be a
package name: an index entry that points elsewhere is refused.

Uninstall:

```bash
matrix uninstall <agent-id>
```

## Configuring Agents

### Set a custom binary path

If the agent binary is in a non-standard location:

```bash
matrix agent set-binary claude /usr/local/bin/claude-agent-acp
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
curl -H "X-Matrix-Key: $MATRIX_API_KEY" -X POST http://127.0.0.1:9091/v1/runs \
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
`protocol_meta.agent_launch_policy.effective.model_reasoning_effort`.

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
curl -H "X-Matrix-Key: $MATRIX_API_KEY" -X POST http://127.0.0.1:9091/v1/runs \
  -H "Content-Type: application/json" \
  -d '{
    "channel_id": "docs.http",
    "input": "Review this code for security issues",
    "agent_id": "claude"
  }'
```

### Handoff

Prepare another specialist session within the workspace:

```
/handoff gemini
```

Matrix prepares a metadata-based brief for the next Gemini turn. It does not import the source provider transcript; supply the task and evidence explicitly.

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

The release seed definitions include the following agents. Enabled does not mean installed or authenticated:

| Agent | ID | Command | Notes |
|-------|----|---------|-------|
| OpenCode | `opencode` | `opencode acp` | Default agent for new sessions |
| Gemini CLI | `gemini` | `gemini --acp` | Default meta-agent for `/action` |
| Claude Code | `claude` | `claude-agent-acp` | Available but inactive by default |
| Kimi | `kimi` | `kimi acp` | Available but inactive by default |

Codex uses Matrix's dedicated ACP installer and `codex-acp-env-v1` launch
contract. MiMo Code can be registered as a compatible ACP agent and supports
`mimocode-permission-v1`. They are not additional entries in the shipped seed
JSON. Discover/install entries using `matrix agent search` and `matrix install`;
inspect the registered command with `matrix agent show <id>`.

For OpenCode use the configured ACP command; some Gemini installations use a
different ACP flag. Diagnose the actual installed version and adapter before
changing arguments. Matrix does not infer compatibility from the agent name.

See [PAL Execution and Observability](PAL-Execution-and-Observability.md) for
OpenCode/MiMo native permissions and optional Docker isolation.

## Agent Configuration File

`configs/agents.json` and `configs/agents.local.json` are bootstrap seed files.
Existing runtime definitions and overrides are Vault-backed; use CLI
configuration/installation commands rather than editing seed files as a live
configuration update. A seed definition looks like:

```json
{
  "claude": {
    "command": "claude-agent-acp",
    "args": [],
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

Agents can also ask you structured questions mid-run (ACP elicitation). This is
**opt-in and off by default**:

```bash
matrix config set agent.elicitation_enabled true
matrix config set agent.elicitation_timeout_seconds 300
```

When enabled, Matrix advertises the capability, exposes pending requests at
`GET`/`POST /v1/elicitations` (see the
[API Reference](API-Reference.md#elicitations)), and asks in Telegram whenever
the request belongs to a chat the bot owns. Unanswered requests expire as
`cancel` after `agent.elicitation_timeout_seconds` (default 120); cancelling the
run revokes its pending question immediately.

In Telegram the question arrives as a message with inline buttons:

- constrained fields (options, booleans) become one button per choice, with the
  label the agent supplied;
- press `Invia` to confirm, or `Rifiuta`/`Annulla` to decline or abort at any
  time;
- a question that needs free text is answered by replying with a message in the
  chat (that next message is the answer, not a new prompt);
- URL-mode requests show the target host and the full URL and only proceed on
  explicit consent; Matrix never opens the link itself.

A form that mixes free-text and other fields is not answerable from Telegram:
the chat offers decline/cancel, and the HTTP API remains the complete surface.

Why it is off by default: advertising the capability changes how conforming
agents route their questions. `codex-acp` sends MCP-server input requests as
elicitation instead of `session/request_permission` whenever the client
advertises it, so enabling elicitation without a human actually answering turns
approvals that `agent.trust_mode` used to settle immediately into requests that
wait and then expire as `cancel`. Enable it when you are watching the
elicitation surface; leave it off for unattended or auto-approved runs.

For Codex ACP trusted local workspace runs, configure both layers explicitly:

```bash
matrix config set agent.trust_mode true
matrix agent args set codex -- -c 'sandbox_mode="danger-full-access"' -c 'approval_policy="never"'
```

Matrix translates these governed args through Codex policy adapter into
`INITIAL_AGENT_MODE=agent-full-access` and `CODEX_CONFIG`; ignored wrapper argv
never carries policy. Matrix then verifies or sets exact ACP session mode after
new, resume, and load. Run traces separate `requested`, `effective`,
`application_mechanism`, and `verification_status`; `trusted_terminal=true`
appears only when effective full-access policy is verified.

After upgrading from a release that installed Codex without policy contract
marker, run `matrix install codex`. Until then, runtime and
`matrix agent doctor codex` fail closed with `launch_policy_invalid`.

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

## Sessions and outcomes

Registered agent availability is not proof of a restored conversation or a
successful run. Use [Sessions and Recovery](Sessions-and-Recovery.md) for exact
external IDs and typed failures, and [Delegation and Notifications](Delegation-and-Notifications.md)
for async work and input requests. Model traces and usage events do not establish
remaining account money or subscription credits.

## Next

- [Handoff](Handoff.md) -- transfer work between agents
- [API Reference](API-Reference.md) -- run prompts programmatically
- [CLI Reference](CLI-Reference.md) -- all agent commands
