# Code Governance

Code governance is enforced by `code-governance.toml` and the deploy preflight.

Rules:

- Package, file, function, parameter, and branch budgets are hard signals.
- An override is allowed only when it is narrower than the default rule and has a clear engineering reason.
- New features should reduce coupling between protocols, channels, vault, orchestration, and providers.
- New code should prefer small ports, capability types, and testable orchestration units over vertical flows.
- Generated, installer, and governance scripts may have different shape constraints, but they still need readable ownership.

Weakening the budget to pass CI is not allowed. If a budget is wrong, update the policy and document why the new threshold is healthier for Matrix.
