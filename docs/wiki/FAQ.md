# Frequently Asked Questions

## General

### What is Matrix?

A local communication hub for existing coding agents. It connects people and
supervisory software through shared run, session and workspace contracts.
See [Core Concepts](Core-Concepts.md).

### Is Matrix an AI agent?

Matrix routes and manages work; the connected agent generates the answer and
uses its tools. An upstream supervisor defines task planning and review policy.

### Which agents are supported?

The seed configuration includes OpenCode, Gemini CLI, Claude's ACP adapter and
Kimi. Codex has a dedicated ACP installer/launch contract; MiMo Code can be
registered and has a native permission adapter. Other ACP/A2A agents depend on
compatible transports and advertised capabilities. See [Using Agents](Using-Agents.md).

### Does Matrix replace my agents?

You keep their toolchains, model accounts and provider setup. Matrix adds common
communication and inspectable local state.

### Is Matrix a cloud service?

Matrix's daemon and Vault run locally. Connected agent/model services may be
remote, and optional discovery, installation or telemetry can use the network.
Local-first does not mean every prompt stays on the host.

## Setup

### How do I install Matrix?

Use the release installer for your OS. It installs into one PAL application home
without cloning the repository. [Getting Started](Getting-Started.md) covers
Linux/macOS and Windows, authentication and a first task.

### What are the prerequisites?

A supported OS, an accessible project and at least one working, authenticated
agent/adapter or configured A2A endpoint. Optional Docker/FUSE features have
additional prerequisites in the [PAL guide](PAL-Execution-and-Observability.md).

### How do I install from source?

Use the repository's [contribution and build instructions](../../CONTRIBUTING.md).
Public release archives are the normal installation path.

### Does Matrix work on Windows?

Yes: native application home, capacity observation and shared API/CLI contracts
are supported. Unix notification socket wait/ack commands are Linux/macOS only.
Windows supervisors use HTTP events and elicitations. Optional mounts need
WinFsp/rclone; containers need a local Linux-container engine. Contract tests
are distinct from real driver qualification.

## Usage

### How do I send a prompt to an agent?

