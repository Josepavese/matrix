# Phase 3 audit — agnosticism, threat model, quality

Auditor: the independent verifier. Scope: everything from `c8d6aa3` to the
frozen HEAD `45130f3`, plus the working tree as declared frozen by the Lead.
Method: every claim below is either **covered by an experiment** (a revert that
compiles, or a probe that measures the result) or **labelled a hypothesis**. No
claim rests on a report, a commit message or this document's own prose.

## 0. The freeze, and one breach of it

HEAD at the start of the audit: `45130f3` (`fix: a validator does not inherit
the daemon's keys`), tree clean apart from an untracked `issues/` directory that
belongs to no writer here. The four pattern budgets and the quality checker were
run against it.

**During the audit the tree changed without notice**: `grants.go` +4,
`internal/logic/workspacegrant/git_env.go` new (35 lines),
`git_env_test.go` new (102). The delta contains **no agent or provider name and
no identity comparison**, and none of the five capped packages changed
(`cmd/matrix` still 4627, `runapi` still 2568), so the audit below stands for
the frozen scope. The breach is recorded because a measurement is only a
measurement of the tree it was taken on, and the freeze is what makes that
tree knowable. The Lead's formulation, recorded verbatim: *"una misura è la
misura dell'albero su cui è presa"*.

Every number in this document is a **frozen-tree number** (`45130f3`). Work
assigned after the audit — the `cmd/matrix` split, the run-body limit — moved the
tree; where a section below says "measured", it means measured there, and the
sections that report the post-audit state say so explicitly.

## 1. Agnosticism

**Measured on the frozen tree** (checker run with every `max` lowered to 0 in a
copy of the manifest, so each count is printed):

| Budget | Measured | Cap | Verdict |
| --- | --- | --- | --- |
| `agent_name_literals_in_logic` | 14 | 14 | green, all 14 sites pre-declared |
| `adhoc_agent_name_literals` | 0 | 0 | green |
| `agent_identity_branch_shape` | 0 | 0 | green |
| `identity_comparison_shape` (AST) | 0 — 11 comparisons, all reviewed | 0 | green: literal **and constant** forms |

`governance_check` → `failures: 0` / `GOVERNANCE_CHECK_OK`.

