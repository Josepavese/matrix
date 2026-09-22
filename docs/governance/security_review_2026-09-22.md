# Security Review: 2026-09-22

## What this is

A security pass over the repository as it stood before v0.1.34, performed by an
autonomous agent. It covers static analysis, dependency vulnerabilities, secret
scanning, a manual read of the newest attack surface, and an install smoke test
on a clean operating system image.

It is **not** a human security review, not a penetration test, and not a claim
that the code is secure. The release runbook lists security review as requiring a
human; this document is the evidence an operator needs in order to perform one,
not a substitute for it.

## Method

| Control | Tool | Result |
|---|---|---|
| Static analysis | `gosec ./...` (installed from source for this toolchain) | 51 findings before the fixes, 45 after |
| Dependency vulnerabilities | `govulncheck ./...` | 0 affecting called code |
| Secrets in tracked files | `scripts/security_secret_scan.sh` | no high-confidence secrets |
| Clean-OS install | release artifact in a minimal `debian:stable-slim` container | installs and runs; see below |
| Manual review | elicitation surface, vault paths, archive extraction, HTTP surface | see findings |

The manual review covered the code this release series added or changed: the
elicitation request path (wire projection, SSOT validation, HTTP and Telegram
frontends), the trace projection, the vault key handling, and the agent install
path.

## Findings fixed

1. **Reflected content in the OAuth callback page** (`internal/providers/matrixapi`).
   The page served after an OpenRouter login interpolated the router's message
   into HTML without escaping. Today every implementation returns a fixed string,
   but the value crosses an interface boundary, so a future implementation could
   return provider-supplied text and execute it on the local origin. The message
   is now HTML-escaped, with a regression test that fails if the escaping is
   removed.
2. **SSE field injection** (`internal/providers/runapi`). The event name was
   written into a line-oriented protocol unfiltered, so a newline in an event kind
   would have injected additional fields or whole events. Event names are internal
   constants today; they are now reduced to a single safe field value.
3. **Weak hash for trace identity** (`internal/logic/frontendevents`). Correlation
   ids were derived with SHA-1. Nothing is authenticated or hidden by them, so it
   was not a vulnerability, but a reader should not have to reason about whether a
   weak hash matters here. Now SHA-256.
4. **Slowloris on the local HTTP API** (`cmd/matrix`). The server had no
   `ReadHeaderTimeout`, so a connection that never finished its headers could hold
   a worker. Now bounded, with an idle timeout.
5. **World-readable configuration files** (`internal/providers/osfs`). Config
   writes used `0644`. Channel configuration can carry identifiers and tokens, and
   Matrix is single-user, so there is no reader that needs group or world access.
   Now `0600`, consistent with the vault and the log file.
6. **Unbounded archive expansion** (`internal/providers/osfs`). Agent installs
   extract archives downloaded from outside the process, and the copy loop had no
   ceiling: a decompression bomb could fill the disk. Extraction now charges a
   shared 2 GiB budget and refuses to exceed it with a clear error.
7. **Fresh-install readiness** (found by the clean-OS check, not by a scanner).
   On a machine that had never run Matrix, `matrix readiness` exited with a raw
   storage error instead of reporting a blocker, hiding the rest of the report
   exactly when an operator needs it. A vault that cannot be inspected is now
   reported as the `vault file is missing` blocker, and the command exits 0 (or 2
   with `--strict`).

## Findings accepted, with the reason

- **`G304` file inclusion via variable** (23 findings, mostly `scripts/` and the
  CLI): these tools read paths the operator passes on the command line or sets in
  configuration. An operator who can choose the path can already read the file.
- **`G204` subprocess launched with a variable** (7 findings): launching the
  configured agent binary is the product's purpose. The calls use
  `exec.CommandContext(executable, args...)` with no shell, so no interpolation
  occurs.
- **`G703` path traversal via taint in `vaultsec`**: the master key path comes from
  `MATRIX_HOME` or an explicit override, both operator-controlled, and the file is
  opened with `0600` and validated for permissions. No agent-supplied value reaches
  it.
- **`G115` integer conversion in archive mode** (`int64` to `uint32`): the mode is
  immediately masked by `safeArchiveFileMode`, which returns only `0644` or
  `0755`, so the truncated bits cannot survive.
- **`G301`/`G302`/`G306` permissions**: the flagged directories are general
  filesystem operations at `0755`, which is conventional for extracted content and
  mounts. Every path that stores Matrix's own secrets (vault, master key, log
  file, runtime broker descriptor, config files) uses `0600` or `0700`; the
  sensitive ones were re-verified by reading the code, and one was corrected (see
  finding 5).
- **`G122` symlink TOCTOU in `scripts/governance_check`**: a build-time checker
  that walks the repository it lives in.
- **`G110` decompression bomb**: not accepted — fixed, see finding 6.
- **`G705` XSS in the SSE endpoint**: the endpoint serves `text/event-stream` with
  JSON payloads and `%q`-quoted errors, so there is no HTML context. The event
  name was still hardened, see finding 2.

## Clean-OS validation

The published Linux amd64 snapshot artifact was extracted and run inside a
`debian:stable-slim` container with an empty `MATRIX_HOME`:

- `matrix version` reports `0.1.33-snapshot` with the expected commit.
- `matrix doctor` exits 0 and prints a structured report.
- `matrix readiness` on the fresh home exited 1 with a raw error before the fix
  and 0 with `status: not_ready` and `blockers: ["vault file is missing"]` after
  it.
- `matrix vault migrate` followed by `matrix vault backup` produces a backup file.

This is the substitute for the runbook's clean-OS VM step: the Nido VM tooling is
not installed on this workstation, and Docker is. It exercises the artifact and
the first-run path, but not a second operating system.

## What remains open

- No human security review has been performed.
- No authentication or authorization testing beyond reading the code: the local
  API's optional key, the vault broker token, and the channel webhooks were
  reviewed by inspection, not attacked.
- No load, soak, or resource-exhaustion testing beyond the archive budget.
- `install.ps1` is now exercised, but only in part: its checksum and archive-path
  guards are tested by `tests/install_ps1_security_test.ps1`, which runs locally
  under PowerShell Core and on every push in the `install-ps1-security` CI job.
  The rest of the script (release download, PAL home creation, PATH update) still
  needs a Windows host.
- The `gosec` run is a point-in-time snapshot; it is installed ad hoc and is not
  yet part of the preflight.
