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
6. **Every generated release artifact is install-smoked before the deploy is considered done.**

## Required Gates

These gates must be green before a production-oriented release:

- code governance
- Go formatting
- lint
- race-enabled tests
- build
- release dry-run
- install artifact layout check
- local PAL install update
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

### Actions To Watch

Every push to `main` must leave the `CI` workflow green.

Watch these jobs explicitly:

- `governance`: code size, structure, and repository hygiene.
- `lint`: `golangci-lint` static analysis.
- `test`: full Go test suite.
- `build`: CLI build check.
- `release-dry-run`: GoReleaser snapshot for Linux, macOS, and Windows on amd64 and arm64.

The `release-dry-run` job is a packaging gate, not a cosmetic check. It must prove that the release config can generate the installable archive set that the no-clone installers expect.

Tag pushes also start the `Release` workflow.

Watch this job explicitly:

- `goreleaser`: publishes the tagged release artifacts and checksums.

The `Release` workflow is allowed only after `CI` is green on the exact commit being tagged.

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

For a complete local deploy, use the wrapper:

```bash
scripts/deploy_local.sh
```

It runs preflight, generates GoReleaser snapshot artifacts, and installs the host artifact into the local PAL home.

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

### 5. Local PAL Install Update

After the executables and archives have been generated, update the local system from the generated host-matching artifact:

```bash
scripts/deploy_local_install.sh
```

This step installs from `dist/`, not from source. It is the deploy smoke that proves the generated package can update the local PAL home.

Expected behavior:

- selects the current host OS and architecture
- installs `matrix` into `MATRIX_HOME/bin`
- creates the PAL home directories if missing
- copies only missing seed configs
- leaves existing vault data untouched
- prints the resolved PAL home from the installed binary

Use a custom PAL home for isolated deploy drills:

```bash
MATRIX_HOME="$(mktemp -d)" scripts/deploy_local_install.sh
```

### 6. Tag Release

Only tag after CI is green on `main`.

Use semantic versions in `vX.Y.Z` form. When Matrix creates a release
autonomously, increment only `Z` from the latest existing tag. Increment `X`
or `Y` only when explicitly requested by the maintainer.

```bash
git tag vX.Y.Z
git push origin vX.Y.Z
```

After pushing the tag, watch the `Release` workflow until the `goreleaser` job is green.

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
- local deploy scripts if install behavior changed

## Done Criteria

A deployment process is considered healthy when:

- `main` is green
- failed Actions are triaged immediately
- release commands are documented
- local PAL install update has been run from generated artifacts
- local preflight matches CI gates
- readiness output is reviewed before deploy
