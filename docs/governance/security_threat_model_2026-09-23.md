# Matrix Security Threat Model — 2026-09-23

This is the first written threat model for Matrix. It is a read-only code review, not
a penetration test and not a claim that the code is secure. Prior release evidence
records the same gap it fills: no human security review has been performed
(`docs/governance/security_review_2026-09-22.md:110-123`,
`docs/governance/releases/2026-09-23-v0.1.37.md:110-111`).

## How this document was produced

- Reviewed revision: git `d093ef1` (`docs(issues): scope the ACP v2 authentication
  migration`), 2026-09-23, plus the uncommitted working-tree changes present at review
  time. The checkout is shared with other sessions
  (`docs/governance/concurrent_workstations.md`), so line numbers can drift; the
  function or symbol is named next to each citation so a stale line number can be
  re-resolved.
- Method: read the security-relevant code paths and cite them. No dynamic testing, no
  fuzzing, no dependency or cryptographic review beyond reading the parameters in use.
- Each threat records its state at the reviewed revision: "Mitigated" means a
  control exists in code and is active in the default configuration, "Partially
  mitigated" means a control exists but something is left open, and "Not mitigated"
  means no control was found. Where the state depends on operator configuration, the
  configuration is named.
- Out of scope: provider SDK internals (`pkg/zedacp*`, `a2a-go`), third-party
  vulnerabilities, the Telegram wire protocol, and Windows behaviour that cannot be
  observed from this workstation (see Unknowns).

Default deployment posture assumed throughout is the one the code ships: both listeners
on loopback (`cmd/matrix/constants.go:5-6`), no API keys, Telegram disabled, agent
permission requests denied by default. Appendix A lists the defaults.

## 1. Assets

| Asset | Where it lives | Why it matters |
| --- | --- | --- |
| Vault database | `$MATRIX_HOME/data/matrix-vault.db` (`cmd/matrix/constants.go:4`), single bbolt bucket `matrix_vault` (`internal/providers/bolt/bolt.go:21`) | Contains every persisted secret and all session state |
| Agent environment overrides and endpoint headers | Vault keys `agent.config.<id>` → `Override.Env`, `Config.Headers` (`internal/logic/agentcfg/store.go:15,18-39`) | Provider API keys and A2A credentials; injected into agent processes and outbound requests |
| Agent metadata | Vault keys `agent.meta.<id>`, including artifact verification evidence (`internal/logic/agentcfg/meta.go:15,42-50`) | Describes what was installed and whether it was verified |
| Configuration values | Vault keys `config.*` (`internal/logic/config/manager.go:11,27-58`) | `matrix_api_key`, `daemon_api_key`, Telegram token, channel allowlists |
| Vault master key | `$MATRIX_HOME/configs/vault-master.key` (`internal/logic/vaultsec/crypto.go:21,98-104`), or `MATRIX_VAULT_MASTER_KEY` / `MATRIX_VAULT_MASTER_KEY_FILE` (`internal/logic/vaultsec/crypto.go:32-49`) | Decrypts every vault value; no rotation or escrow exists |
| Runtime HTTP API key and A2A inbound key | One value, `config.matrix_api_key`, read at startup (`cmd/matrix/run.go:135,158-169`) | The only credential in front of the local HTTP surface |
| Runtime broker token | `$MATRIX_HOME/data/runtime-broker.json`, 32 random bytes hex-encoded (`internal/logic/runtimebroker/descriptor.go:40-60`; written `0600` at `:68`) | Authenticates CLI access to the daemon's vault storage |
| Installed agents and downloaded artifacts | `$MATRIX_HOME/agents/<id>` (`internal/logic/agentmgr/installer.go:209-212`), temp download (`:190`) | Executable code the runtime launches as the operator |
| Event sinks and run traces | Vault keys under the `runtrace` prefix (`internal/logic/runtrace/sinks.go:22-44`), registered over HTTP (`internal/providers/runapi/sinks.go:11-43`) | Sinks push run content to operator-configured URLs |
| Logs | `$MATRIX_HOME/logs`, files created `0600` (`internal/providers/oslog/sink.go:57`) | Prompts, agent stderr, paths, possibly secrets |
| Backups | `CreateBackup` destination, `0600` (`internal/logic/vaultsec/vaultsec.go:94-129`) | Copy of the encrypted vault plus whatever was plaintext in it |
| Operator shell configuration | `~/.profile`, `~/.zshrc` appended by the installer (`install/install.sh:218-243`); user `PATH` (`install/install.ps1:139-157`) | Persistence point outside the PAL home |

## 2. Trust boundaries

