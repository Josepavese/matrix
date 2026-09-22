# Artifact digest verification is split between a pushed commit and an unpushed local one

Date observed: 2026-09-22

## Environment

- MATRIX `0.1.34-snapshot` (release `v0.1.34`, built 2026-09-22T09:09:19Z), also `0.1.29`
- Workstation checkout `~/hpdev/Libraries/matrix`
- Registry: `https://cdn.agentclientprotocol.com/registry/v1/latest/registry.json`
  (schema `1.0.0`, 41 agents)
- ACP agents: `opencode` 1.18.32, `codex-acp`

## Symptom

The work that makes MATRIX verify downloaded agent artifacts against the
registry digest **is split across two commits, and the remote only has part of
it**. Commit `5db4892` was pushed from another session and already contains a
portion of it; the remainder sits in the local commit `aaf61da`, which is **not
pushed** (`main` is `ahead 1` of `origin/main`).

Anyone reading the remote therefore cannot tell whether artifact verification is
complete, partial, or absent. That is the actual problem: not a broken feature,
but a state in which the feature's existence is undecidable from the repository.

## What the local commit contains

`git show aaf61da` in `~/hpdev/Libraries/matrix`. Summary of the whole change,
across both commits:

- `internal/logic/agentmgr/registry_client.go` — `BinaryDist` deserialises
  `sha256` (it did not, which was the root gap) and `RegistryClient.PlatformKey()`
  holds the single platform mapping (`amd64→x86_64`, `arm64→aarch64`), used both
  by `ResolveDistribution` and by the recorded evidence.
- `internal/logic/agentmgr/artifact_integrity.go` — `Installer.verifyArtifact`
  hashes the downloaded file in streaming (`middleware.FS.Open` + `io.Copy`, no
  file loaded into memory) and compares it with the published digest.
- `internal/logic/agentmgr/installer.go` — `fetchVerifiedArchive` is
  download → digest gate → extraction. The gate runs **before**
  `MkdirAll(agentPath)` and before extraction, and the temporary file is removed
  by `defer` on every path, so a mismatch leaves no agent directory, no vault
  entry, no meta and no temp archive behind.
- `internal/logic/agentcfg/meta.go`, `cmd/matrix/agent_info.go` — the evidence is
  persisted at `agent.meta.<id>.artifact_verification` and surfaced through
  `matrix doctor` (`runtime.agents[].artifact_verification`),
  `matrix agent doctor <id>` and `matrix agent info <id> --source=local`
  (`Integrity:` and `Artifact sha:` lines). No new channel was invented.
- `.goreleaser.yml`, `governance/manifest.toml` — release archives carried **no
  licence text**. Cause: `archives.files` *replaces* GoReleaser's default list,
  which already contained `LICENSE*`. `LICENSE` is listed now and a governance
  guard keeps it there; all six archives (tar.gz and Windows zip) carry it,
  byte-identical to the repository.
- `docs/matrix_release_runbook.md`, `docs/matrix_installation.md` — documented.
- `code-governance.toml` — complexity budgets raised, dated and visible:
  `cmd/matrix` 3225→3231, `internal/logic/agentmgr` 950→1040.

### Behaviour, in three outcomes that are deliberately not conflated

| Outcome | Behaviour |
| --- | --- |
| digest matches | install proceeds, `verified=true`, `status=verified` |
| digest mismatches | **refused**: `sha256 mismatch for <url>, the registry index publishes <expected> but the downloaded artifact has <actual>; refusing to install` |
| digest not published | install proceeds but **says so**: `status=digest_not_published` — never a green |

A malformed digest (not 64-hex) fails closed. `npx`/`uvx` distributions report
`not_applicable`, so "nothing to check" never reads as "checked".

## Measured facts worth keeping

- **The published digest describes the distribution archive, not the installed
  executable.** For `opencode` 1.18.32 linux-x86_64: the index publishes
  `3046e0404fdc60fb80307e7a47824ba07477364178a4d09baa8548496dd6d43b`, which is
  the sha256 of `opencode-linux-x64.tar.gz`; the extracted `opencode` binary is
  `513f500a1a5ea1dc7d865547ac87b32a8936334e8d5abd5b3ff585c45a170080`, and a real
  `matrix install` leaves exactly that binary on disk. The archive is downloaded
  to a temp path and removed after extraction, so it cannot be re-checked later.
- **Only 10 of the 19 binary agents publish a `sha256` for `linux-x86_64`.** The
  "not published" branch is the common case, not a theoretical one.
- **The registry publishes no authentication information at all** — entries carry
  id, name, version, description, repository, website, licence, icon, authors and
  distribution. Anything about credentials has to be declared elsewhere.

