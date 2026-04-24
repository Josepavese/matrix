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
