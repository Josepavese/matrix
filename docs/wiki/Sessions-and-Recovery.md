# Sessions and Recovery

Matrix preserves a local logical session and its provider's remote identity.
Persistence of a local record alone cannot guarantee that a provider will still
load the conversation.

## Know which ID you are using

| ID | Identifies | Use |
|---|---|---|
| `run_id` | A task execution | Events, actions, trace and notifications |
| Logical session ID | Matrix's local conversation | Channel attachment and session inspection |
| `remote_session_id` | The provider conversation | External import and identity verification |

Use `matrix session inspect <logical-session-id>` to inspect the local mapping.
HTTP `POST /v1/session-actions` provides listing, discovery, import and
capability inspection. Check the provider's advertised lifecycle operations
before depending on resume/load/fork.

## Import an external ACP session

This example uses a registered DeepSeek Harness agent named `dsh`; substitute
your configured agent ID, exact provider session ID and original workspace:

```bash
curl --fail-with-body -sS http://127.0.0.1:9091/v1/session-actions \
  -H "X-Matrix-Key: $MATRIX_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "channel_id": "docs.http",
    "action": "import",
    "agent_id": "dsh",
    "target": "exact-provider-session-id",
    "workspace_path": "/absolute/path/to/original/workspace"
  }'
```

Matrix verifies that exact ID with `session/resume` or `session/load` before
storing a mirror. Import sends no task prompt and creates no replacement
provider conversation. The receipt records verification method, provider `cwd`
when available, and discovery warnings. Matrix cannot inspect the provider's
entire private history; a verified ID is not transcript migration proof.

A declared additional directory outside the workspace needs the same path in
an explicit request allowlist. See the [import contract](API-Reference.md#post-v1session-actions).

## Common restoration failures

| Code | Next step |
|---|---|
| `not_found` | Verify the exact ID and whether the provider still retains the session |
| `resume_unsupported` | Inspect capabilities and adapter support; choose a fresh session explicitly if needed |
| `workspace_mismatch` | Use the original accessible workspace; inspect reported provider `cwd` |
| `provider_auth_required` | Fix the provider account/authentication in its own setup |
| `provider_failure` | Inspect bounded diagnostics and provider state before retrying |

The strict imported-session path retains identity after Matrix restart. It does
not turn a failed restoration into a new conversation. This applies to supported
ACP agents generally, not only the example harness.

## Continue across channels

Bindings are explicit. Attach a new channel to the logical Matrix session:

```bash
matrix session attach docs.http <logical-session-id>
```

Use the same workspace and agent when submitting further work. Preserve a
workstream's existing remote conversation where the provider supports it;
create new conversations for new work deliberately.

## A daemon stopped during a run

On restart, persisted active runs become `outcome_unknown` with stop reason
`daemon_interrupted`. Matrix does not replay their prompts.

1. Inspect the run trace and events.
2. Check the provider conversation and any changed project files.
3. Decide whether to continue the existing session, cancel remaining work or
   submit a new task.

A submission retry with an existing idempotency key returns its original run;
it is not permission to execute the task again. Missing events or an incomplete
window are not proof that nothing happened.

## Timeouts, stalls and cleanup

Hard wall-clock and inactivity timeouts are explicit run settings. A stall view
can show what a run is waiting on; an input request or attention notice need not
be terminal. Follow the [timeout/recovery policy](../matrix_timeout_recovery_policy.md).

Cancellation, remote close/delete, local forgetting and provider-process reap
are different operations. Cleanup responses report proof and retained/shared
owners. Wait for an appropriate cleanup result before starting dependent work;
do not treat an HTTP success alone as proof all provider resources were removed.
See [session cleanup](API-Reference.md#post-v1session-actions).

## Next

- [Delegation and Notifications](Delegation-and-Notifications.md)
- [Handoff](Handoff.md)
- [API Reference](API-Reference.md)
