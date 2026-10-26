# Agent artifact store and regenerable software

This page describes the contract Matrix offers so that many PAL homes can run the
same installed agent software without keeping one copy each, and so a consumer can
leave that software out of a data backup without losing recoverability.

It is a contract about *software* Matrix installed from the governed agent
registry. It does not change the Vault, the run surface, the protocol behaviour of
any agent, or any consumer's backup policy: it gives the consumer the facts and
the restore path to make its own policy.

## Why the store exists

An ACP-registry binary install downloads a verified archive and extracts it under
the PAL home (`<home>/agents/<agent_id>`). That makes every home, and therefore
every worker in a Hub, carry its own copy of the same bytes. Measured on one
deployment, that copy dominated the data set: eight worker homes held the same
~185 MB agent tree each, and a Hub backup peak exceeded the free space it had.

The store is the smallest generic change that fixes the duplication without
weakening any integrity evidence:

- the bytes are addressed by the same sha256 Matrix already verifies against the
  registry index, so a store generation is *the* artifact a verified install
  would have extracted;
- each PAL home keeps a small reference to a generation instead of the software;
- the inventory and restore commands make the software reproducible, which is
  what allows a consumer to exclude it from a backup.

## Roles: one publisher, many consumers

The store is written by one publisher role and read by consumers. That split is
the security boundary:

- `matrix agent store publish <agent_id> --store <root>` downloads the binary the
  registry publishes, refuses anything it cannot verify (no published sha256, a
  malformed sha256, or a digest mismatch), extracts it into a staging directory,
  records a tree manifest, and publishes it atomically as an immutable generation.
  npx/uvx distributions download no artifact and are not publishable.
- `matrix install <agent_id> --store <root> [--digest <sha256>]` registers the
  agent for this PAL home but points it at the verification store generation. It
  never downloads, never extracts, and never copies the software into the home.

Publishing requires write permission on the store. Installing from it requires
that the runtime user **cannot** write the store: a store the runtime user can
modify cannot prove that the bytes it serves are the bytes the publisher verified,
so the consumer role fails closed instead of trusting it. The check covers the
store root and the layout directories that decide what a generation contains
(`blobs`, `blobs/sha256`, `generations`); the remediation is to publish from a
different account or mount the store read-only for the runtime user.

Matrix does not, and cannot, enforce that separation in the filesystem. That is a
deployment property: the operating system's ownership, mount options or ACLs
decide it. What Matrix does is refuse to proceed when it can see that the property
does not hold.

### Store layout

```
<root>/
  blobs/sha256/<digest>/...          extracted, read-only generation tree
  generations/<digest>.json          generation metadata (agent, version, platform, launcher, args)
  generations/<digest>.manifest.json tree manifest (path, mode, size, sha256) and its digest
  tmp/                               publish staging, publisher only
```

- `<digest>` is the lowercase sha256 of the published archive, the same value the
  registry index published and Matrix verified before extracting.
- Publication is atomic: the tree is staged, sealed (write bits removed; launchers
  stay executable), manifested and renamed into place, and only then are the
  metadata files written. Re-publishing the same generation is idempotent; a
  different tree or different immutable fields under the same digest is refused.
- `matrix agent store verify <digest> --store <root>` recomputes the tree manifest
  from the store and compares it exactly with the published one. This is an offline
  proof of the software's bytes; it does not need the network or the registry.

### Consumer state

A store-backed install writes two small records, never software:

- the Vault record (`configs` + Vault metadata) now carries the absolute launcher
  path inside the store generation and an `ArtifactReference` naming the store,
  digest, tree-manifest digest, launcher, version and platform;
- `<home>/configs/agent_references.json` maps agent id to that same reference, so
  garbage collection and a consumer backup can see which digests this home still
  needs without opening the Vault.

Both are deliberately outside the software directory: a backup that keeps
`configs/` and the Vault keeps everything needed to reinstall, and can drop
`<home>/agents/`.

## Commands

Publisher side:

```bash
matrix agent store publish claude --store /srv/matrix/store
matrix agent store list --store /srv/matrix/store
matrix agent store verify <digest> --store /srv/matrix/store
matrix agent store gc --store /srv/matrix/store --home /home/a/.local/share/matrix --home /home/b/.local/share/matrix
```

`--store` defaults to `$MATRIX_AGENT_STORE` on every store subcommand. `gc` is a
dry run by default and deletes only with `--apply`; it refuses to run without at
least one `--home`, because it must know every home whose references to keep. It
never deletes a referenced generation, and it also removes publish staging older
than one hour. As a guard against a wrong home list, `--apply` is also refused
when the given homes carry no store reference at all: `--allow-empty-refs`
confirms that the store really is unused.

Consumer side:

