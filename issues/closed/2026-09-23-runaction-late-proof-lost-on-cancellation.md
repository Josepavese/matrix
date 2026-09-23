# Flaky: the terminal-proof watcher can exit without recording that a delivery was late

Date observed: 2026-09-23
Status: fixed

## Symptom

`internal/logic/runaction` failed on CI with:

```
--- FAIL: TestAttachContextMarksLateWhenProviderDoesNotReturn (3.01s)
    event run.context.attached/late not found
```

The same Go code passed CI on the immediately preceding commit (the failing run
added only a shell script, a CI job and a document), so this is not a regression
from that change: it is a race that the loaded runner exposed.

## Why it is not just a short timeout

The test waits 3 seconds (`waitRunActionEvent`, `internal/logic/runaction/*_test.go:272`)
while the watcher polls every 500 ms (`attachTerminalPollInterval`,
`internal/logic/runaction/delivery_proof.go:33`). Six poll cycles should be ample,
so raising the deadline would hide the cause rather than fix it.

## The race, in the code

`watchRunTerminal` selects over three cases
(`internal/logic/runaction/delivery_proof.go:175-192`):

```go
select {
case <-watch.done:
    return
case <-ctx.Done():
    return
case <-ticker.C:
    // run no longer running -> cancel, record "late", return
}
```

The "late" proof is recorded **only** on the ticker branch. If the run's context is
cancelled — which is what completing a run does — and that cancellation is observed
before the next tick, the watcher returns through `ctx.Done()` and the delivery is
left with no terminal proof at all. Whether the proof is written therefore depends
on which branch wins a race, and on a busy machine that race is lost often enough to
fail CI.

Two other paths record `late` (`internal/logic/runaction/service.go:192`, when the
attacher returns after the run stopped running), and neither covers the case in this
test: its attacher blocks until cancellation and returns an error, so the "provider
returned late" path is not reached.

## Impact

A live-context delivery that never returned can end up with no recorded terminal
state, which is exactly the evidence an operator uses to decide whether a run's
context actually arrived. The delivery accounting is therefore not just cosmetically
flaky: the missing event is the thing being accounted for.

## Suggested fix

Record the terminal proof on the cancellation branches too, before returning: when
`ctx.Done()` or `watch.done` fires, check whether the run is still running and, if
not, write the same late state the ticker branch writes. Keep the recording
idempotent (the existing `recordLate(..., false)` already suppresses sidecar
emission) so a tick and a cancellation cannot both produce an event.

A regression test should cancel the run's context immediately after the run
completes and assert the event is still recorded — with the current code, that test
fails deterministically instead of only under load.

## Resolution (2026-09-23)

Fixed as suggested. `watchRunTerminal` now records the terminal proof on every exit
path through `recordLateProofIfRunStopped`, which checks whether the run is still
running before writing, and the ticker branch uses the same helper. The recording is
idempotent by construction: the recorder is wrapped in `sync.Once`
(`internal/logic/runaction/service.go:157-159`), so a tick and a cancellation cannot
both produce an event — which is what made this safe to fix rather than merely
plausible.

`TestLateProofIsRecordedWhenTheWatcherLeavesOnCancellation` calls the exit-path
helper with an already-completed run, so it is deterministic where the original race
was not: it asserts the proof is written exactly once, and that a run still in flight
is not marked. With the helper short-circuited to the old behaviour, the test fails.

The original flaky test is unchanged and still passes.