Start or use the running daemon, register a workspace, and send an authenticated
`POST /v1/runs` naming `agent_id`, `workspace_id` and `channel_id`.
[Getting Started](Getting-Started.md#your-first-prompt) has Bash and PowerShell examples.

### How do I switch between agents?

Choose `agent_id` per request, or prepare a `/handoff <agent-id>` in chat and
then send the next specialist a task. Handoff is an operational brief, not a
full provider transcript import. See [Handoff](Handoff.md).

### How do I use Telegram?

Configure its token, enabled state and allowed numeric administrator IDs:

```bash
matrix channel set telegram token "your-bot-token"
matrix channel set telegram enabled true
matrix channel set telegram admins "your-numeric-user-id"
matrix channel show telegram
```

Restart the daemon/service after configuration as needed. An enabled Telegram
channel with no administrators refuses to start. See [Channels](Channels.md).

### Can I delegate and wait without streaming a transcript into my supervisor?

Yes. Submit an async run and retain its ID. Linux/macOS expose durable local
notifications; HTTP run events and the input-request API work across all three
OSes. Filter what the supervisor model actually needs. See
[Delegation and Notifications](Delegation-and-Notifications.md).

### Can Matrix show remaining token credits or account money?

Matrix does not currently provide a normalized account-balance query. ACP
usage/cost information, where advertised by a provider, describes session/run
consumption; it does not establish subscription credit or a remaining prepaid
balance. Matrix's A2A adapter has no account-balance query surface either.

Capacity means host disk/CPU/RAM, not billing. Model confirmation means the
provider confirmed a selector, not that Matrix verified billing or physical
model identity. An account-balance feature would need provider-specific sources
and authorization behind a common Matrix contract; it is not shipped here.

### Can I roll back code with a workspace snapshot?

Snapshots store Matrix work metadata and references. They do not copy or restore
project files. Use Git or your backup process for source rollback.

## Architecture

### Where is my data stored?

`matrix home` shows the resolved PAL home. Defaults: Linux
`$XDG_DATA_HOME/matrix` or `~/.local/share/matrix`, macOS
`~/Library/Application Support/Matrix`, Windows `%LOCALAPPDATA%\Matrix`.
The Vault is `data/matrix-vault.db`; logs and artifacts have separate directories.

### Is my data encrypted?

Vault value encryption requires a configured master key. Supported sources are
`MATRIX_VAULT_MASTER_KEY_FILE`, `MATRIX_VAULT_MASTER_KEY`, or the default
`configs/vault-master.key` under the resolved home. Without a key, values remain
local but unencrypted. `matrix vault doctor` reports the actual status.

Current encrypted values use AES-256-GCM with storage-key binding (`ENCV2`).
`matrix vault seal` seals values with the configured key; migration and backup
procedures are documented in the [CLI reference](CLI-Reference.md#vault-commands).
Encryption of Vault values does not encrypt every project file, log or provider
artifact. Preserve the key separately when backing up/restoring state.

### What protocols does Matrix support?

ACP agent communication and A2A task communication. Transports and individual
operations have specific support and capability gates; see
[protocol coverage](../protocol_coverage.md).

### Does ACP have a `side` or `session/side` feature?

Matrix sidecar is its own auxiliary-context abstraction, projected into the
selected protocol. ACP native branching uses advertised `session/fork`; it is
separate from capsule delivery. See [Sidecar Capsules](Sidecar-Capsules.md).

### Which latest ACP additions does Matrix track?

The [ACP compliance document](../matrix_zed_acp_compliance.md) and
[protocol coverage](../protocol_coverage.md) record stable and draft operations,
usage updates, elicitation and configuration support. Runtime capabilities
determine what the selected provider can actually do.

### How does Matrix connect to agents?

Managed ACP stdio processes, configured external ACP endpoints and A2A HTTP
bindings. Use the agent doctor and runtime registration state to verify your
chosen adapter, not just the presence of its executable.

### Are sandbox and semantic filesystem features automatic?

They are explicit, optional contracts. Native provider permissions and Docker
isolation are separate; the semantic view exposes selected Matrix state, not
project files. The OTLP exporter sends operational logs only. See the
[PAL guide](PAL-Execution-and-Observability.md).

## Troubleshooting

### `matrix` command not found

Open a new terminal and check the OS-specific PATH setup in
[Getting Started](Getting-Started.md).

### Agent not responding

```bash
matrix agent doctor <agent-id>
matrix logs doctor
```

Inspect registration, command/endpoint, provider authentication and run trace.
An input request, rate limit, protocol failure and process failure need different
responses. See [Sessions and Recovery](Sessions-and-Recovery.md).

### External session will not restore

Verify the remote ID and original workspace, inspect provider capabilities and
check typed import/restore errors. Matrix should not silently substitute a new
conversation. See [Sessions and Recovery](Sessions-and-Recovery.md).

### Telegram bot not working

Check `matrix channel show telegram`, token, enabled state and numeric admin
allowlist. Review logs and restart the configured service after changes.

### Vault corrupted

Use `matrix vault doctor`, retain the current files, and restore a verified
backup with its matching key. Do not delete the application home to troubleshoot.

### Port already in use

Check whether Matrix already runs as a service. If a distinct instance is
intentional, configure a separate home and listening address. The address key
is `matrix_http_addr`.

### How do I see logs?

```bash
matrix logs tail
matrix logs show-config
matrix logs doctor
```

Run traces expose task-level detail under their content policy. Exporting OTLP
logs does not export the full trace.

### How do I reset everything?

Stop Matrix, export/backup useful state and preserve the encryption key first.
Removing the PAL home removes conversations and configuration as well as the
binary. A clean installation is a separate operation from repairing a provider
or restoring a session.

## More help

- [Examples](Examples.md)
- [CLI Reference](CLI-Reference.md)
- [API Reference](API-Reference.md)
- [Report an issue](https://github.com/Josepavese/matrix/issues)