```bash
matrix install claude --store /srv/matrix/store           # resolve the current registry version
matrix install claude --store /srv/matrix/store --digest <sha256>   # exact generation
matrix agent inventory --json
matrix agent verify claude
matrix agent restore --inventory inventory.json --store /srv/matrix/store --dry-run
matrix agent prune claude            # dry run: report reclaimable per-home bytes
matrix agent prune claude --apply    # delete only the superseded per-home copy
```

- `matrix agent inventory` is deterministic and secret-free: agents sorted by id,
  environment reduced to variable names, no install timestamps and no values. It is
  a document a consumer can compare across homes (`matrix agent inventory --json`).
- `matrix agent verify` reports `store_tree` for store-backed agents (full offline
  tree verification) and `recorded_evidence` for per-home installs. The second
  method proves the launcher exists and that the install recorded a verified
  digest; it does **not** re-prove the bytes offline. `--strict` exits non-zero
  unless every agent is fully verified.
- `matrix agent restore` installs from a verified store generation when the
  inventory says the agent is store-backed, otherwise from the governed registry.
  It refuses to substitute a different registry version unless
  `--allow-version-drift` is passed, and `--dry-run` reports without downloading or
  writing.
- `matrix agent prune` reclaims a per-home copy only after the recorded store
  generation verifies *and* the agent still points at it. It is a dry run by
  default, removes only the software directory, and keeps the Vault record and the
  store reference: the agent stays installed from the store.

## Using the store to shrink a consumer backup

The store changes what a backup has to contain, not the backup policy: the
consumer decides. A deployment that wants the benefit does this once per store and
per home:

1. As the publisher account, publish each binary agent for each platform:
   `matrix agent store publish <agent_id> --store <root>`.
2. As the runtime user of each home, reinstall the agent from the store:
   `matrix install <agent_id> --store <root>`, then reclaim the old per-home copy:
   `matrix agent prune <agent_id> --apply`.
3. Keep `configs/` and the Vault in the data backup; exclude `agents/` from it.
4. Restore with `matrix agent restore --inventory <inventory.json> --store <root>`
   (or, for registry installs, `matrix agent restore --all`), then verify with
   `matrix agent verify --strict`.

No savings are declared here before measurement: the reclaimable size is the
actual size of the per-home software directories (`du -sb <home>/agents`) and
`matrix agent prune` reports the bytes it would free for the agents it can
reclaim.

## Limits (stated explicitly)

- **Cross-home immutability is a deployment property.** Matrix validates the
  permissions it can see (mode bits and, on Unix, ownership) and refuses a store
  writable by the runtime user. On platforms where ownership cannot be proven
  (Windows), an owner-writable store is treated as writable, so a Windows
  deployment must make the store read-only for the runtime account.
- **A store generation belongs to one platform.** `linux-x86_64`,
  `darwin-aarch64`, … are separate trees because registry artifacts are
  platform-specific. A store can hold many platform generations side by side.
- **The verified unit is the archive, and the tree is what Matrix can prove.**
  Extraction reuses the existing archive provider, which silently drops unsafe
  archive entries (symlinks, hardlinks, absolute paths) instead of extracting them.
  A hostile *launcher* entry therefore fails closed (the launcher is missing), but a
  hostile non-launcher entry is dropped and the reduced tree is what the generation
  publishes and verifies. The tree manifest therefore describes exactly the bytes
  that exist in the store, not the discarded archive entries.
- **No hardlink or symlink conversion.** The store does not link a home's copy to a
  generation: it copies verified bytes once at publish time, and homes only
  reference. Converting an existing per-home install is the explicit sequence
  publish → install from store → prune; Matrix never rewrites or deletes a home's
  software on its own.
- **Garbage collection is deployment-coordinated.** `gc` trusts the `--home`
  paths it is given. A home whose reference file is not passed can have its
  generation reclaimed; run `gc` dry first and keep the home list in the
  deployment runbook. The empty-reference guard refuses `--apply` when the list
  yields no reference at all, but it cannot know about a home it was never told
  about.
- **A store-backed home depends on the store being mounted.** If the store is
  unavailable, the agent cannot launch; that is a visible launch failure, not a
  silent fallback to an old copy. The Vault traffic, sessions and agent records
  are unaffected, and `matrix agent verify` reports the store-backed agent as not
  verified until the store is reachable again.
- **No removal of consumer data.** `prune` deletes only the software directory of
  an agent that already runs from a verified store generation, and only with
  `--apply`. The Vault, agent configuration, sessions, artifacts and backups are
  not touched. Matrix does not change the Hub's backup configuration.
- **Matrix verifies declarations, not agent behaviour.** The store strengthens the
  provenance of the installed bytes; it does not and cannot prove what an agent
  does at runtime, which requests it sends, or what a provider does with them.

## Related

- [Using Agents](wiki/Using-Agents.md) — agent installation and configuration.
- [CLI Reference](wiki/CLI-Reference.md) — command syntax.
- [Deployment policy](matrix_deployment_policy.md) — PAL homes and service layout.
