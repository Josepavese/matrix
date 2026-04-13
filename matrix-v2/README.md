# Matrix V2

Matrix V2 is a Go-based system daemon and CLI tool for AI orchestration and system communication.

## Product Profile

Matrix V2 is now scoped as a local-first orchestration runtime.

Officially supported:
- `run`
- `bootstrap doctor`
- `doctor`
- `logs`
- `agent`
- `channel`
- `vault`
- `session`

Explicitly experimental:
- `apm`
- `fuse`

The detailed product profile is in [PRODUCT.md](/home/jose/hpdev/Libraries/matrix/matrix-v2/PRODUCT.md).

## Architecture

This project strictly adheres to the Platform Abstraction Layer (PAL) and Single Source of Truth (SSOT) patterns.

## Getting Started

```bash
go build -o matrix ./cmd/matrix
./matrix
```

## Bootstrap

Use the bootstrap report as the first entrypoint for a fresh workspace:

```bash
go run ./cmd/matrix bootstrap doctor
go run ./cmd/matrix doctor
go run ./cmd/matrix run
```

`matrix bootstrap doctor` reports:
- whether first-run onboarding is already completed
- whether Telegram is configured
- which agents are active
- the recommended next steps for this workspace

## Local Configuration

Public config files under `configs/` are safe defaults/templates.

Runtime lookup order:

- Telegram: `configs/telegram.json` -> `MATRIX_TELEGRAM_CONFIG` -> vault `channel.telegram.*` -> `MATRIX_TELEGRAM_*`
- Agents: Resoluti dinamicamente tramite Vault e ACP Registry.

Agent runtime state is not read from local JSON override files.
Agent mutations live in the SSOT vault.

Optional Telegram env overrides:

- `MATRIX_TELEGRAM_TOKEN`
- `MATRIX_TELEGRAM_ENABLED`
- `MATRIX_TELEGRAM_ADMINS`

Preferred vault keys:

- `channel.telegram.token`
- `channel.telegram.enabled`
- `channel.telegram.admins`

Example:

```bash
cd matrix-v2
go build -o matrix ./cmd/matrix
MATRIX_TELEGRAM_TOKEN="..." MATRIX_TELEGRAM_ENABLED=true ./matrix run
```

## Agent SSOT Commands

Agent definitions are fetched from the ACP Registry and stored in the Vault.
Runtime mutations are managed through the SSOT vault:

```bash
go run ./cmd/matrix install opencode
go run ./cmd/matrix uninstall opencode
go run ./cmd/matrix agent set-binary custom /usr/bin/ls
go run ./cmd/matrix agent list
```

## Agent Discovery

Browse agents from the ACP Registry:

```bash
go run ./cmd/matrix agent search              # list all available agents
go run ./cmd/matrix agent search codex         # search by name or description
go run ./cmd/matrix agent info codex-acp       # show remote agent details
```

Install any supported distribution type:

```bash
go run ./cmd/matrix install codex-acp          # auto-detect: binary, npx, or uvx
go run ./cmd/matrix install gemini             # npx distribution (no download needed)
```

The registry index is cached locally in the vault for offline access.

## Channel SSOT Commands

Channel providers use the `channel.<provider>.*` namespace in the vault.
That namespace is managed through `matrix channel ...`, not `matrix config ...`.

```bash
go run ./cmd/matrix channel list
go run ./cmd/matrix channel show telegram
go run ./cmd/matrix channel set telegram enabled true
go run ./cmd/matrix channel set telegram admins "[123456789]"
go run ./cmd/matrix channel set telegram token "..."
printf '...\n' | go run ./cmd/matrix channel set telegram token --stdin
go run ./cmd/matrix channel delete telegram token

go run ./cmd/matrix bootstrap doctor
go run ./cmd/matrix vault doctor
go run ./cmd/matrix vault backup --dir ./backups
go run ./cmd/matrix vault restore ./backups/matrix-vault-YYYYMMDD-HHMMSS.db
go run ./cmd/matrix vault get my.secret.key
go run ./cmd/matrix vault get my.secret.key --reveal
MATRIX_VAULT_MASTER_KEY=... go run ./cmd/matrix vault seal
printf 'secret-value\n' | go run ./cmd/matrix vault set my.secret.key --stdin
```

## Security Notes

- Do not commit real bot tokens or API keys into versioned files.
- Prefer SSOT vault or environment variables for live secrets.
- Prefer `matrix vault set ... --stdin` for secret writes to avoid shell history.
- `matrix vault get` redacts secret-like keys by default; use `--reveal` only when necessary.
- Vault encryption is driven by `MATRIX_VAULT_MASTER_KEY` or `MATRIX_VAULT_MASTER_KEY_FILE`, never by a key stored inside the vault itself.
- Use `MATRIX_TELEGRAM_CONFIG` only as an explicit private bootstrap file, not as an implicit default path.
- If a secret was ever committed, rotate it before continuing to use it.