| Boundary | Data crossing | Direction | Controls at the boundary |
| --- | --- | --- | --- |
| ACP registry index and release artifacts (`https://cdn.agentclientprotocol.com/...`, `internal/logic/agentmgr/registry_client.go:97`) | Agent IDs, package names, `cmd`, archive URLs, `sha256` | In | Digest gate before extraction, path containment, hardened extraction (section 4) |
| Remote A2A agent cards and endpoints | Card documents, task traffic, governed headers | In and out | Header injection by config only; SSRF guard on outbound event sinks, none on agent endpoints |
| ACP agent processes (local children) | Prompts, tool-call results, stdout/stderr, file and terminal requests | Both | Permission requests denied by default (`internal/providers/agents/default_handler.go:31-32,158-168`); full process environment inherited |
| Runtime HTTP API on `127.0.0.1:9091` | Session, workspace, run, elicitation, agent-auth requests | In | Optional `X-Matrix-Key` / bearer key (`internal/providers/matrixapi/security.go:9-19`); loopback-only CORS (`internal/providers/matrixapi/cors.go:17-35`) |
| A2A ingress on the same listener | A2A JSON-RPC and REST task traffic | In | Same optional key (`internal/providers/a2a/server_config.go:43-44,65-76`); agent card served unauthenticated (`:45`) |
| JSON-RPC daemon on `127.0.0.1:9090` | `Vault.Get/Set`, `Storage.*`, `Auth.*` | In | Optional `daemon_api_key` for `Vault.*` (`internal/logic/daemon/vault_service.go:51-58`); mandatory random broker token for `Storage.*` (`internal/logic/daemon/storage_service.go:68-71`) |
| Local filesystem / PAL home | Vault, key file, configs, logs, agents | Both | `0700` directories (`internal/logic/matrixhome/home.go:56-70`), `0600` files, POSIX or Windows ACL |
| Operator shell | Installer-written profile snippets and `PATH` | Out | Operator review only |
| Telegram (when enabled) | Inbound messages and callbacks from arbitrary Telegram users | In | None enforced; the configured `admins` list is not consulted |
| Agent → control plane (meta-agent tools) | Tool-call name and arguments produced by an agent | In | None; calls are executed without an advertised-tool or argument check |
| Remote A2A catalogs (`onboarding.discovery.a2a_catalog_urls`, `cmd/matrix/run.go:71`) | Agent records proposed for registration | In | `RegisterRemote` clears command, args, env and env isolation (`internal/logic/agentcatalog/service.go:157-170`) |
| GitHub release metadata and assets (installers) | `checksums.txt`, archives, installers | In | HTTPS to `api.github.com`, mandatory checksum match, archive path/symlink validation |

## 3. Threats

Severity here reflects impact times reachability in the default configuration. The gap
IDs refer to section 5.

### 3.1 Spoofing

| ID | Scenario | Attacker position | Impact | State |
| --- | --- | --- | --- | --- |
| S1 | A local process presents the shared API key (or no key at all when none is set) and is treated as the operator by both the HTTP API and A2A ingress | Any process running as the same user | Full runtime use: run prompts, read workspace and session state, register sinks, answer elicitations | Not mitigated by default (G1); single shared secret, no per-caller identity (`internal/providers/a2a/server_config.go:48-53`) |
| S2 | A spoofed or compromised registry index supplies the package name, `cmd`, archive URL and digest that Matrix trusts | Control of the registry CDN, or of the registry publication path | Chosen code executes as the operator | Partially mitigated: a published digest is enforced, but publication is not required (G5, G6, G7) |
| S3 | Any Telegram user messages the bot and is treated as the operator | Anyone who can reach the bot | Prompt injection into the operator's agents, with the agent's answers returned to the attacker's chat | Not mitigated when the channel is enabled (G3) |

### 3.2 Tampering

| ID | Scenario | Attacker position | Impact | State |
| --- | --- | --- | --- | --- |
| T1 | A tampered archive is installed because the index publishes no digest for the platform | Registry or CDN | Arbitrary files under `$MATRIX_HOME/agents/<id>`; later execution as the operator | Refused only when a digest is published (`internal/logic/agentmgr/artifact_integrity.go:46-56`); "digest_not_published" installs proceed (G5) |
| T2 | The index points `cmd` at an arbitrary path (absolute, or `../..`), which the launcher then executes | Registry or CDN | Execution of a local binary of the attacker's choice with the daemon environment | Not mitigated: only local relative paths are joined to the agent directory (`internal/logic/agentmgr/installer.go:178-183`) (G6) |
| T3 | Archive entries escape the destination through `..`, absolute paths or symlinks | Whoever supplies the archive | Writes outside the agent directory | Mitigated: escaping entries are skipped, symlinked destinations rejected, modes forced to `0644`/`0755` (`internal/providers/osfs/archive.go:62-75,88-91,194-197,241-247,305-326`). Escaping entries are skipped rather than failing the install, so a partial agent can still be registered |
| T4 | A ciphertext is moved from one vault key to another, because values are not bound to their key | Write access to the vault file | Value substitution (for example planting a chosen value under `config.telegram.token`) is not detectable | Not mitigated; AEAD associated data is `nil` (`internal/logic/vaultsec/crypto.go:196`) and the ciphertext is stored under whatever key the caller passes (`internal/providers/bolt/bolt.go:177`) (G12). Requires write access, which in a single-user install implies read access |
| T5 | An agent returns a tool call that was never advertised, or with a path argument, and Matrix executes it | Any agent that can return a tool call | Arbitrary file write as the operator through `Config_Set` | Not mitigated: tool calls are projected without validation (`internal/providers/agents/acp_adapter.go:178,716-732`) and executed on every routed turn (`internal/logic/session/manager_routing.go:81`), although the schemas are only advertised for `/action` turns (`internal/logic/session/chat_commands.go:147-155`), and the path is model-supplied (`internal/logic/system_tools/handlers.go:104-115`, `internal/providers/osfs/config.go:15-24`) (G4) |
| T6 | A rewritten config file keeps broader permissions than `0600` | Same user, after an older Matrix or the operator created the file world-readable | Config values readable by other local users | Partially mitigated: `os.WriteFile` applies the mode only at creation (`internal/providers/osfs/config.go:20-24`) (G14) |

### 3.3 Information disclosure

| ID | Scenario | Attacker position | Impact | State |
| --- | --- | --- | --- | --- |
| I1 | `Vault.Get` is called over the JSON-RPC daemon with no key configured | Any same-user local process | Decrypted disclosure of every vault value, including all API keys and agent credentials (`internal/logic/daemon/vault_service.go:31-58`) | Not mitigated by default (G2) |
| I2 | The HTTP API is read without a key: run resources, workspace state, timeline, decisions, memory, orchestration capabilities | Any same-user local process | Session content and workspace data disclosure (`internal/providers/matrixapi/server.go:179-599`, `internal/providers/runapi/types.go:153-160`) | Not mitigated by default (G1) |
| I3 | A downloaded agent reads vault material from its own environment or from the key file | Installed agent code | The vault key is readable by the same UID; `MATRIX_VAULT_MASTER_KEY` is inherited by children when supplied via environment (`pkg/zedacpstdio/transport.go:43`, `internal/providers/exec/exec_unixlike.go:58`, source at `internal/logic/vaultsec/crypto.go:32-49`) | Partially mitigated: file permissions stop other users, not same-UID processes (G8) |
| I4 | Another local user reads the downloaded artifact or squats the predictable temp path | Any local user on a multi-user host | Artifact disclosure (public release content), or a write primitive against files the victim can write | Not mitigated (`internal/providers/network/tcp_provider.go:87`, `internal/providers/osfs/fs.go:59`, `internal/logic/agentinstall/path.go:35`) (G9) |
| I5 | Governed headers follow a cross-host redirect or travel over plain HTTP | Remote A2A endpoint, or an on-path attacker for `http://` endpoints | Credential disclosure to a third host in cleartext | Not mitigated: the resolver uses the default client with no redirect policy (`internal/providers/a2aclient/adapter.go:38-40,71-84`) and the scheme is not constrained (`:72-76`) (G13) |
| I6 | A secret in agent output reaches the log file | Agent whose output the operator logs | Secret persistence in logs | Partially mitigated: pattern-based sanitizer for bearer tokens, known prefixes and `KEY=value` (`internal/logic/providerdiag/stderr.go:14-19,85-101`), `0600` log files. Values that match no pattern are logged verbatim |

### 3.4 Privilege escalation

| ID | Scenario | Attacker position | Impact | State |
| --- | --- | --- | --- | --- |
| P1 | A local process, including an agent Matrix itself launched, uses the unauthenticated API and vault RPC | Same-user process | Control of the runtime and disclosure of all secrets; no privilege boundary exists between the runtime and the code it runs | Not mitigated by default (G1, G2, G8) |
| P2 | A Telegram stranger drives the meta-agent, including `/action` | Remote, unauthenticated in Matrix terms | Agent turns with the operator's provider credentials; `/action` additionally exposes `APM_Install`, `APM_Uninstall` and `Config_Set` | Not mitigated when the channel is enabled (G3, G4) |
| P3 | The registry delivers code that runs as the operator at install or launch time | Registry or CDN | Code execution outside any sandbox | Partially mitigated: digest gate when published, hardened extraction, `npx`/`uvx` install path unrestricted (G5, G6, G7) |
| P4 | A prompt-injected legitimate agent returns a tool call | Anyone able to influence agent input (workspace file, A2A peer, Telegram) | Arbitrary file write on the operator's filesystem | Not mitigated (G4) |

### 3.5 Denial of service

| ID | Scenario | Attacker position | Impact | State |
| --- | --- | --- | --- | --- |
| D1 | Archive expands far beyond its compressed size | Whoever supplies the archive | Disk exhaustion | Mitigated: shared 2 GiB budget (`internal/providers/osfs/archive.go:39-60`) |
| D2 | A connection opens without finishing headers | Local process | Worker starvation | Mitigated: `ReadHeaderTimeout` 10s, `IdleTimeout` 60s (`cmd/matrix/run.go:219-220`) |
| D3 | The predictable temp archive path is pre-created as a symlink or as a file the daemon cannot replace | Any local user | Failed installs, or truncation of a victim-writable file | Not mitigated (G9) |
| D4 | Oversized request bodies: only the elicitation route bounds the body | Local process | Memory growth in the daemon | Partially mitigated: `http.MaxBytesReader` on elicitations only (`internal/providers/runapi/elicitations.go:66`); all other routes decode `r.Body` directly. Loopback-only by default, so low impact |
| D5 | An agent crashes in a loop | Installed agent | Repeated restarts | Mitigated: five fast crashes in a five-second window stop the supervisor (`internal/logic/agentmgr/supervisor.go:17-18,256-274`) |

### 3.6 Supply chain

| ID | Scenario | Attacker position | Impact | State |
| --- | --- | --- | --- | --- |
| SC1 | A hostile registry entry supplies package name, `cmd` and (optionally) no digest | Registry or CDN | Code of the attacker's choice installed and launched | Partially mitigated (G5, G6) |
| SC2 | `npx -y <package>` and `uvx <package>` resolve whatever the registry names at launch, with npm/PyPI resolution and lifecycle scripts | Registry, plus any npm/PyPI namespace takeover | Code execution at agent start; no version or integrity pinning in Matrix | Not mitigated (G7) |
| SC3 | A compromised GitHub release ships a poisoned archive with a matching `checksums.txt` | GitHub account or release pipeline | Arbitrary binary installed by `install.sh` / `install.ps1` | Partially mitigated: mandatory checksum match and archive validation, but the checksum file is fetched from the same origin and is not signed (`install/install.sh:95-136,138-172`, `install/install.ps1:64-113`) |
| SC4 | A previously fetched index is replayed from the vault cache for up to an hour, or indefinitely on network failure during discovery | Registry or CDN | Stale-but-trusted agent metadata in listings | Bounded: installs call `FetchManifest`, which fetches fresh (`internal/logic/agentmgr/registry_client.go:180-186`); discovery may use the cache (`:149-177`) |

