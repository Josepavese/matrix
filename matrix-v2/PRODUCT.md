# Matrix V2 Product Profile

## Official Scope

Matrix V2 is officially positioned as a local-first AI orchestration runtime with:

- SSOT vault on BoltDB
- structured runtime logging
- dynamic agent discovery via ACP Registry and Vault
- registry-based installation through `matrix install` (binary, npx, uvx)
- runtime control through `matrix run`
- health and bootstrap inspection through `matrix doctor` and `matrix bootstrap doctor`
- channel model with Telegram as the first supported provider
- agent protocols via `stdio` and `ws` (ACP protocol first)
- HTTP `/runs` entrypoint for end-to-end validation and automation

## Supported Surface

The supported surface of the product is:

- `matrix install` (ACP Registry — binary, npx, uvx)
- `matrix agent ...` (SSOT Vault)
- `matrix agent search` (ACP Registry discovery)
- `matrix agent info` (ACP Registry agent details)

These commands are considered part of the maintained product profile.

## Experimental Surface

These areas remain explicitly non-core and may change without stability guarantees:

- `matrix fuse ...`

They stay in the repository, but they are not the center of the product claim.

## Product Non-Goals

Matrix V2 is not currently positioned as:

- a hosted multi-tenant platform
- a remote control plane
- a general-purpose package marketplace
- a production-ready semantic filesystem product
- a broad multi-channel integration suite

## Done Criteria

For the current product profile, "done" means:

- core runtime works from local bootstrap to real request execution
- configuration and secrets follow the SSOT policy
- runtime diagnostics and logging are usable in practice
- security posture is explicit and operationally defensible
- the supported surface is documented and distinguished from experimental areas
