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

## Resolution (2026-09-23, Matrix workstream)

Both claims in this report were re-verified against the code before anything was
changed, and both held: a denied `TerminateProcess` became
`session.cleanup status=failed` plus `run.failed` after the governed transaction
had already completed, and `process_absent=true` could appear together with
`process_retained=true` in a structure that had no field naming *which* process
survived.

### The root cause, precisely

`ReconcileAgentClients` evicts the cache entry **before** closing it
(`internal/providers/agents/router_reconcile.go:26-31`). The client is therefore
already "absent" when the OS refuses to terminate the process, and it is
simultaneously alive: the contradiction was a faithful report of a genuinely
confusing state, not a formatting bug. The only identity left was inside an error
string (`close retained client codex\0<path>`) that the caller discarded
(`internal/logic/session/manager_capabilities.go:50-53`).

### What changed

- **Bounded retry.** `internal/logic/sessioncleanup/termination.go` recognises a
  termination denial (including the Italian `Accesso negato` seen in the report)
  and retries up to three times with backoff, context-aware. It deliberately does
  not match a bare `exit status 128`: that is a shell exit code, not a denial.
- **Named retention.** A new evidence field `process_retention_scope` carries
  `run_scoped_agent_child`, `agent_wrapper` or `cached_shared_agent_client`, so
  `process_retained=true` is now actionable instead of ambiguous.
- **Run outcome.** A retained **cached shared client** no longer fails a completed
  run: the cleanup is reported as degraded (`cleanup_strength=retained`,
  `weak_cleanup_reason=process_retained`, no failure code) with the run preserved.
  A retained **run-scoped child** or **wrapper** still fails the run, and a denial
  that cannot be attributed fails closed rather than being downgraded.
- **Retries are classification-aware, deliberately.** Because the cache entry is
  evicted before the close, a blind retry can report success while the process is
  still alive. Only denials attributable to a cached shared client are retried; a
  denial on the run's own child is reported immediately. This narrows the
  requested "bounded retry" in favour of not producing a false success.

### Evidence

Six new tests in `internal/logic/sessioncleanup/` and six in
`internal/logic/runreconcile/`. Against the restored original logic they fail with
exactly the reported contract violations, e.g. *"a transient denial recovered by
retry must not fail the run"* and *"a retained cached shared client must not fail a
completed run"*. At the HTTP boundary,
`internal/providers/runapi/runs_test.go` now pins both halves: a retained shared
client returns 201 with degraded evidence, and a retained run-scoped child returns
500 with the child named.

### Residual work, deliberately left open

1. **The retry sits one level above the true ownership boundary.** It lives in
   `runreconcile`; the place that owns the process handle and could genuinely
   re-attempt the same termination is `closeReconciledClients` in
   `internal/providers/agents/router_reconcile.go`, which this change did not
   touch. A follow-up should move the retry there and stop evicting before closing.
2. **Not reproduced on Windows.** The verification is at the injected-provider
   level; a real `TerminateProcess` denial was not observed on the Windows guest.
3. The report also noted a caller starting a consecutive run while a retained client
   was still present. That timing effect is not addressed here; the run outcome and
   the evidence are.