## 4. Current mitigations, with evidence

| Control | What it does | Evidence | Caveat |
| --- | --- | --- | --- |
| Vault value encryption | AES-256-GCM with a random 96-bit nonce per value, `ENCV1:` prefix, base64 payload; encrypt on `Set`, decrypt on `Get` | `internal/logic/vaultsec/crypto.go:174-234`, `internal/providers/bolt/bolt.go:141-151,154-166` | Key names are not authenticated (G12); values written before a key existed stay plaintext until `matrix vault seal` (`internal/logic/vaultsec/seal.go:6-23`) |
| Vault writes fail closed without a key | `Set` refuses when no master key is configured | `internal/logic/vaultsec/crypto.go:180-182`, `internal/providers/bolt/bolt.go:158-166` | Reads of legacy plaintext values succeed with no key (`internal/logic/vaultsec/crypto.go:203-206`) |
| Master key generation and storage | 32 random bytes, base64, written `O_CREATE|O_EXCL` at `0600` in a `0700` directory; never overwritten | `internal/logic/vaultsec/crypto.go:69-96,134-172` | Created by the first CLI invocation (`cmd/matrix/root.go:14`, `cmd/matrix/home.go:16-31`), so the key sits next to the vault it protects |
| Master key source validation | Accepts only 32 bytes as base64 or hex; refuses key files whose mode is not exactly `0600` | `internal/logic/vaultsec/crypto.go:106-132,241-249`, `internal/logic/vaultsec/permissions_unix.go:20-26` | Stricter modes such as `0400` are also refused, so hardening the file breaks startup |
| Vault file permissions | `bbolt.Open(..., 0600)` plus an explicit `chmod 0600` on every write-capable open; PAL home directories `0700` | `internal/providers/bolt/bolt.go:55,88`, `internal/logic/matrixhome/home.go:56-70` | Read-only opens do not re-apply the mode |
| Windows ACL hardening | `icacls /inheritance:r` removes Everyone, Authenticated Users and Builtin Users, grants the current user full control | `internal/logic/vaultsec/permissions_windows.go:13-29` | Not executed in this review |
| Plaintext-vault detection | The vault report counts encrypted and plaintext entries and warns with the exact remediation command | `internal/providers/bolt/bolt.go:34-52`, `internal/logic/vaultsec/vaultsec.go:51-74` | Operator-visible only |
| Backup permissions | Backups are created `O_EXCL` `0600` and chmod-ed | `internal/logic/vaultsec/vaultsec.go:94-129` | Backup contents inherit any legacy plaintext |
| Runtime broker token | 32 random bytes; the token is required for every `Storage.*` call and the descriptor is written `0600` and removed at shutdown | `internal/logic/runtimebroker/descriptor.go:17-24,44-60,62-86`, `internal/logic/daemon/storage_service.go:22-71`, `internal/logic/daemon/server_broker.go:37-53` | Token is per-run; the same daemon's `Vault.*` service does not use it (G2) |
| Runtime API authorization | `X-Matrix-Key` or `Authorization: Bearer`, applied by every HTTP route when a key is configured | `internal/providers/matrixapi/security.go:9-19`, `internal/providers/runapi/helpers.go:22-42`, `internal/providers/runapi/agent_auth.go:36-39`, `cmd/matrix/run.go:271-295` | Empty key means "allow" (G1) |
| External bind guard | Refuses to start when a listener is not loopback and its key is unset | `internal/logic/runtimecheck/bind.go:9-27`, `cmd/matrix/run.go:137-144` | Loopback without a key is explicitly allowed |
| A2A ingress control | The same key gates `/a2a` and `/a2a/rest`; the card advertises the scheme only when a key is set; JSON content type required on JSON-RPC | `internal/providers/a2a/server_config.go:30-46,65-87,133-145` | Card endpoint intentionally unauthenticated; single shared key, no per-caller identity |
| CORS containment | Only loopback `http://` origins receive CORS headers; other origins get `403` | `internal/providers/matrixapi/cors.go:17-67` | Requests without an `Origin` are unaffected, which is the local-process case |
| Artifact digest gate before extraction | The downloaded archive is hashed and compared with the registry's published digest; extraction starts only after the match; malformed digests and mismatches refuse the install and leave no partial install | `internal/logic/agentmgr/artifact_integrity.go:32-96`, `internal/logic/agentmgr/installer.go:189-217` | An unpublished digest is not a failure (G5) |
| Install-time verification evidence | The outcome is recorded in vault metadata as `verified`, `digest_not_published` or `not_applicable`, and surfaced by `matrix doctor`, `matrix agent doctor` and `matrix agent info` | `internal/logic/agentcfg/meta.go:21-68,88-106`, `internal/logic/agentmgr/installer.go:118-132`, `cmd/matrix/agent_doctor.go:104-106`, `cmd/matrix/agent_info.go:86-87` | "Not published" is reported, not blocked |
| Hardened archive extraction | 2 GiB budget, path-traversal rejection, symlink-escape rejection, non-regular entries skipped, setuid/sticky stripped, modes forced to `0644`/`0755` | `internal/providers/osfs/archive.go:39-60,62-75,77-145,191-229,241-326` | Escaping entries are skipped silently rather than failing the install |
| Install path containment | Agent IDs and versions are validated as safe path tokens and the resulting path is verified to stay under the install root | `internal/logic/agentinstall/path.go:9-49` | Does not constrain `cmd` from the registry (G6) |
| Installer checksum verification | Exactly one platform asset and exactly one `checksums.txt` are required; the archive's sha256 must match its entry; the archive is validated for traversal and symlinks before extraction; only `matrix` at archive root is installed, `0755`; seeded configs are `0600` | `install/install.sh:84-136,138-172,189-206,245-257`, `install/install.ps1:47-62,64-113,118-129,159-169` | Checksum and asset come from the same unsigned release |
| Installer stale-index fallback | When the by-tag release document reports no assets, the id-addressed endpoint is used instead of failing | `install/install.sh:64-82`, `install/install.ps1:33-45` | Availability control, not a security control |
| Telegram token cannot live in the seed file | A non-empty token in `configs/telegram.json` is a fatal error; the token comes from the vault, an override file or the environment | `internal/logic/channelcfg/telegram.go:91-120` | The `admins` list is loaded but never enforced (G3) |
| Agent permission default | ACP permission requests are denied unless `agent.trust_mode` is `true` | `internal/providers/agents/default_handler.go:31-32,158-168`, `cmd/matrix/run.go:91-93` | Agent file and terminal requests are handled by the same handler |
| Event sink SSRF guard | Sinks must be absolute `http`/`https`, must not resolve to loopback, private or link-local addresses; each dial re-checks resolved addresses and each redirect is revalidated | `internal/logic/runtrace/sinks.go:46-64`, `internal/providers/runsink/delivery.go:45-113` | Applies to sinks only, not to A2A agent endpoints |
| Log and stderr handling | Log files `0600`; agent stderr is bounded, home paths collapsed, bearer tokens and known key patterns redacted | `internal/providers/oslog/sink.go:57`, `internal/logic/providerdiag/stderr.go:14-19,85-101` | Heuristic, not value-based (I6) |
| Runtime report carries no secrets | The report exposes paths, addresses and booleans, and whether Telegram is configured, never the values | `internal/logic/runtimecheck/report.go:24-53` | The endpoint is unauthenticated when no key is set |
| Agent crash-loop cap | Five fast crashes in the window stop the supervisor and record `crash_loop` | `internal/logic/agentmgr/supervisor.go:17-18,256-274` | Availability control |
| HTTP server timeouts | `ReadHeaderTimeout` 10s, `IdleTimeout` 60s, plus a 5s shutdown budget | `cmd/matrix/run.go:216-224,235-241` | No write timeout or body limit (D4) |

