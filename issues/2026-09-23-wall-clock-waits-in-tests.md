# Wall-clock waits in tests: the flake surface, and what to do about it

Date observed: 2026-09-23
Status: audited, not rewritten

## Triage release 2026-09-24

Inventario aggiornato: 68 occorrenze di `time.Sleep` nei file `*_test.go`.
Il preflight completo, compresa la suite `go test -race -v ./...`, è passato.
Questa prova non elimina il rischio di flake sotto carico: il refactor dei
wait fissi non è stato fatto e l'issue rimane aperta. La correzione della
race di produzione citata sotto è già chiusa separatamente. Il prossimo
intervento deve partire dai test che fanno un'asserzione immediatamente dopo
una sleep fissa e usare un evento osservabile o una condizione con deadline;
non aumentare indiscriminatamente i timeout per chiudere l'issue.

La CI successiva ha trovato due altri ordini di eventi non deterministici. Il
test A2A di risottoscrizione ora consuma un eventuale aggiornamento `working`
prima dell'evento terminale. Il test `runaction` ora forza il caso in cui il
watcher registra `late` prima del ritorno del provider; il codice conserva
anche la prova successiva del provider sotto lo stesso `delivery_id`. Entrambi
sono fix mirati verificati con race detector, non completano il refactor dei
wait fissi inventariati qui.

## Why this exists

A CI run failed on `TestAttachContextMarksLateWhenProviderDoesNotReturn` at a commit
that changed only a shell script, a CI job and a document. The rerun passed. The
cause was a race in production code (fixed in `df8490e`, issue closed in
`issues/closed/2026-09-23-runaction-late-proof-lost-on-cancellation.md`), but the
reason it *surfaced* as a flake rather than as wrong evidence is that the test
decides by waiting: it polls for an event with a three-second deadline.

The same shape appears throughout the suite. This records what is there and what to
do, so the next person does not re-derive it.

## The inventory

49 `time.Sleep` call sites in test files, in two shapes that fail differently:

1. **Poll with a deadline** (self-correcting, but time-bounded). The test loops
   until an event appears or the deadline expires. This is the better pattern, and
   it still flakes when the deadline is not comfortably longer than the interval the
   production code polls at — the runaction test waited 3s against a 500ms poll and
   lost the race on a loaded runner for a different reason, but the arithmetic is the
   same trap.
2. **Fixed sleep, then assert immediately** (the fragile one). The test sleeps a
   constant and then asserts that something has happened. Under load the sleep
   expires before the work does, and there is no retry: this is where a green suite
   on a workstation and a red one on a runner come from.

The longest waits are worth looking at first, because they are the ones somebody
already found insufficient: `internal/providers/runtimevault/provider_test.go` (two
at 100ms), `internal/providers/agents/acp_client_test.go` (two at 50ms),
`pkg/zedacp/stdio_transport_unix_test.go` (50ms and 20ms), and the
elicitation tests, which account for eleven sites between the package and its
adversarial suite.

## What to do

- Prefer synchronisation to waiting: a channel, a `sync.WaitGroup`, or an
  observable hook the test can wait on. Several packages already expose one (the
  runaction tests were fixed by making the production path record deterministically,
  which is the strongest form).
- Where waiting is unavoidable, keep the poll-with-deadline shape and make the
  deadline a comfortable multiple of the interval being polled — not a round number
  that happens to be three times it.
- Never assert immediately after a fixed sleep. If a test cannot observe the event,
  it is testing the clock.
- When a flake appears, fix the cause rather than raising the timeout: the runaction
  case looked like a timing problem and was a lost-evidence bug.

## Not done here, deliberately

The sites were not rewritten. Turning 49 waits into synchronisation is a real piece
of work with a real risk of churn — a test rewritten without reproducing the flake
is a test whose new failure mode nobody has seen — and it is better done package by
package, starting from the fixed-sleep-then-assert sites, with the flake reproduced
first where possible.
