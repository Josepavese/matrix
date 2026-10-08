# Security Policy

Matrix is experimental local-first software. Do not expose its HTTP or JSON-RPC
listeners outside localhost unless you have configured API keys and understand
the risk.

## Supported Versions

Only the latest tagged release receives security fixes.

## Reporting A Vulnerability

Please report suspected vulnerabilities privately by opening a GitHub security
advisory for this repository, or by contacting the maintainer through the
repository owner profile if advisories are unavailable.

Do not publish exploit details until a fix or mitigation is available.

## Secret Handling

Never include provider tokens, Telegram bot tokens, API keys, vault databases,
or logs containing prompts/secrets in public issues. Redact `matrix-vault.db`,
`configs/*.local.json`, and runtime logs before sharing diagnostics.


## Vault value identity and upgrades

Vault values use AES-GCM with the exact storage key as authenticated associated
data (`ENCV2`). Moving a ciphertext to a different key fails authentication.
The first writable open converts older ENCV1/plaintext records atomically, after
checking space and verifying a private compact backup of all original records.
A failed conversion leaves the original records unchanged. Read-only access
never performs a conversion and never treats a retired envelope as plaintext.

The backup defaults to the vault directory. An operator may set the existing
absolute directory `MATRIX_VAULT_MIGRATION_BACKUP_DIR` before upgrading to place
it on another filesystem. Keep that backup and the original master key for
rollback; an older binary must not be used against the upgraded vault.

Agent programs and explicitly authorized validators run with the operator's OS
privileges. Child environments exclude daemon vault material, but file access
by the same OS user is not a sandbox boundary. OS isolation is a separate policy.
