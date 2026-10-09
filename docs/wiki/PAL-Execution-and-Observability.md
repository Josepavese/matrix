# PAL Execution and Observability

These optional surfaces use shared contracts on Linux, Windows and macOS.
An explicit request is refused when its driver, image or observation is missing;
Matrix does not silently remove the requested boundary.

## Platform support and evidence

| Surface | Linux | macOS | Windows |
|---|---|---|---|
| Native home, capacity, semantic CLI/HTTP, collector contracts | Native tests | Native tests | Native tests |
| Provider permission contracts | Shared contract | Shared contract | Shared contract |
| Optional Docker Linux container | Local engine + cached image | Local Linux-container engine, such as Docker Desktop | Local Linux-container engine, such as Docker Desktop |
| Optional semantic mount | rclone + FUSE | rclone + macFUSE/FUSE-T | rclone + WinFsp |
| Private Unix run notifications / CLI wait/ack | Available | Available | Use HTTP events/elicitations |

Real container and FUSE qualification is recorded for Linux. Native tests on
macOS/Windows do not certify their Docker or mount drivers. Qualify those
installed drivers in your intended environment. See
[implementation evidence](../governance/pal_implementation_2026-10-08.md) and
[v0.1.53 release evidence](../governance/releases/2026-10-08-v0.1.53.md).

## Sandbox

`MATRIX_SANDBOX` is a JSON declaration in the agent environment. Configure it
with `matrix agent env set <id> MATRIX_SANDBOX '<json>'` in Bash, or pass the same
JSON as one quoted argument in your platform shell. Unknown fields, duplicate
keys, unsupported profiles and unknown contracts are refused.

Provider-native permissions and the OS boundary can be requested independently
or together. Native policy reports `configured_not_attested`: provider-managed,
project or agent settings can affect the final tool policy. Docker uses a local
engine and Linux containers; Matrix does not install the engine, pull images
or build them at launch.

### Native permissions

```json
{"native_contract":"opencode-permission-v1","native_profile":"read-only"}
```

For MiMo use `mimocode-permission-v1`. The operator declares a versioned contract;
it is independent of the agent's assigned ID. These adapters apply
`OPENCODE_PERMISSION` / `MIMOCODE_PERMISSION`, preserving existing ordered rules
and adding denials:

| Profile | Added denials |
|---|---|
| `workspace` | `external_directory`, `task`, `actor` |
| `read-only` | The above plus `edit`, `write`, `patch`, `multiedit`, `bash` |

Permission bypasses in governed argv/env are refused. Matrix disables its own
host ACP tools, terminal authentication and automatic selection of the most
permissive mode. Caller MCP, extra directories and extension tools would need
another boundary and are refused under this policy.

