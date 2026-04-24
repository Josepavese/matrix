# Release Evidence Template

Use this template for every production-oriented Matrix release.

## Release Identity

- Version:
- Commit:
- Tag:
- Release URL:
- Operator:
- Date:

## Local Preflight

- Command: `bash scripts/deploy_preflight.sh`
- Result:
- Governance gate:
- Code governance:
- Lint:
- Race tests:
- Build:
- GoReleaser check:

## GitHub Actions

- CI run URL:
- CI conclusion:
- Governance job:
- Lint job:
- Test job:
- Build job:
- Release dry-run job:
- Release workflow URL:
- Release workflow conclusion:

## Artifacts

- Linux amd64:
- Linux arm64:
- macOS amd64:
- macOS arm64:
- Windows amd64:
- Windows arm64:
- Checksums:
- Installer shell:
- Installer PowerShell:

## Local Install

- Install command:
- PAL home:
- Installed `matrix version`:
- Installer used release artifact, not source build:
- Existing vault data preserved:

## Runtime Evidence

- Readiness command:
- Readiness result:
- Runtime expected up:
- Smoke command:
- Smoke agent:
- Smoke protocol:
- Smoke result:
- Cleanup evidence:

## Warnings And Exceptions

- Accepted warnings:
- Reason:
- Owner:
- Follow-up issue:

## Signoff

- Release captain:
- Runtime operator:
- Security reviewer:
