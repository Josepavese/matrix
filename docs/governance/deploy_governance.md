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

Use [release_evidence_template.md](release_evidence_template.md) to record the result.

Normal operator UX must be `matrix`, without manual environment variables. Development and test workflows may override `MATRIX_HOME` only to isolate versions or avoid touching the user PAL home.

Do not kill or restart a shared Matrix daemon during governance-only work. Runtime smoke that may affect shared users requires explicit coordination.