## 5. Gaps

Ordered by severity. Each entry states what is missing, the evidence that it is
missing, and one concrete recommendation.

### G1 — High — The runtime HTTP API and A2A ingress are unauthenticated by default

- Missing: authentication on the local HTTP surface when `matrix_api_key` is unset,
  which is the shipped default. The key is optional (`cmd/matrix/run.go:135,158-169`),
  the external-bind guard permits loopback without one
  (`internal/logic/runtimecheck/bind.go:15-18`), and every route treats an empty
  configured key as "allow" (`internal/providers/matrixapi/security.go:9-14`,
  `internal/providers/runapi/helpers.go:22-25`,
  `internal/providers/a2a/server_config.go:70-73`, `cmd/matrix/run.go:275-279`). The
  code states the assumption explicitly: "The local API is reachable by any process on
  the machine" (`cmd/matrix/run.go:216-218`).
- Impact: any same-user process — including every ACP agent Matrix launches — can
  start runs with the operator's provider credentials, read session and workspace
  state, and register event sinks; when elicitation is enabled (it is off by default,
  `cmd/matrix/run.go:316-321`), it can also answer the prompts that gate agent actions.
- Recommendation: generate a per-install API key on first run, the way the master key
  and broker token are generated, and require it even on loopback; keep an explicit
  `--no-auth-loopback` opt-out for development. A Unix domain socket with `0600` is
  an equivalent alternative on POSIX.

### G2 — High — The JSON-RPC daemon returns decrypted vault values without authentication by default

- Missing: an authentication requirement on `Vault.Get` and `Vault.Set` when
  `daemon_api_key` is unset. `VaultService.authorize` returns `nil` for an empty key
  (`internal/logic/daemon/vault_service.go:51-58`), the key is optional at startup
  (`cmd/matrix/run.go:196-200`), and the listener is loopback by default
  (`cmd/matrix/constants.go:5`). The broker token that protects `Storage.*`
  (`internal/logic/daemon/storage_service.go:68-71`) is not required here.
- Impact: one unauthenticated RPC call returns any vault value once its key is known,
  including `config.matrix_api_key`, `config.telegram.token`, agent environment
  overrides and endpoint headers.
- Recommendation: reuse the per-run broker token for `Vault.*` as well, or require the
  persisted API key unconditionally; never treat an empty key as authenticated, and
  make the daemon refuse to start with neither credential configured.

### G3 — High when the Telegram channel is enabled — The configured `admins` list is never enforced

- Missing: any sender check on inbound Telegram messages. The bot is constructed with
  only the token and the router (`internal/logic/channelruntime/runtime.go:77`,
  `internal/providers/telegram/bot.go:212`); the update handler processes every message
  and logs `user_id` without comparing it to anything
  (`internal/providers/telegram/bot_update.go:46-71`); the provider never references
  the admins list, which is only parsed (`internal/logic/channelcfg/telegram.go:16-52,122-190`)
  and displayed (`internal/logic/channelcfg/providers.go:96-104`).
- Impact: when the channel is enabled, any Telegram user who can reach the bot drives
  the operator's agents and receives their answers in the attacker's chat. With
  `/action` reachable, that includes `APM_Install`, `APM_Uninstall` and `Config_Set`
  (`internal/logic/system_tools/handlers.go:14-79`).
- Recommendation: pass the resolved admins list into `NewBot` and drop updates whose
  `From.ID` is not in it; refuse to enable the channel when the list is empty, and
  cover the rejection with a test in `internal/providers/telegram`.

