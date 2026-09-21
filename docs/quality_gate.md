# Quality Gate

The repository has one executable gate: `scripts/quality_gate.sh`. It exists
because coverage alone proved insufficient — twice during the adversarial pass a
test passed **with the fix removed**, so a green suite was not evidence of
anything. The gate therefore checks behaviour (tests, fuzzing) *and* test
strength (mutation testing), not just line counts.

## Running it

```bash
scripts/quality_gate.sh              # format, vet, build, race tests, coverage ratchet, governance
scripts/quality_gate.sh --light      # resource-constrained host: no repository-wide race run
scripts/quality_gate.sh --static     # + golangci-lint and govulncheck when installed
scripts/quality_gate.sh --fuzz       # + a short campaign for every fuzz target
scripts/quality_gate.sh --mutation   # + mutation testing on Tier A (slow)
scripts/quality_gate.sh --all        # everything
```

### On a resource-constrained workstation

A repository-wide `go test -race ./...` needs several GB of RAM and a lot of CPU,
and a mutation run multiplies that by the number of mutants. On a small machine:

- use `--light`: the race detector is then applied only to the packages that
  carry concurrency, and the rest of the suite runs without it;
- run `--mutation` one package at a time (`gremlins unleash ./internal/logic/session/`)
  and never in parallel with `--fuzz`, which is CPU-saturating by design;
- keep an eye on free space: the Go build cache plus a fuzz corpus plus
  gremlins' working copies can consume gigabytes. `go clean -cache` and clearing
  `$TMPDIR` between campaigns is part of running the gate, not an afterthought.

A disk that fills during a mutation run fails the run with `ENOSPC` and leaves no
summary line; treat that as an incomplete measurement, never as a pass.

`FUZZ_TIME` overrides the per-target fuzz duration (default `20s`), e.g.
`FUZZ_TIME=5m scripts/quality_gate.sh --fuzz`.

Exit status is `0` only when every selected check passes. The last line is
`QUALITY_GATE_OK` or `QUALITY_GATE_FAILED`, which is what a CI job should assert.

## Tiers

Not every statement is worth the same, so the repository is graded in tiers.

| Tier | Packages | Bar |
|---|---|---|
| A — protocol, SSOT, security, channel frontends | `middleware`, `logic/elicitation`, `providers/agents`, `providers/runapi`, `providers/telegram`, `logic/session`, `logic/runtrace`, `logic/rundelivery`, `logic/vaultsec`, `logic/workspace`, `pkg/zedacp`, `pkg/zedacpstdio` | 100% is the goal; the coverage ratchet prevents regression, mutation efficacy ≥ 80%, a fuzz target per decoder |
| B — logic | remaining `internal/logic` packages | ≥ 85% with error paths covered |
| C — wiring and CLI | `cmd/*`, `bootstrap`, `channelcfg` | smoke tests; no percentage target |
| D — OS-specific and external | build-tagged files, provider transports | contract tests; excluded from the ratchet |

## Coverage ratchet

The floors in `scripts/quality_gate.sh` are the measured values at the time the
gate was introduced. A floor may be raised freely. Lowering one is a deliberate
decision that must be explained in the change, never a way to make a change pass.

## Mutation testing

Mutation testing is the control that catches tests without teeth: it edits the
production code (flip a comparison, invert a condition, change a constant) and
requires the suite to fail. A **survivor** means an assertion is weaker than it
looks.

```bash
gremlins unleash ./internal/providers/telegram/
gremlins unleash --workers 5 ./internal/logic/session/
```

Measured baseline, all with the aggressive mutant types enabled:
**2,042 mutants killed, zero survivors** across eleven Tier A packages.

