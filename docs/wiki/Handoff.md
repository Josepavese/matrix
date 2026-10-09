# Handoff

Handoff passes an operational brief from one specialist to another within a
workspace. It preserves useful routing context and source identity.

## How it works

1. Matrix resolves the workspace and source session for the caller's channel.
2. It creates or reuses a destination session for the selected agent/workspace.
3. It stores a pending handoff packet and records the transition.
4. On the next destination turn, Matrix adds the handoff brief to the prompt.

The packet contains source logical/remote IDs, source and target agent IDs,
workspace, mode, creation time and a deterministic summary. The summary uses
session metadata/status/title and an optional operator note. It does **not**
copy the full transcript, every tool result or hidden provider reasoning.
Provide the next specialist with the actual goal, evidence and file references.

## From Telegram

Use configured agent IDs and an existing workspace:

```text
/use billing-api
/handoff claude
Review the changes in src/payments. Check duplicate-charge handling and report findings; do not edit files.
```

The handoff command prepares the destination. The following prompt supplies the
work. A request to avoid edits is not an enforced OS or provider permission policy.

## From HTTP API

Assume `billing-api` exists and `claude` is configured and authenticated.
Retrieve `MATRIX_API_KEY` as in [Getting Started](Getting-Started.md).
Use the same `channel_id` for the source work, handoff and next task:

```bash
curl --fail-with-body -sS http://127.0.0.1:9091/v1/intents \
  -H "X-Matrix-Key: $MATRIX_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "channel_id": "docs.http",
    "intent": "handoff",
    "target": "claude",
    "workspace_id": "billing-api",
    "note": "Review src/payments for duplicate-charge handling."
  }'

curl --fail-with-body -sS http://127.0.0.1:9091/v1/runs \
  -H "X-Matrix-Key: $MATRIX_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "channel_id": "docs.http",
    "agent_id": "claude",
    "workspace_id": "billing-api",
    "input": "Review the payments patch and report findings with file references."
  }'
```

There is no `matrix workspace switch --handoff` flag. Use the intent API or chat
command; CLI workspace switching alone changes a channel binding.

## Tracking handoffs

```text
/timeline
/decisions
```

Or inspect locally:

```bash
matrix workspace timeline billing-api
matrix workspace decisions billing-api
```

A prepared handoff is not proof that the destination completed its task.
Inspect the destination run and its result.

## Handoff vs agent switch

A direct agent selection chooses a target. Handoff additionally records and
schedules a brief from the source session. Provider session restoration,
conversation import and native fork remain separate operations.

## The operator loop

Implement, inspect the result, prepare a specialist brief, request review and
continue later. Metadata snapshots help inspect the transition; they do not
restore repository files.

## Next

- [Using Agents](Using-Agents.md)
- [Sessions and Recovery](Sessions-and-Recovery.md)
- [Sidecar Capsules](Sidecar-Capsules.md)
- [Examples](Examples.md)
