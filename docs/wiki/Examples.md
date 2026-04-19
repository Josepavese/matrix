# Examples

Step-by-step walkthroughs of common Matrix workflows.

## Example 1: First Agent Conversation

You just installed Matrix and want to send your first prompt.

```bash
# Start the daemon
matrix run

# Send a prompt via HTTP
curl -X POST http://127.0.0.1:9091/v1/runs \
  -H "Content-Type: application/json" \
  -d '{
    "channel_id": "docs.http",
    "input": "What is the structure of this project?"
  }'
```

Matrix routes the prompt to your default agent (OpenCode) and returns the result. No workspace setup needed for a quick test.

---

## Example 2: Multi-Agent Project Workflow

You have a project and want to use different agents for different tasks.

### Step 1: Create a workspace

```bash
matrix workspace add billing-api --path /home/user/billing-api
```

### Step 2: Start with OpenCode for implementation

```bash
curl -X POST http://127.0.0.1:9091/v1/runs \
  -H "Content-Type: application/json" \
  -d '{
    "channel_id": "docs.http",
    "input": "Add input validation to the /payments endpoint",
    "workspace_id": "billing-api"
  }'
```

### Step 3: Snapshot before review

```
/snapshot before-review
```

### Step 4: Hand off to Claude for code review

```
/handoff claude
```

Claude receives a handoff packet with full context and continues from where OpenCode left off.

### Step 5: Check the timeline

```
/timeline
```

Output:

```
[1] handoff created opencode -> claude - Review the billing API patch [2026-04-16 14:45]
[2] snapshot created: before-review [2026-04-16 14:44]
[3] entered implementation mode [2026-04-16 14:30]
[4] created session for opencode [2026-04-16 14:00]
```

### Step 6: Continue work the next day

```
/resume billing-api
```

Matrix restores the workspace state, picks up the last session, and you are back where you left off.

---

## Example 3: Telegram Bot Setup

Set up Matrix as a Telegram bot so you can talk to your agents from your phone.

### Step 1: Create a Telegram bot

1. Open Telegram and message [@BotFather](https://t.me/BotFather)
2. Send `/newbot` and follow the prompts
3. Save the bot token (looks like `123456789:ABCdefGHIjklMNOpqrSTUvwxYZ`)

### Step 2: Get your Telegram user ID

1. Message [@userinfobot](https://t.me/userinfobot)
2. Save your numeric user ID

### Step 3: Configure Matrix

```bash
matrix channel set telegram.token "123456789:ABCdefGHIjklMNOpqrSTUvwxYZ"
matrix channel set telegram.enabled true
matrix channel set telegram.admins "123456789"
```

### Step 4: Restart

```bash
matrix run
```

### Step 5: Use it

Open Telegram, find your bot, and send:

```
What files are in the billing-api project?
```

The bot responds with the agent's answer. Try:

```
/review
/handoff gemini
/timeline
```

---

## Example 4: Scripted Agent Workflow

Use the HTTP API to build a scripted CI/CD workflow that uses agents.

```bash
#!/bin/bash
MATRIX="http://127.0.0.1:9091"
KEY="your-api-key"

# Run tests via OpenCode
RUN_ID=$(curl -s -X POST "$MATRIX/v1/runs" \
  -H "Content-Type: application/json" \
  -H "X-Matrix-Key: $KEY" \
  -d '{
    "channel_id": "docs.http",
    "input": "Run the test suite and report any failures",
    "workspace_id": "billing-api",
    "execution_mode": "sync"
  }' | jq -r '.run_id')

echo "Run: $RUN_ID"

# Check the trace
curl -s "$MATRIX/v1/runs/$RUN_ID/trace" \
  -H "X-Matrix-Key: $KEY" | jq '.'
```

---

## Example 5: Workspace Memory and Snapshots

Track work over time using workspace memory and snapshots.

```bash
# Create a workspace for a long-running project
matrix workspace add migration-tool --path /home/user/migration-tool

# Day 1: Implementation
curl -X POST http://127.0.0.1:9091/v1/runs \
  -H "Content-Type: application/json" \
  -d '{
    "channel_id": "docs.http",
    "input": "Implement the database migration helper",
    "workspace_id": "migration-tool"
  }'

# Snapshot at a good stopping point
# In Telegram: /snapshot day1-implementation-done

# Day 2: Review and iterate
# In Telegram: /resume migration-tool
# In Telegram: /review

# Check what happened
# In Telegram: /memory
# In Telegram: /timeline
# In Telegram: /snapshots
```

The workspace remembers turn-by-turn summaries across sessions. When you resume on Day 2, the context is there.

---

## Example 6: Using the Meta-Agent

Delegate system tasks to the meta-agent.

```
/action install the latest version of opencode
```

```
/action change the default agent to claude
```

```
/action check if all agents are healthy
```

The meta-agent (Gemini by default) has system tool access and can perform administrative tasks on your behalf.

---

## Example 7: Streaming a Long-Running Task

For tasks that take time (code generation, analysis), use stream mode to see progress.

```bash
curl -N -X POST http://127.0.0.1:9091/v1/runs \
  -H "Content-Type: application/json" \
  -d '{
    "channel_id": "docs.http",
    "input": "Refactor the authentication module to use JWT tokens",
    "execution_mode": "stream",
    "workspace_id": "billing-api"
  }'
```

The `-N` flag tells curl to stream the response. You will see partial results as the agent works.

---

## Example 8: Supervisor Sidecar Context

Use sidecar capsules when a supervisor wants to attach evidence or constraints without making them ordinary chat history.

```bash
curl -X POST http://127.0.0.1:9091/v1/runs \
  -H "Content-Type: application/json" \
  -d '{
    "channel_id": "supervisor.noema",
    "agent_id": "opencode",
    "execution_mode": "sync",
    "input": {
      "text": "Add optional timeout support to the config parser."
    },
    "sidecar_capsules": [
      {
        "provider": "noema",
        "id": "caps_timeout",
        "schema": "sidecar.intent.v0",
        "version": "0.1",
        "visibility": "llm_visible",
        "format": "noema_xml",
        "content": "<noema id=\"caps_timeout\">success: existing tests pass; avoid: do not make timeout mandatory</noema>"
      }
    ]
  }'
```

The agent receives the model-visible guidance. The trace records `sidecar.capsule.delivered`, and normal chat views can hide the capsule internals.

---

## Example 9: Live Sidecar Suggestion

Attach supervisor context to an already active async run:

```bash
curl -X POST http://127.0.0.1:9091/v1/runs/run-abc123/actions \
  -H "Content-Type: application/json" \
  -d '{
    "action": "attach_context",
    "reason": "supervisor_suggestion",
    "sidecar_capsules": [
      {
        "provider": "noema",
        "id": "sug_loop_guard",
        "schema": "noema.sidecar.suggestion.v0",
        "visibility": "llm_visible",
        "content": "<noema-suggestion>Stop retrying the same failing validation without changing inputs.</noema-suggestion>"
      }
    ]
  }'
```

Matrix returns a `delivery_id`. Run events show `run.context.attached` and, when delivered, `sidecar.capsule.delivered`.

---

## Next

- [Handoff](Handoff.md) -- the key feature behind multi-agent workflows
- [Workspaces](Workspaces.md) -- how workspace memory and snapshots work
- [API Reference](API-Reference.md) -- build your own integrations
