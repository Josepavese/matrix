# Contributing

Matrix is currently experimental. Contributions are welcome, but public APIs,
commands, and protocol integrations can still change.

## Local Checks

Before opening a pull request, run:

```bash
go run ./scripts/governance_check --manifest governance/manifest.toml
go run ./scripts/code_governance.go --config code-governance.toml
golangci-lint run
go test -race ./...
goreleaser check
```

For release packaging changes, also run:

```bash
goreleaser release --snapshot --clean
HOME="$(mktemp -d)" MATRIX_HOME="$(mktemp -d)" scripts/deploy_local_install.sh
```

## Runtime State

Do not commit generated binaries, release archives, local PAL homes, vault
databases, logs, or local config overrides.
