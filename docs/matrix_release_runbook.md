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

The preflight includes GoReleaser config validation. Before tagging a release, also run a snapshot release and inspect archive layout:

```bash
goreleaser release --snapshot --clean
```

Each archive must contain the executable, `configs/`, installers, and installation docs.

If you expect a live local runtime to be up before release validation:

```bash
matrix readiness --expect-runtime-up
```

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
- `matrix readiness` returns `ready` or `ready_with_warnings`
- vault schema is `current`
- no unexpected retention overflows remain
- test suite is green
- backup command works
- restore path has been drilled recently
- release archives install into one PAL home through `install/install.sh` and `install/install.ps1`

## Things That Still Need Human Review

The runbook does not replace:

- security review
- load/soak validation
- platform-specific packaging validation
- operator signoff on warnings

Warnings may still be acceptable for a local release, but blockers are not.
