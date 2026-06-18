# Deploy Governance

A Matrix deploy is complete only when release evidence exists.

Required release evidence:

- Governance gate passes.
- Code governance passes.
- Lint passes.
- Race-enabled tests pass.
- The CLI builds.
- GoReleaser validates the release config.
- GitHub Actions is green for CI.
- Tag release workflow completes.
- Cross-platform artifacts are produced for Linux, macOS, and Windows on amd64 and arm64.
- The installer can install from release artifacts without cloning the repository.
- A local install is performed from the generated release.
- Runtime readiness and at least one real smoke are recorded when the change touches runtime behavior.
- Any Nido VM created for validation is reused when possible, never duplicated
  for the same OS/purpose, and deleted when it is no longer needed.

Use [release_evidence_template.md](release_evidence_template.md) to record the result.

Normal operator UX must be `matrix`, without manual environment variables. Development and test workflows may override `MATRIX_HOME` only to isolate versions or avoid touching the user PAL home.

Do not kill or restart a shared Matrix daemon during governance-only work. Runtime smoke that may affect shared users requires explicit coordination.

Nido VM hygiene is part of release governance. Before creating a Linux,
Windows, or macOS validation VM, inspect the existing Nido inventory and reuse a
matching clean VM when possible. Do not create duplicate VMs for the same
release/OS role. Delete disposable VMs immediately after the install/smoke
evidence is captured. If a VM must be retained, record its name, owner, reason,
and expiry in the release evidence.
