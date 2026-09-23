# Windows Release Validation: v0.1.34

## Why this document exists

The release runbook lists validation on a clean operating system as a release
criterion, and for v0.1.30 through v0.1.33 the evidence recorded it as not done.
A container run covered Linux; the Windows installer and the Windows binary were
packaged, checksummed, and never executed. This is the record of closing that gap
on a real Windows 11 guest.

## What was validated

Artifact: the Windows archive published from the v0.1.34 release, installed by
the `install.ps1` published in the same release.

Host: `Microsoft Windows 11 IoT Enterprise LTSC Evaluation`, build
`10.0.26100.1742`, PowerShell 5.1, AMD64, created from the
`windows-11-iot-ltsc-eval` Nido blueprint and booted with 2 vCPUs and 4 GiB.

Script: `tests/windows_release_validation.ps1`, which takes the installer from
the published release rather than a local copy.

## Result

All nine checks passed:

```
host_os=Microsoft Windows 11 IoT Enterprise LTSC Evaluation
powershell=5.1.26100.1591
arch=AMD64
ok   download install.ps1 from the published release -> 5464
ok   installer sha256 -> ddce284db40bad67364138195c971ef862e92c4cf16674327d939067e72d2058
ok   matrix.exe installed -> True
ok   PAL home created -> agents,artifacts,backups,bin,configs,data,logs,tmp
ok   matrix.exe runs -> matrix 0.1.34
ok   matrix.exe reports the release version -> matrix 0.1.34
ok   doctor runs on Windows -> exit=0
ok   readiness runs on Windows -> status=not_ready blockers=vault file is missing; vault schema is not current
ok   configs were seeded without overwriting -> 5
```

What each line establishes:

- The installer is reachable from the published release and downloads.
- `install.ps1` verified the archive against `checksums.txt` before unpacking
  (`Verified checksum for matrix_0.1.34_windows_amd64.zip`).
- The PAL home was created with the same directory shape as on Linux, plus the
  `agents` directory.
- The installed `matrix.exe` runs and reports the released version.
- `matrix doctor` succeeds on Windows.
- `matrix readiness` on a host with no vault reports the missing vault as a
  **blocker** rather than failing with a raw storage error. This is the fresh-host
  fix from v0.1.34, confirmed on a second operating system, and it is the check
  that would have failed before that fix.
- Configuration files were seeded without overwriting anything.

## How the guest was driven

The VM has no shared filesystem with the host, so the script and its report
travelled over HTTP: the host served the script on the QEMU user-network gateway
(`10.0.2.2:8000`) and the guest posted its report back to the same collector. The
console was driven over VNC.

Two defects were found in the harness itself, not in Matrix:

- `$LASTEXITCODE` is not set by invoking a PowerShell script, so the installer's
  success was being read from a stale value. The check now uses a try/catch.
- With `$ErrorActionPreference = "Stop"`, PowerShell 5.1 turns a native command's
  stderr output into a terminating error, which made the readiness check fail on
  the warning that the fresh-host fix deliberately writes to stderr. The check
  relaxes the preference for that one call.

## Installed tooling

Nido on this workstation was v4.5.12 from February 2026 and was updated to
v4.5.27 from the repository source during this work, with the previous binary
kept at `~/go/bin/nido.v4.5.12.bak`. `nido doctor` reports every check passing
with the new binary.

## Defects found in the tooling

- **Nido v4.5.12, the installed binary, predates the blueprint variable
  substitution fix** that is present in the repository source. It wrote
  `{{windows_image_index}}` into `Autounattend.xml` unsubstituted, and Windows
  Setup failed with `0x80070003 - 0x40030` ("path not found") before the
  installation started. Building Nido from the repository source and running the
  build with that binary produced a correct answer file and a successful install.
  The installed binary is dated February 2026; the repository is current.
- The same build cached the Windows ISO under a name derived from the URL
  (`?clcid=0x409&country=us&culture=en-us&linkid=2270353`) rather than the
  blueprint's `iso_name`. The current source names it correctly and migrates the
  legacy file, so the 5 GiB download was reused rather than repeated. This is the
  same stale-binary root cause.
- `nido spawn --port 2222:22` produced `hostfwd=tcp:127.0.0.1:22-:2222`, that is,
  the mapping is interpreted as `guest:host`, not `host:guest`. Not a blocker,
  but it fails in a way that reads like a port conflict.
- A VM named `qa-native-01` was deleted as part of the disk cleanup and then
  reappeared on its own within a minute. This was first recorded here as Nido
  reusing a freed name, and that was wrong: the name belongs to a native-QA bench
  driven by **another agent session working on the same workstation**, which
  recreates its guest and later renamed it to `qa-installer-01` to stop the
  conflict. One `nido delete` of an unrelated session destroyed that bench in the
  middle of a validation. Nido behaved correctly throughout; the mistake was
  deleting VMs without checking who owned them.

## Second release checked the same day

The same guest and the same script were re-run for v0.1.35 with
`-Version v0.1.35`, and all nine checks passed again, including `matrix.exe`
reporting `matrix 0.1.35`. That run exposed a defect in the script rather than in
the release: the version assertion was hard-coded to `0.1.34`, which pinned the
script to the release it was written for. It now derives the expected version from
`-Version`.

## Running it: host side is automated

`tests/windows_validation_host.sh` does the host half, which used to be a throwaway
Python server rewritten by hand every time:

```
tests/windows_validation_host.sh --version v0.1.38
```

It serves `tests/windows_release_validation.ps1`, prints the two lines to type in the
guest, waits for the POSTed report, and exits non-zero unless every check passed.
`--self-test` drives the whole collector with a synthetic report — including the
case where a check failed and the run must not be reported as valid — so the
plumbing is verified on a host with no Windows guest at all; CI runs that in the
`windows-validation-host` job.

What remains manual is the guest console itself: the VM has no SSH login, so the two
commands are typed into it. Automating that needs the guest to accept a connection
(OpenSSH or the QEMU guest agent) and is worth doing when the guest is next rebuilt,
because it is the last step that requires a human at the screen.

## Rebuilding the guest

The guest's writable overlay was deleted on 2026-09-23 to reclaim 4.9 GB on a
disk that runs near capacity; the base image and the blueprint were kept, so the
validation capability is intact and only the guest's own state is gone. To rebuild
it:

```
nido spawn matrix-win-validation --blueprint windows-11-iot-ltsc-eval
nido start matrix-win-validation --gui
```

That boots a fresh Windows 11 IoT Enterprise LTSC guest. It then needs the same
one-time setup this document assumed: create the `vmuser` account, log in, and note
that the console is reachable over VNC on `127.0.0.1:59000` while the HTTP server
that serves this script must listen on the QEMU user-network gateway, reachable
from the guest as `10.0.2.2`. Budget roughly half an hour, most of it Windows
first-boot.

Two things learned the hard way, worth keeping: the guest's console locks itself,
and typing into a locked screen sends the keystrokes to the password field, so
check the screen before typing; and `vncdo` mangles shifted characters unless it is
run with `--force-caps`.

## What this does not cover

- No interactive desktop use of Matrix on Windows: the checks are the installer,
  the binary, the doctor and readiness. The Telegram bridge, the runtime daemon
  and the vault broker were not exercised.
- The VM is a Nido guest on this workstation, not a physical machine, and the
  evaluation image is a 90-day Windows evaluation.
- PowerShell 5.1 (Windows PowerShell) ran the validations; PowerShell 7 was not
  exercised on the guest.
- `install.ps1`'s checksum and archive-path guards are additionally covered on
  every push by the `install-ps1-security` CI job, which does not need Windows.
