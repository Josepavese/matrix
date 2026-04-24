# Security Governance

Matrix is local-first, but local-first is not security by itself.

Rules:

- Vault material is secret and must never be committed, logged, or copied into docs.
- Provider tokens, HTTP keys, Telegram tokens, and installer credentials are secrets.
- Public installers must not require a GitHub token.
- Installer scripts may download release artifacts and write the PAL home, but must not clone the full repository for normal users.
- Logs must avoid request secrets and vault keys.
- Backups and exports must preserve the boundary between user data and executable artifacts.

If a workflow needs a secret, the secret must be configured through the PAL home or the platform secret store, not through command examples in documentation.
