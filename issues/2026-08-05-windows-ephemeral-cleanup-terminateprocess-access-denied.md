# Windows: ephemeral cleanup marks a completed run failed when TerminateProcess is denied

Date observed: 2026-08-05

## Environment

- MATRIX `0.1.29`, commit `4ff1f4d`
- Windows workstation
- ACP agent: Codex
- session policy: `new_ephemeral_delete_after_run`
- cleanup policy: `delete_remote_or_cancel_and_forget_local`

## Symptom

An ephemeral ACP run completed its business-side commands and deleted the
remote Codex session, but MATRIX emitted `session.cleanup status=failed` and
then `run.failed` because reconciliation could not terminate a retained Codex
agent client:

```text
failure_code=run_agent_client_reconcile_failed
error=agent_client_reconcile: close retained client codex\0C:\Users\rober\Documents\Half Pocket: exit status 128
TerminateProcess: Accesso negato.
remote_deleted=true
local_forgotten=true
process_absent=true
process_retained=true
cleanup_strength=failed
```

Observed run: `run-5631b897-2a0a-42df-a6a8-ccb086482281`.

The failure is intermittent: immediately preceding ephemeral runs on the same
runtime completed with strong cleanup.

A consecutive caller started at `2026-08-05T09:52:00Z`, but fresh Codex child
activity for that run appeared only at approximately `09:56:15Z` (about 255
seconds later) while the retained client from the failed cleanup was still
present. This makes a small inbox batch look as if email analysis itself took
more than four minutes, although most of that interval precedes provider work.

## Impact

The caller receives a failed MATRIX run even though the governed application
transaction has already completed. Halfdesk Triggerd consequently records a
false MATRIX failure, increments its failure streak and activates backoff.
Retrying can therefore duplicate provider work; application idempotency is the
only protection against duplicate proposals. A subsequent run can also remain
queued for several minutes behind the unreconciled client.

## Expected behavior

MATRIX should reconcile the Windows agent client deterministically. A denied
`TerminateProcess` must be handled with a bounded retry or a verified ownership
strategy, and the final run status must reflect whether the requested strong
cleanup actually succeeded. It must not report a contradictory combination
such as `process_absent=true` and `process_retained=true` without a precise,
actionable distinction between the ACP child, wrapper and cached shared client.

## Suggested regression test

On Windows, run consecutive ephemeral ACP sessions for the same
`agent_id + workspace_path`, including a new run started soon after a prior
cleanup. Assert that:

1. every remote session is deleted;
2. no disallowed Codex/node client survives reconciliation;
3. cleanup never fails with `TerminateProcess: Accesso negato`;
4. a completed agent transaction is not converted into `run.failed` by a race
   against a retained or newly reused client;
5. cleanup metadata cannot mark the same process both absent and retained.
6. a consecutive run begins promptly instead of waiting behind a client whose
   prior ephemeral session was already deleted.
