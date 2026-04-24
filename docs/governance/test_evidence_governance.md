# Test Evidence Governance

Tests must match the risk being changed.

Canonical evidence keyword: real-agent.

Evidence levels:

- Unit: pure parsing, policy, capability, and orchestration functions.
- Integration: vault, provider registry, session lifecycle, channel command parity, and HTTP surfaces.
- Real-agent smoke: changes touching ACP, provider behavior, streaming, fork, cancel, delete, session lifecycle, sidecar, or interrupt semantics.
- Release smoke: installer, PAL home layout, binary execution, readiness, and one real run from the installed artifact.

Real-agent evidence must name the agent, protocol entrypoint, command used, result, and unsupported capabilities observed.

If an agent does not expose a behavior through ACP or another protocol, Matrix must record that as capability-unsupported instead of claiming success.
