# Delegation and Notifications

A supervisor should retain a task ID and its outcome, without repeatedly
reading every agent token. Matrix provides async runs, event cursors and
intervention surfaces. The supervisor decides when to wake or invoke its own model.

![Submit a task, let the agent work, receive an outcome or intervention event, then inspect and retain the cursor.](../assets/readme/delegation.svg)

## Submit explicit work

Assume `my-project` exists and `opencode` is ready. Use the API key loaded in
[Getting Started](Getting-Started.md#4-verify-authenticated-access):

```bash
curl --fail-with-body -sS http://127.0.0.1:9091/v1/runs \
  -H "X-Matrix-Key: $MATRIX_API_KEY" \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: supervisor-project-review-001" \
  -d '{
    "channel_id": "supervisor.review",
    "agent_id": "opencode",
    "workspace_id": "my-project",
    "execution_mode": "async",
    "input": "Review the error handling. Report findings with file references."
  }'
```

Retain the returned `run_id`. Reuse the same submission key only for retries of
this exact task under this channel. A different payload with the same key is
refused; a replay returns the existing run. Use a new key for genuinely new work.

From v0.1.54, the CLI can submit the same asynchronous work:

```bash
matrix run submit --agent opencode --workspace my-project \
  --channel supervisor.review --prompt "Review error handling and report findings." \
  --idempotency-key supervisor-project-review-001 --json
```

The default channel is `cli.run.submit`. Choose a separate `--channel` for
caller bindings and idempotency scopes, and name the existing workspace explicitly. No project
is inferred from the invoking shell's directory. Without `--workspace`, the
runtime uses its established channel/session binding. A different channel does
not force a new provider conversation; workspace affinity may reuse a session.
Select/create sessions explicitly for conversation isolation. Acceptance does not mean
completion; keep the returned `run_id` and consume its outcomes.
The v0.1.53 payload defect is recorded in the
[closed correction](../../issues/closed/run-submit-missing-channel.md).

## Linux and macOS: private outcome channel

```bash
matrix run wait <run-id> --timeout 10m --json
matrix run wait <run-id> --after <saved-cursor> --on-attention --json
```

Wait/ack are Unix-only. `run wait` consumes the authenticated private socket
under `MATRIX_HOME/data/run-notifications.sock`. It returns terminal outcomes
(`completed`, `failed`, `cancelled`, `outcome_unknown`), or a nonterminal attention
notice when `--on-attention` is set. A timeout is not task completion; retain the
printed cursor and continue with `--after`.

Input-request wakeups are `elicitation.opened`. Interactive wait displays the
bounded question but continues waiting; JSON wait accumulates input summaries
and prints them when it returns. A supervisor needing **immediate** intervention
should consume the socket SSE stream directly:

```bash
MATRIX_HOME="$(matrix home)"
curl -N --unix-socket "$MATRIX_HOME/data/run-notifications.sock" \
  -H "X-Matrix-Key: $MATRIX_API_KEY" \
  'http://localhost/v1/run-notifications?stream=sse&after=0'
```

Filter by `run_id` as described in the [API reference](API-Reference.md#local-supervisor-notifications).
Wakeups contain identity and outcome, not transcripts or reasoning. An open
input request is the exception: it carries a question limited to 200 runes,
with a truncation flag when necessary.

## Delivery and acknowledgement

Notifications are durable and delivered at least once. Persist your consumer
cursor, deduplicate by sequence, and inspect state after reconnect. Matrix
repairs missing terminal notifications on startup without renumbering them.

```bash
matrix run ack --run-id <run-id> --sequence <sequence> \
  --idempotency-key supervisor-review-delivery-001 --json
```

Acknowledgement records the supervisor's delivery claim. It does not verify that
some downstream application actually handled the message. Exactly-once replay
applies to the acknowledgement key, not to all notification delivery. Optional
acknowledgement retention changes that replay window; the
[API contract](API-Reference.md#post-v1run-notificationsack) states the details.

## All platforms: run events and intervention

Windows has no Unix notification socket or CLI wait/ack. Use the authenticated
run event API on every platform:

```bash
curl --fail-with-body -sS -H "X-Matrix-Key: $MATRIX_API_KEY" \
  'http://127.0.0.1:9091/v1/runs/<run-id>/events?after=0'

curl -N -H "X-Matrix-Key: $MATRIX_API_KEY" \
  'http://127.0.0.1:9091/v1/runs/<run-id>/events?stream=sse&after=0'
```

Persist the event page's `next_cursor`. Run-event cursors and local-notification
sequences are different streams; do not interchange them. Events can contain
agent content, so select the event fields you need before sending anything to a
supervisor model.

Open input requests are available through `GET /v1/elicitations`; replies use
`POST /v1/elicitations`. Active-run actions include cancellation and supported
sidecar attachment. See [run actions](API-Reference.md#post-v1runsrun_idactions)
and [elicitations](API-Reference.md#elicitations) for payloads and provider limits.

## Recovery is a decision

A daemon restart makes previously active runs `outcome_unknown` with
`daemon_interrupted`. Do not equate missing output with failure or automatically
repeat a prompt that may already have changed files. Inspect the provider session
and project state first. See [Sessions and Recovery](Sessions-and-Recovery.md).
