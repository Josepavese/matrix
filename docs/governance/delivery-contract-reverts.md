# Delivery contract — behavioural reverts

Every fix in the delivery contract is claimed by an experiment, not by an
assertion in a report. Each row below is a revert that **compiles**, is applied
to the tree, is run, and fails the named test for the stated reason; the file is
then restored byte-identical. A revert that does not compile is not a proof, and
a revert that reaches no code the test exercises is not one either.

Format: | Test | Reverted change | Observed failure |

## The contract and its vocabulary

| Test | Reverted change | Observed failure |
| --- | --- | --- |
| `TestNotDeclaredIsNotAcceptance` | an undeclared contract evaluates to `accepted` | `acceptance = "accepted", want "not_declared"` |
| `TestCompletedRunWithoutTheDeclaredArtifactIsIncompleteDelivery` | a missing artefact evaluates to `accepted` | the report shows `accepted` where the artefact was never produced |
| `TestPresentArtifactWithTheWrongContentIsNotAccepted` | a digest mismatch is ignored | the artefact with wrong content is reported accepted |
| `TestNoWorkspaceIsUnverifiableNotIncomplete` | no workspace evaluates to `incomplete` | "I could not look" is reported as "it was not delivered" |
| `TestAnUnevaluableContractStopsTheRunAtTheBoundary` | validation at the boundary is removed | an un-evaluable contract is accepted before the run exists |
| `TestAContractIsRefusedBeforeTheRunExists` | an empty contract counts as declared | `a contract is refused before the run exists` fails |
| `TestTheContractIsDecodedFromTheRequestJSON` | the JSON tag is renamed | all three malformed bodies answer `201`, want `400` |

## Artefact containment

| Test | Reverted change | Observed failure |
| --- | --- | --- |
| `TestArtifactPathCannotLeaveTheRunWorkspace` | lexical containment removed | `../escape.md` is accepted as a declared artefact |
| `TestArtifactSymlinkOutOfTheWorkspaceIsRefused` | post-`EvalSymlinks` containment removed | a symlink out of the workspace is accepted |
| `TestAnUnresolvablePathSaysWhyItCouldNotBeResolved` | every path resolution error is swallowed | the detail says "could not be read" where it could not be resolved |
| `TestArtifactWithoutDigestSaysOnlyExistenceWasChecked` | the existence-only note is dropped | an existence check is reported as if the content had been verified |

## The caller's validator

| Test | Reverted change | Observed failure |
| --- | --- | --- |
| `TestContractRejectsAShellString` | a single shell string is accepted | a string with spaces is split and run through a shell |
| `TestValidatorExitCodeDecidesAcceptance` | the validator exit code is ignored | a failing validator leaves the run accepted |
| `TestValidatorThatCannotBeRunIsUnverifiable` | a timeout is swallowed as a pass | a validator that cannot run reports accepted |
| `TestValidatorIsARealProcess` | the validator is answered without running it | the verdict is settled without the process |
| `TestARealNoisyValidatorCannotLeakItsOutput` | output is captured and written into the detail | `MATRIX_SENTINEL_LEAK` appears in the verdict |
| `TestAValidatorCannotSeeTheDaemonsEnvironment` | the child inherits the daemon's environment | `the validator read a daemon secret out of its environment: MATRIX_API_KEY=…` |
| `TestTheValidatorEnvironmentIsAnAllowlistOfNames` | an entry is added to the allowlist | `the validator allowlist is […], want [PATH HOME LANG LC_ALL TZ TMPDIR]` |

## The verdict

| Test | Reverted change | Observed failure |
| --- | --- | --- |
| `TestTheTerminalPathSettlesTheContract` | `terminalResult` stops settling the contract | the terminal path records no verdict at all |
| `TestTheVerdictIsDecidedOnceAndSurvivesTheWorkspaceChanging` | the decided-once guard is removed | `the run recorded 2 delivery verdicts, want exactly one` |
| `TestARunWithoutAContractIsNotReportedAsAccepted` | no contract evaluates to `accepted` | `acceptance = accepted, want "not_declared"` |
| `TestADeclaredContractTheRunNeverEvaluatedIsUnverifiable` | an un-evaluated contract is reported `incomplete` | an accusation stands where the contract was never looked at |
| `TestTheVerdictSurvivesARedactingTracePolicy` | the verdict is written in metadata only | `verified event status = "completed", want "incomplete"` under a redacting policy |
| `TestAVerdictEventWhosePayloadIsGoneStillAnswersFromItsStatus` | the fallback to `event.Status` is removed | `acceptance=unverifiable` where the status alone answers |
| `TestUnverifiableOutranksIncompleteInEitherOrder` | the `unverifiable` guard in `Verdict.add` is removed | the second ordering reports `incomplete` for an artefact it refused to look at |
| `TestSimultaneousTerminalsRecordExactlyOneVerdict` | the per-run lock is removed | 8 simultaneous terminals record more than one verdict (`-race`) |
| `TestACompletedRunWithoutItsArtifactIsReportedAsIncompleteDelivery` | `/explain` stops exposing the verdict | the report shows `completed` with no acceptance status |
| `TestCompletedRunWithoutTheDeclaredArtifactIsIncompleteDelivery` | `verdict.explain()` is removed | `an incomplete delivery must say why it is incomplete` |

