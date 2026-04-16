# Matrix Deployment Policy

## Purpose

Deployment must be boring, visible, and repeatable.

No release should depend on memory, chat history, or one person remembering which checks matter.

## Deployment Principles

1. **Every deploy starts from a clean Git state.**
2. **Every deploy has an explicit preflight.**
3. **Every CI failure is tracked before more code is added.**
4. **Every release is reproducible from the repository root.**
5. **Every warning is either accepted explicitly or fixed.**

## Required Gates

These gates must be green before a production-oriented release:

- code governance
- Go formatting
- lint
- race-enabled tests
- build
- release dry-run
- install artifact layout check
- readiness check

Local preflight command:

```bash
bash scripts/deploy_preflight.sh
```

## CI Policy

CI is split into independent jobs:

- `governance`
- `lint`
- `test`
- `build`
- `release-dry-run`

This is intentional.

If governance fails, tests and build still run. The team gets the full failure map instead of losing signal after the first failed step.

## Action Failure Policy

When any GitHub Action fails:

1. Open the failed run.
2. Identify the failed job and step.
3. Reproduce the failed command locally.
4. Fix or document the failure.
5. Push a repair commit.
6. Confirm the next run is green.

Do not continue feature work while `main` is red unless the work is directly repairing the red build.

## Deployment Roles And Skills

### Release Captain

Owns the release checklist and signs off on the final deploy.

Required skills:

- read GitHub Actions failures
- run local preflight
- understand readiness output
- decide whether warnings are acceptable

### CI Triage

Owns failed pipeline diagnosis.

Required skills:

- reproduce CI locally
- inspect governance output
- distinguish blocker vs warning
- produce a minimal repair commit

### Runtime Operator

Owns the runtime environment.

Required skills:

- run `matrix readiness`
- run `matrix doctor`
- manage vault migration and backup
- inspect logs and runtime ports

## Release Workflow

### 1. Preflight

```bash
git status --short
bash scripts/deploy_preflight.sh
```

### 2. Runtime Readiness

```bash
go run ./cmd/matrix vault migrate
go run ./cmd/matrix readiness
go run ./cmd/matrix doctor
```

Use strict mode only when the target environment has the required vault and runtime state:

```bash
go run ./cmd/matrix readiness --strict
```

### 3. Backup

Before a real environment deploy:

```bash
matrix vault backup --dir ./backups
```

### 4. Release Dry-Run

```bash
goreleaser release --snapshot --clean
```

The dry-run must produce OS/architecture archives that include:

- `matrix` or `matrix.exe`
- `configs/`
- `install/install.sh`
- `install/install.ps1`
- installation documentation

Installers must install into one PAL home and must not require cloning the repository.

### 5. Tag Release

Only tag after CI is green on `main`.

```bash
git tag vX.Y.Z
git push origin vX.Y.Z
```

## Governance Policy

The code-governance file is a ratchet.

- Existing exceptions are allowed only as explicit baseline.
- New exceptions require a reason.
- Large files should be split before increasing limits.
- Increasing a governance budget is a deployment-policy change, not a casual fix.

## Rollback Policy

Rollback must be possible from:

- Git tag
- previous binary artifact
- vault backup

Minimum rollback checklist:

```bash
matrix vault backup --dir ./backups
matrix vault restore ./backups/<known-good-backup>
```

## Documentation Policy

Any deploy workflow change must update:

- this document
- [matrix_release_runbook.md](matrix_release_runbook.md)
- CI workflow if commands changed

## Done Criteria

A deployment process is considered healthy when:

- `main` is green
- failed Actions are triaged immediately
- release commands are documented
- local preflight matches CI gates
- readiness output is reviewed before deploy