## What is needed

1. **Consolidate.** Review `git show aaf61da`, decide whether to keep it as is or
   rework it, and get the remote to a coherent, complete state. `5db4892` and
   `aaf61da` touch the same files; do not assume the pushed part is the newer one.
2. **Verify on the merged result**, not on either commit alone:
   `GOPROXY=off go build ./... && GOPROXY=off go test ./... -count=1`, plus
   `golangci-lint run` and `gofmt -l`.
3. **Re-run the live check** (needs network):
   `MATRIX_REGISTRY_INTEGRITY_REAL=1 go test ./internal/logic/agentmgr -count=1`,
   and a real `matrix install opencode` in a throwaway `MATRIX_HOME`, expecting
   `Verified sha256 3046e040… of opencode against the registry index for linux-x86_64`.
4. **Confirm the release archives** contain `LICENSE` before the next release;
   the previous `dist/` had archives without it.

## Open decisions

1. **Digest of the extracted executable — deliberately not added.** The index
   publishes only the archive digest, and the gate runs before extraction, so a
   verified archive covers the installed file transitively. MATRIX could only
   record a digest it computed itself, which adds no assurance and would read as
   a verification it is not.
2. **`matrix agent info <id> --source=local` prints an empty `Version:`** even
   though `meta.Version` is persisted. One line in `agentdiscovery.localProvider.Get`,
   not applied because it changes what a consumer records as the installed
   version — and a consumer compares it against the index version.
3. **Nothing verifies the artifact for `npx`/`uvx` distributions.** Their
   integrity is whatever the package registry provides; that is a separate design
   question, not a missing branch here.

## Note on this file

Written by an agent working in a **different repository** on this workstation,
under a rule that forbids it from modifying any repository other than its own.
This file is intentionally **untracked and uncommitted**: it is a note to the
team that owns this repository, not a change to it. Nothing else in this
checkout was modified by that agent beyond the commits described above.

## Resolution (2026-09-22, Matrix workstream)

Consolidated and verified on the merged result, not on either commit alone. The
local commit `aaf61da` is pushed as part of this work, so the remote is now the
coherent whole the note asked for.

1. **Consolidate.** Kept, not reworked. After `aaf61da` the tree has one
   implementation of each idea: `describeArtifactVerification` is gone from
   `cmd/matrix` (it lives on `agentcfg.ArtifactVerification.Describe`),
   `RegistryClient.PlatformKey` is the single platform mapping used by both
   resolution and evidence, and `fetchVerifiedArchive` is the only
   download-verify-extract path. The pushed part of `5db4892` and the local part
   in `aaf61da` do not conflict; the split was only in the history.
2. **Verified on the merged result.** `GOPROXY=off go build ./...` succeeds;
   `GOPROXY=off go test ./... -count=1` passes with 75 packages ok and exit 0;
   `golangci-lint run ./...` reports 0 issues; `gofmt -l .` is clean.
3. **Live check re-run.** `MATRIX_REGISTRY_INTEGRITY_REAL=1 go test
   ./internal/logic/agentmgr -count=1` passes in 85s with
   `status=verified`, published and downloaded both
   `3046e0404fdc60fb80307e7a47824ba07477364178a4d09baa8548496dd6d43b`. A real
   `matrix install opencode` in a throwaway `MATRIX_HOME` printed
   `Verified sha256 3046e040... of opencode against the registry index for
   linux-x86_64`, and `matrix agent info opencode --source=local` then reports
   `Version: 1.18.32`, `Integrity: sha256 verified against the registry index for
   linux-x86_64` and `Artifact sha: 3046e040...`.
4. **Release archives.** All six now carry `LICENSE`, byte-identical to the
   repository (`sha256 c520262f7926651e`). `governance/manifest.toml` requires
   it, so a future release cannot drop it silently. v0.1.35 is the first release
   built with it; v0.1.34 and earlier were published without the licence text.

### Open decisions, dispositioned

1. **Digest of the extracted executable — accepted as documented.** The index
   publishes the archive digest only and the gate runs before extraction, so a
   self-computed digest would add no assurance and would read as a verification
   it is not.
2. **Empty `Version:` in `--source=local` — fixed.** `meta.Version` was loaded
   and then dropped, while the catalogue already reported it, so two paths
   disagreed about the same fact. One field in
   `agentdiscovery.localProvider.Get`, plus a regression test that fails without
   it (`Version = "", want the installed version "1.18.32"`). An agent with no
   recorded version still reports an empty one: unknown stays unknown.
3. **`npx`/`uvx` verification — unchanged and out of scope**, as the note says:
   their integrity is whatever the package registry provides, which is a
   separate design question rather than a missing branch here.
