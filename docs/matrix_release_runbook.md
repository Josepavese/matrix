# Matrix Release Runbook

## Goal

A release should not depend on tribal knowledge.

This runbook defines the minimum repeatable release flow for Matrix in its current local-first form.

The governing policy is [matrix_deployment_policy.md](matrix_deployment_policy.md).

Record every production-oriented release with
[governance/release_evidence_template.md](governance/release_evidence_template.md).

## Preflight

Run:

```bash
bash scripts/deploy_preflight.sh
bash scripts/security_secret_scan.sh
```

For the full local deploy path, run:

```bash
scripts/deploy_local.sh
```

This wrapper runs preflight, generates snapshot artifacts, and updates the local PAL home from the generated artifact.

The preflight includes GoReleaser config validation. Before tagging a release, also run a snapshot release and inspect archive layout:

```bash
goreleaser release --snapshot --clean
```

The `sboms` section of `.goreleaser.yml` shells out to `syft`, and GoReleaser
fails rather than publishes a release without the SBOMs it was asked for, so
`syft` must be on `PATH` for that snapshot run. The `Release` workflow and the
CI `release-dry-run` job install it with `anchore/sbom-action/download-syft`
from the version pinned there; install the same version locally instead of
whatever `syft` happens to be first on `PATH`, or the snapshot's SBOMs are not
the ones the release will publish.

Each archive must contain the executable, the `LICENSE` text (Apache-2.0 requires
it to travel with a redistributed binary), `configs/`, installers, and
installation docs. Each archive is also published with an SPDX JSON inventory
named after it, `<archive>.sbom.json`.

After artifacts are generated, install the host-matching archive into the local PAL home:

```bash
scripts/deploy_local_install.sh
```

This is the final local deploy step. It must run from the generated archive in `dist/`, not from `go run` or a source build.

For an isolated smoke install:

```bash
MATRIX_HOME="$(mktemp -d)" scripts/deploy_local_install.sh
```

For clean-OS validation, use Nido VMs. Check the Nido inventory first, reuse a
matching clean VM when possible, and do not create duplicate VMs for the same
OS/release role. Delete disposable VMs after evidence is captured. If a VM is
kept, record its name, owner, reason, and expiry in release evidence.

If you expect a live local runtime to be up before release validation:

```bash
matrix readiness --expect-runtime-up
```

Without `--expect-runtime-up`, an inactive runtime is not a release warning.
Use that mode for artifact and install validation. Use `--expect-runtime-up`
only for environments where the daemon is expected to be serving traffic.

Runtime shutdown evidence:

```bash
matrix logs tail | grep runtime_signal_received
```

`matrix run` should remain reachable across sequential runs unless the process
receives an operator/system signal or the JSON-RPC daemon exits with an error.
When a signal stops the runtime, Matrix logs `runtime_signal_received` with the
exact signal before `shutdown_started` and `daemon_exited`. Treat
`connection refused` during a batch as inconclusive until logs prove whether
the process received `SIGINT`/`SIGTERM`, crashed, or was never started.

## Vault Key Runtime Context

The normal runtime key location is:

```text
MATRIX_HOME/configs/vault-master.key
```

Matrix discovers this file automatically. `MATRIX_VAULT_MASTER_KEY_FILE` and
`MATRIX_VAULT_MASTER_KEY` remain supported as explicit overrides for isolated
tests, CI, and emergency operations.

## Backup

Always back up the vault before any release deployment or manual restore drill:

```bash
matrix vault backup --dir ./backups
```

## Restore Drill

Periodically validate restore flow:

```bash
matrix vault restore ./backups/<backup-file>
```

Matrix will refuse restore while the local runtime appears active on the default runtime ports.

## Release Criteria

Minimum criteria for a local release candidate:

- high-confidence secret scan passes on tracked files
- governance manifest and architecture guardrails pass
- CI is green on `main`
- the `CI` workflow jobs `governance`, `lint`, `test`, `windows-codex-policy`,
  `build`, and `release-dry-run` are green
- tagged releases publish through the `Release` workflow `goreleaser` job
- every archive is published with an SBOM (`<archive>.sbom.json`, SPDX JSON,
  produced by the `sboms` section of `.goreleaser.yml` with the pinned `syft`),
  and the same job attests build provenance for every file in `checksums.txt`
  with `actions/attest-build-provenance`, which is why that job carries
  `id-token: write` and `attestations: write`
- the published release verifies end to end with `scripts/verify_release.sh <tag>`:
  the nine expected assets, one SBOM per archive under the expected name, every
  archive's and SBOM's sha256 against `checksums.txt`, `LICENSE` present in every
  archive and identical to the repository, and a build provenance attestation for
  every archive digest. Read the release **by id** when the tag-addressed view
  reports no assets: GitHub has served the tag document with an empty asset list
  for over an hour after publication (v0.1.36 and v0.1.37), which is what broke
  the installers, and a verification that reads it signs off a release nobody can
  install
- the SBOM and attestation checks are requirements, so that command fails on
  releases published before them (v0.1.39 and earlier); re-verifying an old tag
  is expected to report `SBOM asset present -> missing` and
  `provenance attestation ... -> no attestation for this digest`
- `matrix readiness` returns `ready` or `ready_with_warnings`
- vault schema is `current`
- no unexpected retention overflows remain
- test suite is green
- backup command works
- restore path has been drilled recently
- release archives install into one PAL home through `install/install.sh` and `install/install.ps1`
- local PAL home has been updated from the generated host artifact through `scripts/deploy_local_install.sh`
- clean-OS validation VMs, if used, were reused or deleted; retained VMs have owner/reason/expiry evidence

If a previous local daemon is still running during validation, `matrix doctor`
or `matrix readiness` may warn that the runtime endpoint report is invalid and
that local probe fallback was used. This is acceptable only when blockers are
empty and the warning clearly identifies the fallback path.

The SBOM and the attestation are evidence about what was published, not a
security review. The SBOM records what `syft` could identify inside each archive,
and the attestation binds those digests to the workflow, commit, and tag that
produced them. Neither says the binary is free of defects, and neither replaces
the secret scan or the human review below.

## Versioning Rule

Matrix releases use tags in `vX.Y.Z` form.

When creating a release autonomously, increase only the patch component `Z`.
Do not increase `X` or `Y` unless the maintainer explicitly requests a major
or minor release.

## Things That Still Need Human Review

The runbook does not replace:

- security review
- load/soak validation
- platform-specific packaging validation
- operator signoff on warnings

Warnings may still be acceptable for a local release, but blockers are not.
