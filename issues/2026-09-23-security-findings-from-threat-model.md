# Security findings from the first threat model

Date observed: 2026-09-23
Source: `docs/governance/security_threat_model_2026-09-23.md` (14 gaps, each with
file:line evidence). This issue exists so the actionable ones are tracked as work
rather than living only inside a document.

## Bugs with a small, clear fix — do these first

**1. The Telegram `admins` list is never enforced.** Any Telegram user who can
reach the bot drives the operator's agents and receives their answers, and
`/action` exposes `APM_Install`, `APM_Uninstall` and `Config_Set`. The list is
parsed (`internal/logic/channelcfg/telegram.go:16-52,122-190`) and displayed
(`internal/logic/channelcfg/providers.go:96-104`) but never read by the provider:
`internal/providers/telegram/bot.go:212` takes only the token and the router, and
`internal/providers/telegram/bot_update.go:46-71` processes every update. The
documentation promises the opposite (`docs/wiki/FAQ.md:186`,
`docs/wiki/Channels.md:18`, `docs/wiki/CLI-Reference.md:350`).
*Fix*: pass the resolved list into `NewBot`, drop updates whose `From.ID` is not
in it, refuse to enable the channel when the list is empty, and cover the
rejection with a test in `internal/providers/telegram`.

**2. `Config_Set` writes a model-supplied filesystem path with no containment.**
`internal/logic/system_tools/handlers.go:112` takes the path from the tool call
and `internal/providers/osfs/config.go:23` writes it. The existing security review
accepted the G304 finding on the grounds that the path is "chosen by an operator"
(`docs/governance/security_review_2026-09-22.md:67-69`); an agent tool call is not
an operator. *Fix*: resolve and confine the path to the Matrix config directory,
and reject anything outside it.

**3. Agent-returned tool calls are executed without checking the turn advertised
the tool.** `internal/providers/agents/acp_adapter.go:178` and
`internal/logic/session/manager_routing.go:81` execute what the agent returns.
*Fix*: refuse a tool call the session did not advertise for that turn, and record
the refusal.

## Design decisions to take deliberately, not by default

**4. The local HTTP API and A2A ingress are unauthenticated when
`matrix_api_key` is unset** — the shipped default. An empty key means "allow"
(`internal/providers/matrixapi/security.go:9-14`,
`internal/providers/runapi/helpers.go:22-25`,
`internal/providers/a2a/server_config.go:70-73`), and only non-loopback binds are
refused without a key (`internal/logic/runtimecheck/bind.go:15-18`). The
consequence is that every process running as this user, including every agent
Matrix launches, has full runtime use, and with elicitation enabled it can answer
the prompts gating agent actions.
*Options*: generate a per-install key on first run the way the master key is
generated, require it even on loopback, keep an explicit development opt-out; or
move the local surface to a Unix socket with `0600`.

**5. The JSON-RPC daemon returns decrypted vault values with no key configured.**
`internal/logic/daemon/vault_service.go:51-58` authorizes an empty
`daemon_api_key` by returning nil, so `Vault.Get` exposes every provider key,
agent env override and endpoint header to any local caller.

**6. An install proceeds with no integrity check when the index omits `sha256`**
(`internal/logic/agentmgr/artifact_integrity.go:46-50`, then extraction at
`internal/logic/agentmgr/installer.go:203-214`). This is a deliberate, documented
outcome (`digest_not_published`) and 53 of 100 registry distributions do publish a
digest. Decide whether "no digest published" should be installable by default or
require an explicit `--allow-unverified`.

**7. The registry index supplies the command that is launched**
(`internal/logic/agentmgr/installer.go:178-183`); only local relative paths are
contained, so a hostile index can name any local binary. *Options*: pin the
commands Matrix is willing to launch, or verify that the resolved command is the
artifact that was just installed.

## Documentation that contradicts the code

Each of these is a claim in a committed document that the cited code does not
support; the documents should be corrected whichever way the decision goes:

- Telegram admins gating (above), claimed in three wiki pages.
- `docs/governance/security_review_2026-09-22.md:70-73` accepts G204 because "no
  shell, so no interpolation occurs", but the env-isolation launcher runs
  `bash -c` with Go `%q` (`internal/logic/agentlaunch/stdio_unix.go:16-21`), which
  does not escape `$` or backticks.
- `docs/governance/security_review_2026-09-22.md:50-53` reports config files fixed
  to `0600`; `os.WriteFile` applies the mode only at creation, so pre-existing
  `0644` files stay `0644` (`internal/providers/osfs/config.go:23`).

## Notes

- A human security review still has not happened. This threat model is a written
  analysis by an agent, reviewed by another agent; it is better than nothing and is
  not a substitute.
- The Windows-side items (ACLs, `install.ps1`) and real-agent tool-call behaviour
  are recorded as Unknowns in the document rather than assumed.
