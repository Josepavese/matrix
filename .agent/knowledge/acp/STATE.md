# ACP (Agent Client Protocol) Knowledge Base

> Last updated: 2026-04-08

## Spec Location
- Official: https://agentclientprotocol.com
- GitHub: https://github.com/agentclientprotocol/agent-client-protocol
- NOTE: `agentcommunicationprotocol.dev` is a DIFFERENT protocol (Linux Foundation/BeeAI, REST-based agent-to-agent). Not related.

## Transports
- **stdio**: JSON-RPC over stdin/stdout, newline-delimited. Spawned on-demand.
- **WebSocket** (`ws`): JSON-RPC over WebSocket to `ws://host:port`.
- **HTTP**: JSON-RPC over HTTP POST.
- **Unix socket** (`unix`): JSON-RPC over Unix domain socket, newline-delimited.

## Session Lifecycle

### 1. Initialize
- Method: `initialize`
- Params: `{ protocolVersion: int, clientInfo: { name, version } }`
- Response: `{ capabilities: {...} }`

### 2. Session/New
- Method: `session/new`
- Params: `{ cwd: string (required), mcpServers: [] (required), tools: [] (optional) }`
- Response: `{ sessionId, modes?: { currentModeId, availableModes }, configOptions?: [...] }`
- Mode is NOT set at creation time — modes are returned by the agent.

### 3. Set Mode
- **Preferred**: `session/set_config_option` with `{ sessionId, configId: "mode", value: "..." }`
- **Legacy (still valid)**: `session/set_mode` with `{ sessionId, modeId: "..." }`
- Method name uses **underscore** (`set_mode`), NOT camelCase (`setMode`)
- Mode IDs are **agent-defined strings** (e.g. "code", "ask", "architect", "yolo"), NOT protocol constants
- Available modes come from the `session/new` response

### 4. Prompt
- Method: `session/prompt`
- Params: `{ sessionId, prompt: [{ type: "text", text: "..." }] }`
- Streaming via `session/update` notifications with `sessionUpdate: "agent_message_chunk"`

## Permission Flow

### Agent → Client: `session/request_permission`
- Params: `{ sessionId, toolCall: {...}, options: [{ optionId, name, kind }] }`
- `kind` values: `"allow_once"`, `"allow_always"`, `"reject_once"`, `"reject_always"`

### Client → Agent Response (discriminated union)
```json
{
  "outcome": {
    "outcome": "selected",
    "optionId": "<chosen optionId>"
  }
}
```
Or cancel:
```json
{
  "outcome": {
    "outcome": "cancelled"
  }
}
```

**IMPORTANT**: The response is NOT `{"approved": true}`. It MUST use the `outcome` discriminated union.

## Agent → Client Methods (requests)
| Method | Purpose |
|--------|---------|
| `session/request_permission` | Ask client to approve/reject a tool call |
| `fs/read_text_file` | Read a file |
| `fs/write_text_file` | Write a file |
| `terminal/create` | Create terminal session |
| `terminal/output` | Terminal output |
| `terminal/release` | Release terminal |
| `terminal/wait_for_exit` | Wait for process exit |
| `terminal/kill` | Kill process |

## Client → Agent Methods
| Method | Purpose |
|--------|---------|
| `initialize` | Handshake |
| `authenticate` | Auth |
| `session/new` | Create session |
| `session/load` | Load existing session |
| `session/list` | List sessions |
| `session/set_mode` | Set mode (legacy) |
| `session/set_config_option` | Set config option (preferred) |
| `session/prompt` | Execute prompt turn |
