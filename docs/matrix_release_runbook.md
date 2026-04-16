# Matrix Release Runbook

## Goal

A release should not depend on tribal knowledge.

This runbook defines the minimum repeatable release flow for Matrix in its current local-first form.

The governing policy is [matrix_deployment_policy.md](matrix_deployment_policy.md).

## Preflight

Run:

```bash
bash scripts/deploy_preflight.sh
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

Each archive must contain the executable, `configs/`, installers, and installation docs.

After artifacts are generated, install the host-matching archive into the local PAL home:

```bash
scripts/deploy_local_install.sh
```

This is the final local deploy step. It must run from the generated archive in `dist/`, not from `go run` or a source build.

For an isolated smoke install:

```bash
MATRIX_HOME="$(mktemp -d)" scripts/deploy_local_install.sh
```

If you expect a live local runtime to be up before release validation:

```bash
matrix readiness --expect-runtime-up
```

Without `--expect-runtime-up`, an inactive runtime is not a release warning.
Use that mode for artifact and install validation. Use `--expect-runtime-up`
only for environments where the daemon is expected to be serving traffic.

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

- CI is green on `main`
- the `CI` workflow jobs `governance`, `lint`, `test`, `build`, and `release-dry-run` are green
- tagged releases publish through the `Release` workflow `goreleaser` job
- `matrix readiness` returns `ready` or `ready_with_warnings`
- vault schema is `current`
- no unexpected retention overflows remain
- test suite is green
- backup command works
- restore path has been drilled recently
- release archives install into one PAL home through `install/install.sh` and `install/install.ps1`
- local PAL home has been updated from the generated host artifact through `scripts/deploy_local_install.sh`

## Things That Still Need Human Review

The runbook does not replace:

- security review
- load/soak validation
- platform-specific packaging validation
- operator signoff on warnings

Warnings may still be acceptable for a local release, but blockers are not.
