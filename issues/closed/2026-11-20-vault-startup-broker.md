# Vault startup and broker publication

**Decision.** Accepted as a Matrix performance and lifecycle defect.
[GitHub #22](https://github.com/Josepavese/matrix/issues/22).

The writable bbolt open used to scan every value and commit a transaction
even when the on-disk format was already current. The first inspection now
records the format in bbolt metadata. Legacy conversion records it in the
same transaction as the verified writes. Subsequent opens only read the
format. The broker startup record is published before vault open and retired
after the broker is published. The daemon holds the OS lock for its lifetime.
An orphaned JSON marker after a crash does not prevent CLI access or restart.

**Linux qualification.** With a synthetic 415,600,640-byte vault and 98,000
encrypted payloads, v0.1.67 took 123–135 ms to initialize logging and
148–182 ms to publish the broker. Two candidate starts on the same vault took
3.5–4.6 ms and 7.9–19.7 ms respectively. The old value check in isolation took
129 ms. The candidate recorded the existing encoding in 35 ms and then
checked it in 10 µs. Opening bbolt took 53–91 µs, current schema reads
15–897 µs across the runs, and agent registry initialization 94–123 µs.
Measurements use the local Linux filesystem and warm cache. The incident's
140 s remains an observation from the reporting host.

An actual daemon held initialization while a test process held the vault
lock. The CLI waited and returned the agent after broker publication. Killing
the daemon before broker publication left an orphan JSON marker. Offline CLI
inspection and the next start succeeded. Regression tests also cover
exclusive ownership, an aged marker, unlabelled current vaults and rollback
when conversion fails.
