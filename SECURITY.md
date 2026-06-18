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
