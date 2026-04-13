# Matrix V2 Configs

This directory contains versioned defaults and templates.

## Safe to version

- `agents.json`
- `telegram.json`
- `telegram_test.json`
- `node_env.json`
- `locales/*.json`

These files should not contain live secrets.

## Resolution order

### Agents

Agent definitions are no longer read from static files.
Use `matrix agent search` to browse available agents from the ACP Registry.
Use `matrix install <id>` to install agents (binary, npx, or uvx distributions).
Use `matrix agent info <id>` for remote agent details.
Runtime mutations belong in the SSOT vault through `matrix agent ...`.

### Telegram

1. `configs/telegram.json`
2. `MATRIX_TELEGRAM_CONFIG` if explicitly set
3. SSOT vault keys
4. `MATRIX_TELEGRAM_TOKEN`, `MATRIX_TELEGRAM_ENABLED`, `MATRIX_TELEGRAM_ADMINS`

Vault keys supported:

- `channel.telegram.token`
- `channel.telegram.enabled`
- `channel.telegram.admins`

`channel.telegram.admins` can be stored as JSON array or comma-separated list.

## Recommended workflow

1. Keep versioned files as safe templates.
2. Run `matrix bootstrap doctor` first.
3. Keep `configs/telegram.json` secret-free.
4. Use `MATRIX_TELEGRAM_CONFIG` only as an explicit bootstrap override file when needed.
5. Prefer vault or env vars for live Telegram secrets.
6. Use `printf '...' | matrix channel set telegram token --stdin` for channel secrets.
7. Use `matrix vault set ... --stdin` only for raw vault entries, not as a bypass for `matrix channel ...`.
8. Use `matrix vault doctor` to inspect vault file security posture and encryption state.
9. Use `matrix vault backup --dir ./backups` before risky local operations.
10. Use `MATRIX_VAULT_MASTER_KEY` or `MATRIX_VAULT_MASTER_KEY_FILE` to enable vault encryption, then run `matrix vault seal`.
11. Use `matrix vault restore <backup>` only while the runtime is stopped.
12. Manage channel runtime overrides through `matrix channel ...`.
13. Manage agent runtime state through `matrix agent ...`.
14. Rotate any secret that was previously committed or shared.

Do not manage `channel.*` keys through `matrix config ...`.
Each channel provider exposes an explicit supported key set through `matrix channel show <provider>`.