| Package | Killed | Lived | Mutator coverage |
|---|---|---|---|
| `logic/session` | 705 | 0 | 81% |
| `providers/agents` | 542 | 0 | 82% |
| `pkg/zedacp` | 234 | 0 | 78% |
| `logic/runtrace` | 178 | 0 | 90% |
| `providers/runapi` | 154 | 0 | 72% |
| `logic/vaultsec` | 99 | 0 | 96% |
| `logic/rundelivery` | 62 | 0 | 95% |
| `pkg/zedacpstdio` | 38 | 0 | 62% |
| `logic/elicitation` | 15 | 0 | 94% |
| `middleware` | 11 | 0 | 65% |
| `logic/runconfig` | 4 | 0 | 100% |

Zero survivors means no assertion in the covered code is weaker than it looks.
Mutator coverage is the number still worth raising: it is the share of the code
the tests can reach at all, and the low values (`zedacpstdio` 62%, `middleware`
65%, `runapi` 72%) mark the next places to write tests, with better aim than line
coverage gives.

Configuration lives in `.gremlins.yaml`: the aggressive mutant types
(`invert-logical`, `invert-loopctrl`, `invert-bitwise`, `invert-assignments`) are
enabled on purpose, and the efficacy threshold is 80%.

Two numbers come out of a run and they mean different things:

- **test efficacy** — of the mutants the tests can reach, how many they kill.
  This is the test-quality number.
- **mutator coverage** — how much of the code is reachable by the tests at all.
  This is a coverage number with better aim than line coverage, and it is the
  best guide for where to write the next test.

## Fuzzing

Every decoder that consumes external bytes has a fuzz target: the ACP envelope
and payload decoders, the elicitation schema projection, the shared answer
validator, the Telegram callback decoder, the stdio frame reader, and the HTTP
answer endpoint. The invariant asserted in a target is a safety property that
must hold for arbitrary input (never panic, never lose meaning, never accept a
value the SSOT validator refuses), not a restatement of the implementation.

Campaigns seed from the behaviour of real peers; a crasher is written to
`testdata/fuzz/<Target>/` and then becomes a permanent regression case, which is
why the target's invariant must be *correct* before the corpus is committed.

## Defects the controls found

Recorded so the rules are not abstract. Every one of these was found by a
control introduced in this pass, not by review:

| Control | Defect |
|---|---|
| Fuzzing (`FuzzWireToNeutralElicitation`) | A property named `""` produced a field no frontend could label and no agent could key. Degenerate schemas are now rejected. |
| Fuzzing (`FuzzWireToNeutralElicitation`) | URL mode accepted any scheme with a host (`A://0`, `ftp://`, `smb://`), so an agent could steer a browser or a protocol handler off the web. Now http/https only. |
| Fuzzing (`FuzzDecodeCallbackData`) | An unknown or empty action fell through to the option branch and silently selected option 0. Only an explicit `pick` may set an answer now. |
| Fuzzing (`FuzzDecodeCallbackData`) | A whitespace-only token was accepted; blank identifiers can never match a pending request. |
| Fuzzing (`FuzzDecodeInboundRaw`) | The decoder that exists to handle malformed input dereferenced a nil logger and panicked. |
| Reverting a fix to check the test fails | A regression test passed with the fix removed because it used an invented token, so the code path under test was never reached. |
| Reverting a fix to check the test fails | A cyclic-fork test also passed without its guard, because the depth backstop masked it; the assertion now measures recursion depth. |
| Mutation testing | No survivors, which is the point: a survivor would mean an assertion weaker than it looks. |

The pattern is consistent: the defects that reached users were in the parts of
the system nobody had pointed a hostile input at, and the tests that needed
repair were the ones that had never been proven to fail.

## Controls that are not thresholds

- `gofmt` — formatting is not negotiable.
- `go vet` — including the checks the race detector does not perform.
- `golangci-lint` — when installed; the tool must be rebuilt with the toolchain
  on `PATH`, otherwise it fails to read export data and the run is meaningless.
- `govulncheck` — the last run reported no vulnerabilities affecting called code.
- `go run ./scripts/governance_check --manifest governance/manifest.toml` — the
  repository's own layering and budget rules.
