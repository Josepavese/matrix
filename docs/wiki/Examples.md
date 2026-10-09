# Examples

These examples assume a running daemon, an existing workspace named
`my-project`, an authenticated agent named `opencode`, and an HTTP key loaded as
shown in [Getting Started](Getting-Started.md). Replace IDs with your setup.

## Example 1: First agent conversation

Run `matrix run` in a separate terminal, or use your existing service. In Bash:

```bash
MATRIX_API_KEY="$(matrix config get matrix_api_key)"
curl --fail-with-body -sS http://127.0.0.1:9091/v1/runs \
  -H "X-Matrix-Key: $MATRIX_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "channel_id": "docs.http",
    "agent_id": "opencode",
    "workspace_id": "my-project",
    "input": "Explain the project architecture and cite the files you inspected.",
    "execution_mode": "sync"
  }'
```

Keep the `run_id`; inspect the result and trace before deciding the next task.

## Example 2: Implementation and specialist review

In a configured Telegram conversation:

```text
/use my-project
Add validation to the payments endpoint and report the changes and test results.
/snapshot before-review
/handoff claude
Review the payments changes. Check duplicate-charge handling and cite findings.
/timeline
/decisions
```

`claude` must be installed, enabled and authenticated. Handoff passes a brief,
not the complete source conversation. Snapshot records Matrix state, not source
files. Keep source changes in Git or your project's backup workflow.

When moving this work to HTTP, explicitly attach that channel to the intended
logical session; see [Sessions and Recovery](Sessions-and-Recovery.md).

## Example 3: Scripted delegation

Submit explicit async work and wait for its result or an intervention event.
The complete HTTP request, Unix socket listener, cursor and acknowledgement
examples live in [Delegation and Notifications](Delegation-and-Notifications.md).

A Bash caller with `jq` installed can extract the acceptance ID:

```bash
RUN_ID=$(curl --fail-with-body -sS http://127.0.0.1:9091/v1/runs \
  -H "X-Matrix-Key: $MATRIX_API_KEY" \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: docs-review-001" \
  -d '{"channel_id":"docs.review","agent_id":"opencode","workspace_id":"my-project","input":"Review error handling and report findings.","execution_mode":"async"}' | jq -er '.run_id')

# Linux/macOS: observe terminal outcome; this is not immediate input-request dispatch.
matrix run wait "$RUN_ID" --timeout 10m --json

# All platforms: inspect the recorded trace.
curl --fail-with-body -sS -H "X-Matrix-Key: $MATRIX_API_KEY" \
  "http://127.0.0.1:9091/v1/runs/$RUN_ID/trace"
```

The key identifies one task. Choose another for new work. Windows uses the HTTP
run event API instead of the Unix wait command. Do not retry an interrupted task
blindly: its outcome may be unknown, and project files may already have changed.

## Example 4: Inspect project state without another model call

```bash
matrix workspace show my-project
matrix workspace timeline my-project
matrix workspace memory my-project
matrix workspace snapshots my-project
matrix capacity /absolute/path/to/project
matrix fs list
matrix fs path runs <run-id>
```

`matrix fs path` returns the full `status.json` path. Encoded IDs in semantic
paths are not raw run IDs. Use `matrix fs read <returned-path>` to inspect the
selected state; these views exclude arbitrary task content by default.
Work memory, in contrast, may contain private mirrored turns.

## Example 5: Supervisor context

Attach `sidecar_capsules` separately from the human task body, with declared
visibility, format and correlation IDs. Use the request examples in
[Sidecar Capsules](Sidecar-Capsules.md) and inspect `sidecar.capsule.delivered`
in the trace. Delivery alone does not prove successful use by the model.

For optional native permission settings, prepared Docker isolation, semantic
mounts and collector configuration, see the
[PAL guide](PAL-Execution-and-Observability.md). Driver prerequisites and real
qualification differ across operating systems.

## Next

- [Channels](Channels.md): Telegram setup and common API semantics.
- [Handoff](Handoff.md): HTTP handoff plus the next destination prompt.
- [Sessions and Recovery](Sessions-and-Recovery.md): external session import.
- [FAQ](FAQ.md): provider credits, encryption and troubleshooting.
