# Code Governance

Code governance is enforced by `code-governance.toml` and the deploy preflight.

Rules:

- Package, file, function, parameter, and branch budgets are hard signals.
- An override is allowed only when it is narrower than the default rule and has a clear engineering reason.
- New features should reduce coupling between protocols, channels, vault, orchestration, and providers.
- New code should prefer small ports, capability types, and testable orchestration units over vertical flows.
- Generated, installer, and governance scripts may have different shape constraints, but they still need readable ownership.

Weakening the budget to pass CI is not allowed. If a budget is wrong, update the policy and document why the new threshold is healthier for Matrix.

Quality warnings are tracked as explicit debt in [code_debt_register.md](code_debt_register.md).

The warning budget is a ratchet:

- existing warnings may remain only while they are documented baseline debt
- new warnings fail governance
- worse maximum branch complexity fails governance
- when refactors reduce debt, lower the baseline immediately

## Test governance

Size budgets measure production code only. Test lines are counted separately
(`prod_loc` and `test_loc` in the report), so writing tests can never push a
package towards a budget failure. The test policy in the same config states what
must be tested:

- **Every package with production code carries tests.** `min_test_functions_per_package`
  counts `Test*` and `Fuzz*` functions; benchmarks and examples do not count,
  because they verify nothing.
- **Packages with real logic assert a failure path.** `min_behavior_tests_per_package`
  requires at least one test whose name names a rejection, failure, or degraded
  case, gated to packages above `behavior_min_prod_loc`. A package that only
  declares data has no rejection path, and demanding one would be theatre. The
  name fragments live in `behavior_name_pattern`, so the rule is tuned in the
  config and not in the checker.
- **Test code has a floor relative to production code** (`min_test_loc_ratio`),
  applied only to packages above `ratio_min_prod_loc`.
- **Anything that decodes external bytes carries a fuzz target**
  (`fuzz_required_packages`). A green example-based suite never finds the
  malformed case nobody imagined; that is exactly how the unnamed-property,
  non-web-URL, and unknown-callback-action defects were found.
- **Anything that defines a wire contract carries an end-to-end test**
  (`interop_required_packages`). An in-process test cannot prove a peer agrees.

### Baselines, not silent exemptions

A package that does not yet reach the absolute minimum sits on a recorded
baseline (`baseline_test_functions`, `baseline_behavior_tests`,
`baseline_ratio`). A baseline is a floor:

- dropping below it fails governance, so legacy debt cannot grow;
- rising above it is reported, so the number gets recorded and the ratchet moves
  up;
- once the policy minimum is met, the entry is reported for deletion, so the
  list can only shrink;
- a new package is never allowed a baseline: it must meet the policy minimum
  from its first commit.

Regenerate the block after real improvement:

```bash
go run ./scripts/code_governance.go -config code-governance.toml -print-test-baseline
```

A package may also be exempted from the minima entirely (`test_policy.exemptions`),
but only with an ISO date and a reason, and the exemption is printed in every
report. An exemption is a claim that a test cannot be meaningful, not that it is
inconvenient.
