# Mutation baseline: re-measurement, 2026-09-23

## Result

Every Tier A package was re-measured on the current tree, on a quiet machine, against
the gate's thresholds (test efficacy 80%, mutator coverage 60%):

| Package | Killed | Lived | Not covered | Test efficacy | Mutator coverage |
| --- | --- | --- | --- | --- | --- |
| `internal/logic/runconfig` | 4 | 0 | 0 | 100.00% | 100.00% |
| `internal/logic/rundelivery` | 61 | 0 | 1 | 100.00% | 98.39% |
| `internal/logic/vaultsec` | 99 | 0 | 4 | 100.00% | 96.12% |
| `internal/logic/elicitation` | 15 | 0 | 1 | 100.00% | 93.75% |
| `internal/logic/runtrace` | 179 | 0 | 19 | 100.00% | 90.40% |
| `internal/providers/runapi` | 180 | 0 | 44 | 100.00% | 80.36% |
| `pkg/zedacp` | 265 | 0 | 64 | 100.00% | 80.55% |
| `internal/providers/telegram` | 206 | 0 | 54 | 100.00% | 79.23% |
| `pkg/zedacpstdio` | 38 | 0 | 23 | 100.00% | 62.30% |
| `internal/middleware` | 9 | 0 | 6 | 100.00% | 60.00% |
| **Total** | **1,056** | **0** | **216** | **100.00%** | — |

**No mutant survived anywhere.** Every mutant the tests can reach is killed, which is
the property the gate exists to protect: a survivor would mean a test asserting less
than it appears to. The vault crypto tests, the ACP client and the Telegram provider
carry the load — 265, 206 and 180 killed respectively — and the package this
workstream touched most, `runapi`, has zero survivors.

## Two things worth watching

- `internal/middleware` sits at **exactly** its 60% mutator-coverage threshold, and
  `pkg/zedacpstdio` at 62.30%. They pass with no margin: code added to either without
  tests fails the campaign. That is the ratchet working, and it means the next change
  to those packages should budget for tests rather than discover the failure in a
  campaign run.
- 216 mutants are not covered by any test. The concentration says where the suite is
  thinnest — `pkg/zedacp` (64), `internal/providers/telegram` (54), `runapi` (44) —
  and those are error and edge paths. This is the honest answer to "what do the tests
  still not exercise".

## A discrepancy stated rather than smoothed over

The v0.1.30 baseline recorded 2,042 mutants killed. This run killed 1,056 across the
ten Tier A packages, with 216 more not covered. The two figures are not comparable
without checking what changed, and I did not establish which of the plausible causes
it is: the earlier figure may have covered a different package set, or been taken
with a different gremlins version, since mutant generation changes between versions
and this machine's gremlins was updated in the interim. The number the gate enforces
is the one above, measured on this tree; reconciling it against v0.1.30 would need
that run's per-package output, which is not in the repository.

## How to reproduce

```
export PATH="$HOME/go/bin:$PATH"
# scripts/quality_gate.sh --mutation runs all of these with the gate's thresholds
for pkg in internal/middleware internal/logic/elicitation internal/logic/rundelivery \
           internal/logic/runconfig internal/logic/vaultsec internal/logic/runtrace \
           pkg/zedacp pkg/zedacpstdio internal/providers/runapi internal/providers/telegram; do
  gremlins unleash --workers 3 --test-cpu 1 \
    --threshold-efficacy 80 --threshold-mcover 60 "./$pkg/"
done
```

The campaign stays opt-in and out of CI: it is a long serial tail, and the CI
`quality-gate` job runs the light gate — coverage ratchet plus the plain and race test
suites — on every push to main instead.
