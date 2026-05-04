# External Project Issue Boundary Policy

## Purpose

Matrix can receive issue reports, integration feedback, and reproduction traces
from external projects. Those reports are valuable, but they must not turn
Matrix into project-specific glue.

Matrix remains a protocol-neutral, channel-neutral, local-first communication
matrix for humans, supervisory AI, and agents. Every correction or feature
accepted from an external project must strengthen that product boundary.

## Rule

Do not implement ad hoc behavior for a specific external project.

External project feedback can be accepted only when it is translated into a
general Matrix capability, contract, bug fix, or observability improvement that
is useful across providers, channels, workspaces, and agent-to-agent flows.

## Acceptance Criteria

An external issue is acceptable when all of these are true:

- The problem maps to a Matrix-owned responsibility such as routing,
  lifecycle, cleanup proof, session mirroring, protocol abstraction,
  channel abstraction, runtime observability, installer behavior, or governance.
- The resulting change is expressed in Matrix terms, not in the vocabulary or
  control flow of the reporting project.
- The implementation works without importing assumptions from the external
  project.
- The behavior is reusable by HTTP, Telegram, future channels, and supervisory
  agents where applicable.
- The behavior is protocol-neutral where possible, or explicitly capability
  gated where a protocol/provider lacks the required feature.
- Tests prove the generic contract with Matrix-owned fakes or real provider
  probes, not only the external project's scenario.
- Documentation describes the Matrix contract, not the external integration.

## Rejection Criteria

Reject or defer an external issue when any of these are true:

- The requested behavior exists only to satisfy one project's local workflow.
- The fix would encode project names, artifact paths, task arms, evaluator
  states, or domain-specific assumptions into Matrix.
- The request bypasses Matrix abstractions instead of improving them.
- The issue cannot be reproduced or reasoned about without running the external
  project, and no protocol-neutral failing contract can be extracted.
- The implementation would make one channel, provider, or caller special.

## Translation Pattern

When a project reports a problem, translate it before coding:

1. Identify the Matrix-owned invariant that failed.
2. Rename the failure using Matrix vocabulary.
3. Define the generic contract.
4. Add or update Matrix tests for that contract.
5. Implement at the abstraction boundary.
6. Document the Matrix behavior and the caller expectations.

Example:

- Project-specific report: "Project X fork/resume run retained an OpenCode ACP
  process after cleanup."
- Matrix contract: "Run cleanup must not report strong cleanup while any
  run-owned provider client remains alive or unaccounted."
- Generic implementation: cleanup proof includes related sessions, retained
  clients downgrade cleanup, and orphaned provider clients are reconciled.

## External Reproduction Policy

Do not run an external project's workflow from a Matrix maintenance session
unless the user explicitly asks for that exact cross-project validation.

Preferred flow:

- Read the issue report.
- Extract the Matrix-owned contract.
- Build Matrix-local tests and provider probes.
- Give the external project team a concise verification checklist.

If external execution is explicitly authorized:

- Treat the external repo as read-only unless separately authorized.
- Use a clearly named temporary output directory.
- Report any artifacts created.
- Stop all spawned external processes after the test.
- Do not commit or modify external project files.

## Documentation Requirement

Accepted external feedback must update Matrix documentation when it changes a
public contract. The documentation must use Matrix product language:

- protocol-neutral session lifecycle;
- channel-neutral commands and APIs;
- cleanup and recovery evidence;
- runtime and run-trace observability;
- provider capability gates;
- PAL/local-first installer behavior.

Avoid naming the reporting project except in the local issue response or release
evidence. The product docs should remain general.

## Maintainer Checklist

Before closing an externally reported issue:

- The issue has been translated into a Matrix invariant.
- No project-specific identifiers were added to production code.
- Tests cover the invariant without depending on the external project.
- Real provider testing was run when the change touches protocol/provider
  behavior.
- Docs describe the generic Matrix behavior.
- The issue response states what Matrix guarantees and what callers must verify.