## Wiring and the refusal message

| Test | Reverted change | Observed failure |
| --- | --- | --- |
| `TestTheRunRecordsTheContractBeforeItIsDispatched` | the declaration event is appended | the run is dispatched with no contract recorded |
| `TestModelIDConflictNamesTheAgentAndTheRemedy` | the generic 409 text is restored | the answer names neither the agent nor the remedy |
| `TestModelIDConflictKeepsTheRemediesTheRuntimeStatusOffers` | the shared remedy is rewritten on one side only | the two surfaces contradict each other on what the operator should do |

## Workspace evidence: the disagreement a provider creates

EP-03.C (run `54f552c5`) was left open because the provider's logs were said to be
needed. They are needed for the attribution, which stays out of reach; they are not
needed for the disagreement, and the test below is the demonstration: a provider
commits with `git -C <root>` while the run's workspace is a linked worktree of that
same repository, and the artifact plus that repository name the two sides.

| Test | Reverted change | Observed failure |
| --- | --- | --- |
| `TestProviderCommittingOutsideTheResolvedWorkspaceIsNamedByTheArtifact` | `run_workspace_resolution.go:121` stops publishing `workspace_requested` | `the artifact does not publish the requested workspace` — the two sides stop travelling together, so nothing can be compared |
| `TestProviderCommittingOutsideTheResolvedWorkspaceIsNamedByTheArtifact` | `run_workspace_resolution.go:123` stops publishing `workspace_not_derived` | `the artifact does not declare what it did not derive` — an absent repository field reads as an agreement |
| `TestProviderCommittingOutsideTheResolvedWorkspaceIsNamedByTheArtifact` | `run_workspace_resolution.go:122` publishes the requested observation as the resolved one | `the artifact does not say where the run was dispatched: map[workspace_id:ws-worktree]` — the recorded dispatch disappears and the comparison has no left side |

## The release gate

`scripts/release_gate.sh` is the check that used to be a sentence someone
remembered. The rows below are what the script does that remembering it did not.

| Reverted change | Observed difference |
| --- | --- |
| the platform list drops `windows/amd64` | the report keeps no line for the platform whose build is broken: the `release-dry-run` failure of `aeb160b` (`undefined: matrixSurfaceConfig`, `undefined: resolveActiveHome`) becomes invisible again |
| `git archive <rev>` is replaced by the working tree | the working tree builds for `windows/amd64` while the commit exported from `aeb160b` does not: the check reports on the artifact the developer has open instead of the artifact that ships |

## The environment a test is run in

A test that drives git must not read the configuration of the machine running it.
An ambient `commit.gpgsign` or `core.hooksPath` decides whether its commits exist,
and an ambient `GIT_DIR` outranks the `-C` that chooses the repository. The fix is
one implementation, `internal/testgit`, for the five sites that used to build their
own environment; no production binary imports it (`go list -deps ./cmd/matrix`).
The revert below is the whole isolation: `Command` hands the process back the
caller's environment. The last three rows share it, and each fails **only** in the
hostile environment: in an ordinary one the same test passes.

| Test | Reverted change | Observed failure |
| --- | --- | --- |
| `TestCommandCommitsUnderAHostileCallerEnvironment` | `internal/testgit/testgit.go` stops setting the environment (`cmd.Env = nil`) | the command is refused by the caller's environment before it can commit: `git [init -q …/repo]: exit status 128: fatal: Invalid path '/nonexistent': No such file or directory` |
| `TestWorkspaceGrantAPIAndRunPreflight` | the same revert | under `GIT_CONFIG_GLOBAL` with `commit.gpgsign = true`: `git [-C …/repo … commit …]: exit status 128: error: gpg failed to sign the data` |
| `TestGrantCoversOnlyOwnedRepositoryAndSelectedWorktrees` | the same revert | same failure under the same environment |
| `TestElicitationInteropOverRealStdioProcess` | the same revert | under `GIT_DIR` pointing at another repository, `repoRoot` answers with the test's own directory: `build mock agent: exit status 1: stat …/tests/integration/cmd/mock-agent: directory not found` |

## The notification channel: the question, and the record that acknowledges it

Two properties landed here, and each row is a revert that was applied to the tree,
compiled, ran, and failed the named test before the file was restored
byte-identical.

