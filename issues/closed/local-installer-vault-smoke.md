# Local deploy must not migrate the Vault as an artifact smoke

Status: closed — corrected on 2026-10-09 after v0.1.54 installation qualification.

The public release installer does not run `vault migrate`. The maintainer's
`scripts/deploy_local_install.sh` did, followed by bootstrap/doctor/readiness
commands that can open storage. Read-write bbolt initialization also scans for
legacy encrypted values. An already-current schema does not bypass that scan.
The live Vault was 2.6 GB; the qualification wrapper timed out at this phase.

The local deploy wrapper now verifies only version, home and CLI help after
copying the verified archive. Runtime schema/storage initialization remains with
the application; doctor/readiness are run after service startup using its broker.
No ciphertext conversion or schema change is required by the CLI-only v0.1.54 fix.

A fixture archive and spy binary reject every storage/runtime command. The test
installs and reruns over existing Vault/configuration sentinels and checks they
remain byte-identical. It is wired into the installer CI job.

The local install was recovered using the verified public binary, matching
configuration hashes and the coherent offline backup. Service and authenticated
API checks passed. This wrapper is repository tooling, not a distributed binary
or public release installer; the v0.1.54 binary/tag does not change.