### G4 — High — Agent-returned tool calls are executed without an authorization check

- Missing: a check that the turn advertised the tool and that the arguments are
  acceptable. Tool calls from the ACP response are projected verbatim
  (`internal/providers/agents/acp_adapter.go:178,716-732`) and executed on every
  routed turn (`internal/logic/session/manager_routing.go:81,130-137`), while the tool
  schemas are only advertised for `/action` turns
  (`internal/logic/session/chat_commands.go:140-161`). `Config_Set` writes the
  model-supplied key as a filesystem path with no containment
  (`internal/logic/system_tools/handlers.go:104-115`,
  `internal/providers/osfs/config.go:15-24`).
- Impact: an agent (including a downloaded one, or a legitimate one influenced by
  workspace content, an A2A peer or a Telegram message) can write any file the
  operator's user can write, or install and uninstall agents, outside the `/action`
  gate. Precondition: the agent returns a tool call, which the code does not validate.
- Recommendation: execute a tool call only when that turn advertised the tool; validate
  the tool name against the advertised set; and constrain `Config_Set` paths to
  `configs/` inside the PAL home, rejecting absolute paths and `..`.

### G5 — High — Binary installs proceed without integrity verification when the index omits `sha256`

- Missing: a requirement that a published digest exist before a binary distribution is
  installed. An empty digest produces `digest_not_published` evidence and the install
  continues (`internal/logic/agentmgr/artifact_integrity.go:46-50`), the field is
  optional in the index (`internal/logic/agentmgr/registry_client.go:62-67`), and the
  status is defined as "the install proceeds, but nothing was verified"
  (`internal/logic/agentcfg/meta.go:25-28`). Extraction happens after the check, so in
  this case it happens unchecked (`internal/logic/agentmgr/installer.go:203-214`).
- Impact: a compromised or spoofed registry index installs arbitrary content under
  `$MATRIX_HOME/agents/<id>`, which the runtime then launches as the operator.
- Recommendation: refuse binary installs without a published digest unless the operator
  passes an explicit flag (for example `matrix install --allow-unverified`), and record
  the override, its reason and the operator identity in the install evidence.

### G6 — High — The registry index controls the executable path launched as the agent

- Missing: containment of `cmd`. The value is taken from the index
  (`internal/logic/agentmgr/registry_client.go:57-67`), and only local relative paths
  (or `./`-prefixed paths) are joined to the agent directory; absolute paths and paths
  containing `..` are used as-is (`internal/logic/agentmgr/installer.go:178-183`). The
  result becomes the agent command (`:135-138`) that the runtime launches
  (`internal/logic/agentmgr/supervisor.go:201`,
  `internal/providers/agents/acp_transport.go:30`). The archive digest does not cover
  `cmd`, because it is index metadata, not archive content.
- Impact: a hostile index makes Matrix execute a binary of its choice, from anywhere on
  the filesystem, with the daemon's environment and the operator's privileges.
- Recommendation: require `cmd` to resolve inside the extracted agent directory
  (`filepath.IsLocal` plus a `filepath.Rel` containment check), reject absolute paths
  and traversal, and verify the resolved file exists under `agentPath` before
  registering the agent.

### G7 — Medium — npx, uvx and npm distributions have no version or integrity pinning

- Missing: any pinning or script policy for non-binary distributions. The registry
  supplies the package name, which becomes `npx -y <package>` or `uvx <package>`
  (`internal/logic/agentmgr/registry_client.go:220-253`), is registered without
  verification evidence (`internal/logic/agentmgr/installer.go:140-149`), and for the
  canonical Codex package is installed with
  `npm install --prefix ... --no-audit --no-fund` and no `--ignore-scripts`
  (`internal/logic/agentinstall/codex.go:81-96`). The only package rejected is the
  retired Codex provider (`internal/logic/agentidentity/codex.go:39-44`).
- Impact: the registry, an npm/PyPI namespace takeover, or a later package release
  decides which code runs when the agent starts; there is no digest gate because no
  artifact is downloaded by Matrix.
- Recommendation: pin the canonical Codex package with a lockfile carrying integrity
  hashes and install with `--ignore-scripts`, running only the known entrypoint; treat
  registry-provided package names as untrusted and require explicit operator
  confirmation (or an allowlist) before registering npx/uvx distributions.

### G8 — Medium — Agent processes are not isolated from vault material

- Missing: a boundary between the runtime's secrets and the code it launches. Children
  inherit the full daemon environment (`pkg/zedacpstdio/transport.go:43`,
  `internal/providers/exec/exec_unixlike.go:58`), so `MATRIX_VAULT_MASTER_KEY` is
  inherited when the key is supplied that way
  (`internal/logic/vaultsec/crypto.go:32-49`); in every configuration the key file is
  readable by the same UID (`$MATRIX_HOME/configs/vault-master.key`, mode `0600`), and
  the vault itself is readable and writable by that UID. The permission model protects
  against other users, lost backups and a stolen disk image, not against a process
  running as the operator.
- Impact: an installed agent can read every secret in the vault directly, and the
  environment path additionally exposes the key to any same-user process that can read
  `/proc/<daemon-pid>/environ`.
- Recommendation: state plainly in `SECURITY.md` and the architecture docs that agents
  run with the operator's privileges and are trusted at that level; stop passing vault
  key material through the process environment (read the key file in-process only and
  scrub `MATRIX_VAULT_*` from child environments); if agent isolation is a goal, it
  needs an OS-level sandbox, not file permissions.

### G9 — Medium — Artifacts are downloaded to a shared temp directory under a predictable name