The bounded question: a supervisor that wakes on a question it cannot show has to
read the transcript to find out what the run is blocked on. The question therefore
travels on the notification record, and it is user content, so the trace policy
must be able to take it away again - the same rule that empties an event message
empties it.

The acknowledgement record: the window a legitimate replay needs is set by the
supervisor's own persisted cursor, which the daemon does not hold. No default
window was chosen, because a number chosen here would be a promise Matrix cannot
keep; the growth is declared, and an operator who wants a bound sets one. When a
bound is configured, the answer declares it, so a replay that has fallen outside
the window cannot look like a replay that never happened.

| Test | Reverted change | Observed failure |
| --- | --- | --- |
| `TestNotificationAckRecordsOneClaimOnceUnderConcurrency` | the lock around `recordNotificationAck` is removed | 8 parallel identical claims record `prime=7 replay=1 rifiuti=0`, want `prime=1 replay=7 rifiuti=0` (`-race`); the second phase configures a window, so the same race also covers the housekeeping pass and its rate limit, and records inside the window stay uncollected |
| `TestNotificationAckNeverClaimsWhatItCouldNotRecord` | the error from `storage.Set` is ignored | a failed write answers `200 {"run_id":"run-unwritten","sequence":5,"status":"acked"}`, want 500 and no record |
| `TestTheElicitationQuestionTravelsBoundedAndMarked` | the raw message goes on the record and the bounded form only decides the marker | the record carries the whole question, newlines included, with `CODA-OLTRE-LA-SOGLIA` present, want the bounded single-line prefix |
| `TestTheElicitationQuestionDoesNotSurviveARedactingPolicy` | `Project` stops applying the content rule to the stall view | under `content_mode=refs` and under `content_mode=redacted` the projected trace still carries the question, want it removed |
| `TestBoundElicitationQuestionMarksTheCutAtTheDeclaredThreshold` | the truncation marker is not set | a question cut at the declared bound reads as a whole question, want it marked |
| `TestAConfiguredWindowDeclaresItselfAndTurnsAnExpiredReplayIntoAFirstClaim` | the expiry check is removed (`expired := false`) | an expired record answers as a replay, want a first claim that says the record had expired |
| `TestAConfiguredWindowDeclaresItselfAndTurnsAnExpiredReplayIntoAFirstClaim` | the window declaration is dropped from the answer | the answer does not carry `idempotency_window_seconds`, want the window in force declared |
| `TestAConfiguredWindowEvictsExpiredRecordsInsteadOfOnlyHidingThem` | the sweep is removed | 4 acknowledgement records remain where 2 are inside the window: the window hides records instead of bounding them |
| `TestAnUnusableWindowIsRefusedRatherThanQuietlyIgnored` | an unusable window falls back to no expiry | `0`, `-5` and a non-numeric value all answer `200` and store a record, want a refusal and nothing written |
| `TestAConfiguredWindowDeclaresItselfAndTurnsAnExpiredReplayIntoAFirstClaim` | the JSON-string spelling that `matrix vault set` writes is not read | the documented way to set the window answers `500 ... must be a number of seconds`, so a configured window is an outage of the endpoint |

## What the agent reports print: names and counts, values on request

An agent configuration holds the environment and the endpoint headers, and both
carry values. The reports that print a configuration - `agent show`,
`agent override show`, and `agent env list`, which printed `NAME=Value` verbatim
- now print names and counts, and print the values only when the operator asks
for them with `--reveal-values`. The reason is where this output goes: stdout
ends up in a log, in shell history and on a shared screen, and a value printed
there has left the vault for good. The repository already spoke this way in two
places - `agent doctor` prints `effective_env_count` and `override_env_count`,
and `agent set-endpoint` prints `header_names` - so this is that rule applied to
the rest, not an exception carved out for two commands. The values were never
hidden from the operator: they are one flag away, and the report still says which
names it is not showing.

| Test | Reverted change | Observed failure |
| --- | --- | --- |
| `TestNoAgentReportPrintsACredentialUnlessItWasAskedFor` | `addAgentConfigReport` puts the raw configuration, override and environment in the report | `agent show stampa la credenziale senza che nessuno l'abbia chiesta`, with `SENTINEL-CREDENTIAL-5c1e07` in the printed JSON |
| `TestNoAgentReportPrintsACredentialUnlessItWasAskedFor` | `agentOverrideReport` puts the raw override in the report | `agent override show stampa la credenziale senza che nessuno l'abbia chiesta` |
| `TestNoAgentReportPrintsACredentialUnlessItWasAskedFor` | `agentEnvLines` returns the entries instead of the names | `agent env list stampa la credenziale senza che nessuno l'abbia chiesta` |
| `TestTheDisplayOfAConfigurationNamesAndCountsAndCarriesNoValue` | `DisplayConfig` hides the values only when the caller asks to (the flag read backwards) | `il report di default porta un valore`, with the sentinel inside the default report |