**The 14 literals are the declared ones, and the diff added none.** Sites:
`cmd/matrix/constants.go:9`, `internal/providers/runapi/types.go:123`,
`internal/providers/a2a/server.go:35`, `internal/providers/matrixapi/server.go:38`
(all four a *default agent* value), `internal/providers/agents/router_observer_content.go:148`
(reads the provider's own `meta["codex"]` block), `internal/logic/agentidentity/codex.go:10`
(the canonical ID declaration), `internal/logic/onboarding/auth_handler.go:73-74`
(the auth-handler registry, keys literal hence visible), `internal/logic/onboarding/wizard_steps.go:11-12`
(the two name constants), `internal/logic/session/manager.go:97-98` (defaults).
None of the files added by this diff contains an agent or provider name.

**Post-audit extension, requested by the Lead: the declared blind spot, closed.** Two probes were
placed in `internal/logic/agentidentity/`, first on the frozen tree and then
against the extended check:

| Probe | Before the extension | After the extension |
| --- | --- | --- |
| `if agentID == "codex"` (literal) | `failures: 3` — `agent_identity_branch_shape`, `agent_name_literals_in_logic` (15 > 14), `identity_comparison_shape` | `failures: 3`, unchanged |
| `if agentID == CanonicalCodexAgentID` (constant) | **`failures: 0` / `GOVERNANCE_CHECK_OK`** | `failures: 1` — `identity_comparison_shape` |

The rule added to `scripts/governance_check` (budget `identity_comparison_shape`,
new key `agent_name_values`):

- the compared constant is **read from the tree** — same package, or qualified and
  resolved through the file's imports and `go.mod`; package-level `const` only;
- the finding is decided by the **shape** (an identity role compared against a
  package string constant), exactly as the literal rule is decided by its shape.
  Gating it on the constant's *value* was measured and **rejected**: that variant
  costs no review on this tree, but a brand-new agent name behind a brand-new
  constant passed it (`failures: 0`). That probe became a test:
  `TestIdentityComparisonFiresOnANewAgentNameBehindANewConstant`;
- `agent_name_values` labels the finding instead — `which names an agent` vs
  `whose value names no agent in the vocabulary` — matched as a **whole word**, so
  `"codex-acp"` names the codex agent while `"mimosa"` does not name MiMo;
- the reviewed pair is keyed by the constant's **name** (`name=const:ConstName`),
  so a cleared literal can never clear a constant, and renaming the constant
  re-opens the review.

Measured on the tree as it stands (HEAD is still `45130f3`, the freeze, with the
post-audit work uncommitted), the extended rule sees **11 comparisons, all
pre-existing**: the eight files that carry them are untouched in
`c8d6aa3..HEAD` and unmodified in the working tree, so none arrives with this
work. 10 name an agent and 1 a protocol constant. The hand enumeration in the
first draft of this audit had found **8**; the tool found two more that a
name-based grep had missed — `internal/logic/agentidentity/codex.go:26` and
`internal/logic/agentmgr/registry_client.go:283`, both on `CodexRegistryID`. The mechanical set:

| Site | What it decides | Judgement |
| --- | --- | --- |
| `internal/logic/agentidentity/codex.go:17` | maps the canonical Matrix agent ID to the ACP registry ID | legitimate: this package *is* the declaration of the vocabulary |
| `internal/logic/agentidentity/codex.go:26` | refuses the ACP registry id as a public Matrix id | legitimate: the same declaration, on the boundary |
| `internal/logic/agentmgr/registry_client.go:283` | the registry lookup refuses the registry id, then resolves the alias | legitimate: a negative test of the registry identifier |
| `internal/logic/onboarding/wizard_steps.go:109`, `internal/logic/onboarding/wizard_agent_selection.go:40` | the codex install path in the wizard | per-agent configuration flow, operator selected that agent |
| `internal/logic/onboarding/wizard_steps.go:190`, `:194` | the OpenRouter flow for opencode | per-agent configuration flow |
| `internal/logic/onboarding/wizard_auth_flow.go:9`, `:61` | the `quick_login` guard for opencode | verified in T13 as a real credential-misrouting guard |
| `internal/logic/onboarding/wizard_auth_flow.go:12` | the `chatgpt` device flow for codex | flow-correctness guard (T13) |
| `internal/providers/agents/acp_model_selection.go:148` | finds the ACP `model` config option | ACP protocol vocabulary, not an identity — the one review the shape rule costs |

All 11 are in `reviewed_pairs` (7 const pairs + 5 literal pairs), so the budget
stays at zero and the next such branch fails. The three reverts that give the
extension its teeth are in §4.

**What remains invisible** — the declared limit, now narrower: a comparison whose
left side is a call result or a dereference carries no name, and a package-level
`var` is not read, because it can be reassigned and its value at the comparison
would not be a property of the tree. One nuance of the same kind: a local
variable **shadowing** a package constant is resolved to the constant's value, so
the branch is still flagged but the label may name the wrong value — the shape,
which is what decides, is the same either way. The non-literal comparisons enumerated
mechanically (8 sites, of which 2 were scan false positives — a uid comparison, a
tool-name comparison) are unaffected: the remaining 6 —
`runapi/session_cleanup_related.go:197`, `agents/router_client_keys.go:60`,
`session/manager_fork_cleanup_proof.go:133`, `:256`, `:257` — compare two
**runtime identities** for scoping (this session belongs to that agent), which is
the agnostic counterpart of a branch on a name: no site decides behaviour *from*
which agent it is. Two comparisons on classifier constants
(`sessioncleanup/cleanup.go:216`, `:231`, `authType` in the mock agent) are
classifiers, not identities, and their roles keep them out of the shape rule.

## 2. Threat model on the new surfaces

### 2.1 The caller's validator (argv executed by Matrix)

| Property | How it was verified | Result |
| --- | --- | --- |
| argv, never a shell string | revert of the refusal guard | red: `contract_test.go:287` |
| cwd is the run workspace | test plus revert of the seam | red |
| timeout bounded | code: default 30s, cap 5m, negative refused | ok |
| output never reaches the verdict | revert: capturing output into the error | red (`acceptance = "unverifiable", want "incomplete"`) |
| the daemon's environment is not inherited | revert: `cmd.Env = validatorEnv()` removed | red: `the validator read a daemon secret out of its environment: MATRIX_API_KEY=sk-live-second-sentinel` |
| the allowlist is a decision, not a habit | revert: `EDITOR` added to a *neutral* allowlist | red on the pinned list only, proving the pin's distinct value |

**Residual limit, measured and confirmed as a limit**: the allowlist closes the
*environment* channel, not the filesystem. A validator reads and writes outside
the run workspace (probe: `sh -c 'cat <outside>/outside.txt > read.txt; echo
written > <outside>/written.txt'` succeeded). **Containment is the daemon's OS
user, not the allowlist**: the validator runs with the daemon's own filesystem
rights, and the allowlist only decides which variables it inherits. The Lead
confirmed this is to be recorded as a **limit, not a defence**, and that a
sandbox around the validator would be another trade rather than a fix. Recorded
as a limit: the feature is "the caller's own check", and a caller who can create
a run can already run agent code.

### 2.2 The acknowledgement endpoint

| Property | Evidence |
| --- | --- |
| authenticated | `requireAPIKey` is the handler's first statement (`notification_ack.go:62`); the route is registered on the authenticated matrix HTTP mux (`matrixapi/server.go:78`) |
| a claim without a key is refused | 400 before any store is touched (`matrixapi/server_test.go:1264`) |
| key length bounded | `notificationAckKeyMaxBytes = 128`, refused above it |
| body bounded | `MaxBytesReader(..., notificationAckBodyMaxBytes)` (`notification_ack.go:109`) |
| the client key is not a storage key | `notificationAckStorageKey` hashes it (`notification_ack.go:189-191`), so an opaque client value cannot shape the keyspace |
| the same key with a different claim conflicts | `errNotificationAckConflict` |

**Hypothesis (not measured)**: a distinct key per request writes a distinct
durable record with no TTL and no ceiling, so an authenticated caller can grow
that store without bound. Nothing in the code contradicts it; measuring it would
need a quota or a store-size probe, which the audit did not build.

### 2.3 Endpoint discovery and log redaction

`runtimeSurface` serializes `Kind, Transport, Address, Source, Auth, Exposure,
TokenFile, Warning`; the configured credential is held in unexported fields
(`authKey`, `authKeyName`) and cannot be serialized. The report is an answer to
an operator's explicit question, which is exactly the case the redaction policy
excludes; the policy governs records nobody asked for. Only one log site names
an endpoint and it redacts it (`agents/router.go:286`, `logredact.Endpoint`);
the one other site that mentions a protocol writes `Kind` and `Transport`, not a
location (`agentmgr/supervisor.go:123`). Removing the redaction reddens two
tests (`Endpoint("api.deepseek.com") = "api.deepseek.com", want "***"`), so the
policy has a tooth and the default is the protective one
(`MATRIX_LOG_REVEAL_ENDPOINTS` is the single opt-in).

### 2.4 `MATRIX_AGENT_PROCESS_CWD`

Not a daemon environment variable: it is read from the **endpoint declaration**
(`agentlaunch/process_cwd.go:25`, `DeclaredProcessCwd`), matched by exact key.
`ResolveProcessCwd` refuses anything that is not an absolute path to an existing
directory, resolves symlinks, and fails closed with a message naming the
variable. It deliberately does not confine to the workspace, and the package
documents why (a process cwd is where the OS starts the child; a session cwd is
a statement to the peer). No caller-reachable path was found that sets an
endpoint's `Env`; the value comes from operator configuration.

### 2.5 Finding: the run submission body is unbounded

`decodeRunRequest` decodes `r.Body` with no `MaxBytesReader`
(`internal/providers/runapi/run_request.go:17-19`) and the HTTP server sets only
`ReadHeaderTimeout` (`cmd/matrix/run.go:221`). The two endpoints added by this
work *do* bound their bodies, which makes the run path the outlier. A
`delivery_contract` can also declare an unbounded number of artifacts, each
stat'ed and, with a digest, hashed at terminal time; `Contract.Validate()`
(`contract.go:111`) bounds shapes but not counts. Severity: low-to-moderate,
authenticated caller, **pre-existing** (the body was unbounded before this
diff); the diff makes the terminal-time work caller-scalable, so it is recorded
here rather than left implicit. **Decision recorded**: the Lead assigned the
closure to `workspace-identity` — a `MaxBytesReader` on the run body plus a cap on
the declared artifact count (`Contract.Validate()` stays a shape check; the count
belongs with the request limit). This audit does not verify that fix: it is
assigned, not landed, and the measurement above is the frozen tree's.

### 2.6 Child identity probe — hypothesis

`readChildIdentity(pid)` reads `/proc/<pid>/cwd` and `/proc/<pid>/cmdline`
(`cmd/matrix/agent_child_identity_linux.go:12-26`). Between a child's exit and
the read, the pid can be reused and the report would describe a different
process. Diagnostic surface only (a CLI report), and a read failure is reported
rather than smoothed over. **Hypothesis**: the window exists; no exploit was
built and none is claimed.

## 3. Quality budgets

Measured with the release checker on the frozen tree, then tightened — a
ceiling with slack is where the next line hides. Rule applied: ceilings are met
by splitting, never raised.

| Package | Measured | Was | Now | Verdict |
| --- | --- | --- | --- | --- |
| `internal/providers/runapi` | 2568 | 2600 | **2568** | tightened |
| `internal/logic/runtrace` | 1676 | 1680 | **1676** | tightened |
| `internal/logic/workspace` | 1099 | 1140 | **1099** | tightened |
| `internal/logic/onboarding` | 1435 | 1435 | 1435 | already the measurement |
| `cmd/matrix` | **4627** | 4400 | 4400 | **hard failure: over by 227** |

The tightening has a tooth: adding two production lines to `runapi` fails with
`runapi has 2570 LOC (budget 2568)`. `Quality Warnings: none` (file 280,
function 55, params 4, branch 10, averages) at these measurements.

`cmd/matrix` is the one open quality failure and it is **not** closable by
raising the ceiling: 227 production lines must move out of the package.
Measured `c8d6aa3..HEAD`, the client surface grew by the child-identity probe,
the endpoint discovery and the run-notification primitives; the split is a
writer's task, and until it lands the release checker exits non-zero.

**Post-audit re-measurement** (the tree moved; these are current numbers, taken
with the ceilings zeroed in a copy of the config — not frozen-tree numbers):

| Package | Measured now | Cap in the file now | Slack |
| --- | --- | --- | --- |
| `internal/logic/onboarding` | 1435 | 1435 | 0 |
| `internal/logic/runtrace` | 1676 | 1676 | 0 |
| `internal/logic/workspace` | 1099 | 1099 | 0 |
| `cmd/matrix` | **4270** | 4400 | 130 — the split landed and the ceiling was **not** raised; the file's own comment declares 4400 provisional until the release audit tightens it |
| `internal/providers/runapi` | **2538** | **2580** | **42 — the only slack**, and it appeared after this audit tightened the cap to 2568, which was exact at its own measurement. `2580` sits above the current measurement, so the next 42 lines hide there. Whoever lands the run-body fix in `runapi` should set the cap to the measurement *after* that change |

`code-governance.toml` is the Lead's to commit: this audit reports the
measurement and does not move a cap while the run-body work is in flight in that
same package.

## 4. Reverts run in this verification (mandatory table)

Format: `| Test | Reverted change | Observed failure |`. Every revert below
compiled, was applied to the tree, was run, and the file was restored
hash-verified. Rows marked *survived* are the ones that found a hole: they ran
**green**, which is why the test named in the next column was written
afterwards.

| Test | Reverted change | Observed failure |
| --- | --- | --- |
| `TestUnverifiableOutranksIncompleteInEitherOrder` | the `unverifiable` guard in `Verdict.add` | `acceptance = "incomplete", want "unverifiable"` (an unevaluable contract reported as a delivery judgement) |
| `TestAnUnresolvablePathSaysWhyItCouldNotBeResolved` | every path resolution error is swallowed | the detail says "could not be read" where it could not be resolved |
| `TestARealNoisyValidatorCannotLeakItsOutput` | output captured and returned in the error | `acceptance = "unverifiable", want "incomplete"` |
| `TestAValidatorCannotSeeTheDaemonsEnvironment` | `cmd.Env = validatorEnv()` removed | `the validator read a daemon secret out of its environment: MATRIX_API_KEY=sk-live-second-sentinel` |
| `TestTheValidatorEnvironmentIsAnAllowlistOfNames` | `EDITOR` added to the allowlist | `the validator allowlist is [PATH HOME LANG LC_ALL TZ TMPDIR EDITOR], want [...]` |
| `TestAVerdictEventWhosePayloadIsGoneStillAnswersFromItsStatus` | the fallback to `event.Status` removed | `acceptance = "unverifiable", want "incomplete" from the event status` |
| `TestSimultaneousTerminalsRecordExactlyOneVerdict` | the per-run `deliveryMu` removed | 19 of 20 runs red, `8 simultaneous terminal paths recorded 8 verdicts` (the 20th passed: the proof is statistical, not deterministic); with the lock, 5 of 5 green |
| `TestEndpointIsRedactedByDefault`, `TestEndpointRevealsOnlyOnTheDocumentedOptIn` | the redaction in `logredact.Endpoint` | `Endpoint("api.deepseek.com") = "api.deepseek.com", want "***"` |
| `TestModelIDConflictNamesTheAgentAndTheRemedy`, `TestModelIDConflictKeepsTheRemediesTheRuntimeStatusOffers` | the shared remedy rewritten on the runapi surface only | both red: `the refusal does not say what to do ("restart the daemon")` — the remedy has one definition and both surfaces call it |
| `TestIdentityComparisonFiresOnAConstantThatDenominatesAnAgent`, `TestIdentityComparisonResolvesAQualifiedConstantFromAnotherPackage`, `TestIdentityComparisonFiresOnAConstantWhoseValueNamesNoAgent` | the constant resolution removed (the pending-candidate loop) | all three red with `[]string(nil)`: a constant naming an agent became invisible again |
| `TestIdentityComparisonWholeWordRuleIsReportedHonestly` | the whole-word boundary replaced by a substring match | red: `"mimosa"` reported as `which names an agent` |
| `TestIdentityComparisonReviewedConstantPairDoesNotBlindTheName` | the constant pair keyed by value (`name=value`) instead of `name=const:ConstName` | red: 3 findings where the reviewed pair should have cleared exactly one |

Earlier reverts from the same verification work, including the ones that
survived and produced the tests above, are enumerated in
`docs/governance/delivery-contract-reverts.md` (the writer's own table, which
this audit sampled and confirmed) and in the previous baseline sections.

**Sampled from that table and re-run by this audit**: the rows for
`TestUnverifiableOutranksIncompleteInEitherOrder`, `TestAnUnresolvablePathSaysWhyItCouldNotBeResolved`,
`TestARealNoisyValidatorCannotLeakItsOutput`, `TestAValidatorCannotSeeTheDaemonsEnvironment`,
`TestTheValidatorEnvironmentIsAnAllowlistOfNames`,
`TestAVerdictEventWhosePayloadIsGoneStillAnswersFromItsStatus`,
`TestSimultaneousTerminalsRecordExactlyOneVerdict`,
`TestModelIDConflictKeepsTheRemediesTheRuntimeStatusOffers` — all 8 reproduce. The
Lead's summary said this table held 27 rows; it declares **31**, and the Lead has
confirmed the count was miscounted on his side (a message counted as a file, and
an uncounted number). The correction is registered here as his: the count in the
file is the one a reader can check.

## 5. What is not claimed

- No end-to-end exercise of the daemon over a real socket was run for the ack
  and discovery surfaces: they were verified at handler level.
- The storage-growth hypothesis in §2.2 is not measured. **The Lead's ruling: it
  stays a labelled hypothesis**, not a finding — measuring it needs a quota or a
  store-size probe, which the audit did not build.
- The pid-reuse window in §2.6 is not exploited and stays a labelled hypothesis
  by the same ruling.
- The 11 constant-based branches are now **visible to the gate** and declared as
  reviewed pairs (§1), so the next one fails; they are still not *fixed*, because
  fixing them means moving those decisions to a declaration — a design change,
  not this audit's mandate.
- `cmd/matrix` was **4627 LOC against a 4400 ceiling** on the frozen tree; the
  split is a writer's task assigned by the Lead, and this audit neither performs
  it nor re-measures it: the tree moved after the freeze, and a measurement is
  only a measurement of the tree it was taken on.
- **A correction of the Lead's own, registered as his**: his message said the
  revert table held "27 reversioni". The table in
  `docs/governance/delivery-contract-reverts.md` declares **31** rows; the number
  in the file is the one a reader can check. Nothing else in his summary of the
  reverts was wrong.
