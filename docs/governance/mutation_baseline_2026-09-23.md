# Mutation baseline: partial re-measurement, 2026-09-23

## Why this is partial

The Tier A mutation campaign (`scripts/quality_gate.sh --mutation`) covers eleven
packages and is deliberately opt-in and slow. It was last measured in full at
v0.1.30 (2,042 mutants killed, zero survivors). It has not been re-measured since,
because the releases that followed ran on a workstation that was also executing
other workstreams, and a campaign run under contention measures the machine rather
than the tests.

Rather than claim a baseline nobody measured or skip the item entirely, the two
packages most relevant to the day's work were measured with reduced parallelism
(`--workers 2 --test-cpu 1`) so a concurrent workstream's tests were not starved:

| Package | Killed | Lived | Not covered | Test efficacy | Mutator coverage |
| --- | --- | --- | --- | --- | --- |
| `internal/logic/vaultsec` | 99 | 0 | 4 | 100.00% | 96.12% |
| `internal/providers/runapi` | 180 | 0 | 44 | 100.00% | 80.36% |

Both clear the gate's thresholds (efficacy 80%, mutator coverage 60%). `vaultsec`
matches its v0.1.30 figure exactly (99 killed), so the vault crypto tests have not
decayed. `runapi` is the package this workstream touched most (the agent-auth
handler); zero survivors means every mutant the tests can reach is killed, and the
44 uncovered mutants are the honest measure of what those tests still do not
exercise.

## How to reproduce

```
export PATH="$HOME/go/bin:$PATH"
gremlins unleash --workers 2 --test-cpu 1 \
  --threshold-efficacy 80 --threshold-mcover 60 ./internal/logic/vaultsec/
gremlins unleash --workers 2 --test-cpu 1 \
  --threshold-efficacy 80 --threshold-mcover 60 ./internal/providers/runapi/
```

## What remains

Nine of the eleven Tier A packages were not re-measured:
`internal/middleware`, `internal/logic/elicitation`, `internal/logic/rundelivery`,
`internal/logic/runconfig`, `internal/logic/runtrace`, `pkg/zedacp`,
`pkg/zedacpstdio`, `internal/providers/telegram`, and the campaign's own defaults for
the remaining members. The full run should happen on a quiet machine; the
`quality-gate` job in CI runs the light gate (coverage ratchet and the plain and race
test suites) but deliberately not the mutation campaign, because it would add a long
serial tail to every push to main.