The v1 adapters follow the [OpenCode v1 permission schema](https://dev.opencode.ai/docs/permissions/)
and [MiMo source](https://github.com/XiaomiMiMo/MiMo-Code/blob/main/packages/cli/src/config/permission.ts).
The [OpenCode v2 schema](https://opencode.ai/v2/docs/permissions) differs and is
not covered by the v1 contract. Codex's `codex-acp-env-v1` launch policy is a
separate provider contract; see [Using Agents](Using-Agents.md).

### Process isolation

Example declaration: choose paths, UID/GID and a prepared image for your host.
The image must already contain the agent and required tools.

```json
{
  "native_contract": "opencode-permission-v1",
  "native_profile": "workspace",
  "container": {
    "engine": "docker",
    "image": "my-prepared-agent:version",
    "workspace_access": "workspace-write",
    "network": "bridge",
    "state_dir": "/explicit/provider-state-base",
    "user": "1000:1000",
    "memory_bytes": 536870912,
    "cpus": 2,
    "pids": 128
  }
}
```

`workspace_access` is `read-only` or `workspace-write`; network is `none` or
`bridge`. UID/GID are numeric and nonzero. RAM, CPU and PID limits are explicit.
Matrix resolves the cached image to its digest and verifies engine support and
effective container settings before attaching.

The declared workspace mounts at `/workspace`, persistent state at
`/home/matrix`. The root is read-only; `/tmp` is a 64 MiB no-exec mount.
Capabilities are dropped and `no-new-privileges` is required. Binds exclude
nested mounts. Host root/home, state overlapping the workspace, implicit image
volumes and remote engines are refused. Docker's log driver is `none` to avoid
another transcript copy in engine storage.

The guest command must exist **inside the Linux image**, also on Windows hosts:

```bash
matrix sandbox command opencode /usr/local/bin/opencode --arg=acp --arg=--pure
matrix sandbox doctor opencode --workspace /existing/host/workspace
```

Arguments are guest arguments; host paths are not implicitly translated. ACP
receives `/workspace`. A task's `sandbox_execution` reports the image digest and
`engine_configuration_verified`, distinct from native tool-policy attestation.

Persistent provider state is scoped by identity, command and physical workspace
and remains across restart/profile changes. Supply credentials explicitly via
the provider environment or its state directory; Matrix does not mount the host
home. Cleanup removes only the random container created by this driver and
preserves provider state. Changing policy cannot reuse a client with an old
boundary: release its leases and reap it while preserving unrelated tasks.

Doctor checks prerequisites, not a completed container handshake. A workspace
is required to probe actual isolation; global runtime readiness can report
`sandbox_requires_workspace_probe` until the task boundary is exercised.

### Validators

`delivery_contract.validator.sandbox` accepts the same `container` object.
Its command is an argv in the prepared image. The verdict uses the actual exit
code without retaining stdout/stderr. The runner shares limits, mounts,
cancellation and scoped cleanup. Missing drivers or engine failure yield
`unverifiable`, without executing the validator on the host as a fallback.

## Capacity

```bash
matrix capacity /existing/workspace
matrix workspace show <workspace-id>
```

Observations describe the actual workspace filesystem: volume identity, bytes
available to the user and total bytes, logical CPUs and native RAM readings.
Linux uses statfs/sysinfo, Windows volume/memory APIs, macOS statfs/sysctl/vm_stat.
`memory_source` declares semantics: Linux physical free excludes reclaimable
cache; macOS also excludes speculative pages. Windows reports immediately
available physical memory, including standby pages, as defined by
[MEMORYSTATUSEX.ullAvailPhys](https://learn.microsoft.com/windows/win32/api/sysinfoapi/ns-sysinfoapi-memorystatusex).
These observations promise neither allocation nor a RAM quota. Zero is a valid
observation; unavailable data is absent with its reason. Observation creates no
files and does not clean the disk.

Global configuration: `capacity.max_concurrent` and
`capacity.min_disk_free_bytes` (zero disables the respective limit).
`POST /v1/runs` may add:

```json
{"capacity":{"min_disk_free_bytes":1073741824,"reserve_disk_bytes":268435456}}
```

Leases cover active routes across this runtime's channels. Volume reservations
are bookkeeping, not preallocation or filesystem quotas. They release on errors
and cancellation and do not cover other runtimes/processes or provider credits.
The container provides actual process CPU/RAM/PID limits when configured.

Authenticated `GET /_matrix/capacity?workspace_id=<id>` exposes observations.
Missing observations and exhausted limits have distinct codes recorded in the
trace and terminal notification. See [API Reference](API-Reference.md#pal-state-and-observability).

## Semantic filesystem

```bash
matrix fs list
matrix fs path runs <run-id>
matrix fs read runs/<encoded-id>/status.json
matrix fuse mount /existing/empty/mountpoint --driver-path /path/to/rclone
```

This is selected **Matrix state**, not a mount of project files or a semantic
search index. It exposes `agents`, `runs`, `workspaces`: declared agent state,
run status, workspace metadata and capacity. `matrix fs path` returns the full
`status.json` path; do not append that filename again.

IDs are reversibly encoded, distinct on case-insensitive filesystems and avoid
Windows reserved names. Limits: 125 UTF-8 bytes per ID, 4096 entries per directory,
64 KiB per summary. Limit failures are explicit rather than silent truncation.
Configuration, credentials, task input, raw errors and arbitrary caller metadata
are excluded. Terminal `summary.txt` requires `--include-summaries` and can
contain private text. Authenticated `GET/HEAD /_matrix/fs/` always excludes
summaries. Each read is a snapshot; separate files are not one atomic transaction.

The mount uses [rclone HTTP](https://rclone.org/http/) plus a native driver:
Linux FUSE, macFUSE/FUSE-T or Windows WinFsp. Its private local HTTP projection
uses its own random key, not the Matrix admin key. Owner-only settings,
read-only mount and no disk cache are enforced. Live views avoid HEAD/directory
cache; size before reading can be unknown/zero. Readiness needs an actual
installed-driver mount, not just compilation. Existing mountpoint data is not
hidden and unmount does not remove the directory.

## Collector

The optional exporter uses [OTLP/HTTP JSON logs](https://opentelemetry.io/docs/specs/otlp/).
Configure these Vault keys:

```bash
matrix config set system.logging.format json
matrix config set system.logging.collector.endpoint https://collector.example.org/v1/logs
```

If needed, set `system.logging.collector.authorization` privately.
`system.logging.collector.queue_size` is optional (16–4096).
The configured local log sink stays primary.

Only selected event names, labels and allowed counters/durations are exported.
Prompts, transcripts, raw messages and arbitrary attributes are excluded.
HTTPS is required except explicit loopback; URL credentials, query/fragment
and redirects are refused. Queue, batch, timeout and shutdown are bounded;
a slow network does not block the producer. Partial OTLP rejection is counted.

Authenticated `GET /_matrix/telemetry` shows acknowledgements, drops, privacy
filters, failed batches, queue and last error code without credentials.
This exports operational logs, not OTLP spans/metrics or billing integration.
Disabled export opens no connections and starts no worker.