- Missing: a private, exclusive, symlink-safe download destination. The installer asks
  the filesystem provider for its temp directory
  (`internal/logic/agentmgr/installer.go:190`), which is `os.TempDir()`
  (`internal/providers/osfs/fs.go:58-60`), and the name is deterministic
  (`matrix-agent-<id>-<version><ext>`, `internal/logic/agentinstall/path.go:22-41`).
  The download then uses `os.Create` (`internal/providers/network/tcp_provider.go:87`),
  which follows symlinks, truncates, and creates the file with the process umask
  (typically world-readable in `/tmp`).
- Impact: another local user on a multi-user host can pre-create the path as a symlink
  and have the daemon truncate a file the victim can write, or read the downloaded
  artifact. It is tamper and denial of service against the victim, not code execution
  from the attacker's own content, because the bytes written are the verified release
  artifact.
- Recommendation: download into `$MATRIX_HOME/tmp` (`0700`) with
  `O_CREATE|O_EXCL|O_WRONLY` and mode `0600`, and refuse a destination that already
  exists or is a symlink.

### G10 — Medium — The env-isolation launcher interpolates arguments into a shell command

- Missing: shell-safe argument handling. With `env_isolation` enabled, command and
  arguments are rendered into a `bash -c` string using Go's `%q`
  (`internal/logic/agentlaunch/stdio_unix.go:10-21`, used by
  `internal/providers/agents/acp_transport.go:30` and
  `internal/providers/agentprobe/acp.go:15`; the same pattern appears for probes in
  `internal/providers/exec/exec_unixlike.go:48-53`). `%q` escapes quotes and
  backslashes but not `$`, backticks or `${...}`, which bash still expands inside
  double quotes, so an argument containing `$(...)` or a backtick command is executed
  before the agent starts.
- Reachability today: `env_isolation` is set only by the seeded
  `configs/agents.json:8,19,41` and by operator override; registry-installed and
  remotely registered agents get it cleared
  (`internal/logic/agentmgr/installer.go:135-149`,
  `internal/logic/agentcatalog/service.go:167`). This is a latent injection sink, not
  an exploited path: it becomes remotely reachable the moment any registry-, catalog-
  or API-supplied value reaches `Args` for an env-isolation agent.
- Recommendation: stop routing the launch through a shell. Resolve the interpreter or
  NVM-managed binary path in Go and use `exec.CommandContext` with an argument vector;
  where a shell is unavoidable, single-quote arguments and reject shell metacharacters
  rather than relying on `%q`.

### G11 — Low — `/v1/auth/openrouter/callback` is the only route without the API-key check

- Missing: consistency with the rest of the surface. The handler reads `code` and
  `state` and calls the router without `requireAPIKey`
  (`internal/providers/matrixapi/server.go:151-176`, registered at `:599`), while every
  other route in the same file checks the key (for example `:187`).
- Why it is not an authentication bypass: the `state` value must carry an HMAC that
  matches the stored PKCE verifier before the code exchange happens
  (`internal/logic/onboarding/wizard_openrouter.go:98-140,226-240`). Residual risk: an
  unauthenticated caller can trigger an outbound token exchange with an
  attacker-supplied code and learn whether the daemon is in a quick-login flow.
- Recommendation: keep the route unauthenticated (a browser redirect cannot carry the
  key) but document that its authorization is the CSRF state, add a rate limit, and pin
  the CSRF requirement with a test that fails if the check is removed.

### G12 — Low — Vault values are not bound to their vault key

- Missing: AEAD associated data. Values are sealed with `nil` additional data
  (`internal/logic/vaultsec/crypto.go:196`) and stored under whatever key the caller
  passes (`internal/providers/bolt/bolt.go:177`), so ciphertext can be moved between
  keys without detection — for example planting a known value under
  `config.telegram.token`.
- Impact: low, because write access to the vault file in a single-user install implies
  read access; the control matters if permissions are ever relaxed or a vault file is
  partially exposed.
- Recommendation: pass the vault key (for example `config.telegram.token`) as the GCM
  associated data on seal and open, so a moved ciphertext fails authentication.

### G13 — Low — A2A governed headers follow cross-host redirects and may travel in cleartext

- Missing: a redirect policy and a transport requirement for endpoints that receive
  credentials. Configured headers are attached to the request interceptor
  (`internal/providers/a2aclient/adapter.go:38-40`) and to the card resolver
  (`:71-84`), which uses the SDK default client, so Go's default redirect policy
  applies: on a cross-domain redirect `net/http` copies the initial request's headers
  except the six it treats as sensitive (`Authorization`, `Www-Authenticate`, `Cookie`,
  `Cookie2`, `Proxy-Authorization`, `Proxy-Authenticate`, Go 1.27
  `net/http/client.go`, `makeHeadersCopier`). A custom header such as `X-API-Key` is
  therefore forwarded to a redirected host.
  The scheme is only checked for presence (`:72-76`), so an `http://` endpoint sends the
  header in cleartext.
- Recommendation: resolve cards with a client whose `CheckRedirect` refuses a host
  change, and require `https` for endpoints that carry governed headers, with a clear
  error (or an explicit opt-in) for plain HTTP.

### G14 — Low — Config writes do not tighten the permissions of an existing file

- Missing: an explicit `chmod` after writing configuration. `WriteConfig` uses
  `os.WriteFile(path, data, 0o600)` (`internal/providers/osfs/config.go:20-24`), whose
  mode argument applies only when the file is created; a file left `0644` by an older
  version or by the operator stays `0644` after being rewritten. This is the same class
  of issue the 2026-09-22 review reported as fixed
  (`docs/governance/security_review_2026-09-22.md:50-53`).
- Recommendation: open with `O_CREATE|O_TRUNC|O_WRONLY` and call `Chmod` (or `ApplySecurePermissions`)
  after a successful write, and add a test that rewrites a `0644` file and asserts
  `0600`.

## 6. Documentation claims the code contradicts

These are documentation defects rather than code defects; they matter because they
shape what an operator believes is protected.

| Claim | Where it is claimed | What the code does |
| --- | --- | --- |
| The Telegram `admins` list controls who can use the bot ("Make sure your Telegram user ID is in the admin list") | `docs/wiki/FAQ.md:184-187`, `docs/wiki/Channels.md:8-27`, `docs/wiki/CLI-Reference.md:350` | The list is parsed, merged and displayed; it is never passed to the bot or checked in the update path (`internal/logic/channelruntime/runtime.go:77`, `internal/providers/telegram/bot_update.go:46-71`). See G3 |
| "The calls use `exec.CommandContext(executable, args...)` with no shell, so no interpolation occurs" (accepted `G204` risk) | `docs/governance/security_review_2026-09-22.md:70-73` | True for direct launches, false for the env-isolation path, which runs `bash -c` with `%q`-quoted arguments (`internal/logic/agentlaunch/stdio_unix.go:10-21`). See G10 |
| "An operator who can choose the path can already read the file" (accepted `G304` risk) | `docs/governance/security_review_2026-09-22.md:67-69` | For `Config_Set` the path is chosen by an agent's tool call, not by the operator (`internal/logic/system_tools/handlers.go:112`), and the write is not constrained to `configs/`. See G4 |
| Config writes are now `0600` (finding 5, "fixed") | `docs/governance/security_review_2026-09-22.md:50-53` | New files are; existing files keep their previous mode because `os.WriteFile` does not chmod (`internal/providers/osfs/config.go:20-24`). See G14 |
| "`install.sh` still reads the by-tag endpoint and has the same exposure as the old `install.ps1`. Not changed here" | `docs/governance/releases/2026-09-23-v0.1.37.md:41-44,102-105` | `install/install.sh:64-82` now implements the id-addressed fallback (commit `caa8fdf`). The release evidence is stale, in the operator's favour |

## 7. Unknowns

Places where this review could not determine the answer from the code, listed instead of
guessed.

1. Windows behaviour: `icacls` enforcement and the Windows ACL check
   (`internal/logic/vaultsec/permissions_windows.go`) were read but not executed. Whether
   the ACL check correctly identifies all broad principals on every Windows locale is
   unverified. The same applies to `install.ps1` beyond its guard tests.
2. The operator's actual configuration: whether `matrix_api_key` / `daemon_api_key` are
   set, whether either listener is bound off-loopback, and whether Telegram is enabled.
   The threat model describes the default posture and marks configuration-dependent
   items as such.
3. Real agent behaviour with respect to unadvertised tool calls (G4): the code does not
   check, and this review did not run an agent to observe whether conforming agents can
   emit tool calls for tools they were not offered.
4. Push notifications: the A2A server supports them
   (`internal/providers/a2a/server_config.go:19-23,36-38`) but the daemon does not wire
   them (`cmd/matrix/run.go:165-169`), so the push store and sender paths were not
   reviewed as live surface.
5. Whether `npx`/`uvx` receive or honour any integrity metadata: Matrix passes only the
   package name (`internal/logic/agentmgr/registry_client.go:220-253`), so any
   verification would be registry-side and is outside this repository.
6. Effectiveness of the pattern-based log sanitizer against real provider output; only
   the patterns could be read (`internal/logic/providerdiag/stderr.go:14-19`), not a
   corpus of agent stderr.
7. Dynamic behaviour under a hostile same-UID process: no adversarial testing was
   performed, so the analysis of G1, G2 and G8 rests on the code paths cited, not on
   observed access.

## Appendix A — Default posture

| Control | Default | Source |
| --- | --- | --- |
| HTTP API listener | `127.0.0.1:9091` | `cmd/matrix/constants.go:6` |
| JSON-RPC listener | `127.0.0.1:9090` | `cmd/matrix/constants.go:5` |
| `matrix_api_key` / A2A inbound key | empty (no authentication) | `cmd/matrix/run.go:135,158-169` |
| `daemon_api_key` | empty (no authentication for `Vault.*`) | `cmd/matrix/run.go:136,196-200` |
| Broker token for `Storage.*` | random per run, mandatory | `internal/logic/runtimebroker/descriptor.go:44-60`, `internal/logic/daemon/storage_service.go:68-71` |
| Vault encryption | on once the master key exists; the key is created by the first CLI invocation | `cmd/matrix/root.go:14`, `internal/logic/vaultsec/crypto.go:69-96` |
| Registry URL | fixed third-party CDN over HTTPS | `internal/logic/agentmgr/registry_client.go:97,111-114` |
| Telegram channel | disabled, no token | `configs/telegram.json:1-3`, `internal/logic/config/first_run.go:10` |
| Elicitation | disabled | `cmd/matrix/run.go:107-116` |
| A2A push notifications | not wired | `cmd/matrix/run.go:165-169` |
| Agent permission requests | denied unless `agent.trust_mode=true` | `internal/providers/agents/default_handler.go:158-168` |
| Event sink SSRF guard | on | `internal/logic/runtrace/sinks.go:46-64` |
| Archive extraction budget | 2 GiB | `internal/providers/osfs/archive.go:43` |
| HTTP header/idle timeouts | 10s / 60s | `cmd/matrix/run.go:219-220` |
