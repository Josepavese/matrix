# Matrix request: align ACP session lifecycle capabilities with March/April 2026 upstream changes

Date: 2026-04-22
Reporter: Noema integration
Priority: medium-high

## Summary

Noema depends on Matrix as the provider-neutral execution/control layer for strong market agents.

ACP has moved several session lifecycle features forward in March and April 2026. Matrix should track these capabilities explicitly per adapter/provider, expose them in its own capability/evidence surface, and gate behavior by stability level so Noema does not infer support from provider-specific behavior.

This is not a request to add Noema semantics to Matrix. It is a request for Matrix to keep its ACP lifecycle surface aligned with official upstream ACP/Zed capabilities and to make support/unsupported states observable.

## Official Sources

Primary official sources:

- Zed ACP overview: https://zed.dev/acp
- Zed ACP client page: https://zed.dev/acp/editor/zed
- ACP protocol schema: https://agentclientprotocol.com/protocol/schema
- ACP updates: https://agentclientprotocol.com/updates
- ACP RFD updates: https://agentclientprotocol.com/rfds/updates

Relevant RFDs:

- `session/resume` RFD, Preview as of 2026-04-14: https://agentclientprotocol.com/rfds/session-resume
- `session/close` RFD, Preview as of 2026-04-14: https://agentclientprotocol.com/rfds/session-close
- `session/fork` RFD, Draft: https://agentclientprotocol.com/rfds/session-fork

Observed upstream timeline from official pages:

- 2026-02-04: Session Config Options stabilized: https://agentclientprotocol.com/updates
- 2026-03-09: ACP Registry released: https://agentclientprotocol.com/updates
- 2026-03-09: `session_info_update` stabilized: https://agentclientprotocol.com/updates
- 2026-03-09: `session/list` stabilized: https://agentclientprotocol.com/updates
- 2026-04-14: `session/resume` moved to Preview: https://agentclientprotocol.com/rfds/updates
- 2026-04-14: `session/close` moved to Preview: https://agentclientprotocol.com/rfds/updates
- 2025-11-20: `session/fork` moved to Draft: https://agentclientprotocol.com/rfds/updates

## Why Noema Needs This

Noema needs to know whether Matrix can safely:

- list sessions without provider-specific hacks
- observe session metadata updates
- resume an existing session without replaying history
- close sessions to free agent/provider resources
- potentially fork a session for temporary interpreter/subagent work

Today, Noema can work around some of this through Matrix run/session cleanup evidence, but capability ambiguity weakens product claims.

For Noema, the important distinction is:

```text
supported by official stable ACP
vs preview capability
vs draft/experimental capability
vs Matrix/provider-specific fallback
vs unsupported
```

Without this distinction, Noema risks treating diagnostic behavior as product capability.

## Requested Matrix Behavior

Please consider adding/normalizing a Matrix ACP session lifecycle capability model.

Suggested shape, exact field names up to Matrix:

```json
{
  "provider": "opencode",
  "protocol": "acp",
  "session_capabilities": {
    "list": {
      "status": "supported",
      "stability": "stable",
      "source": "acp.session/list"
    },
    "info_update": {
      "status": "supported",
      "stability": "stable",
      "source": "acp.session_info_update"
    },
    "load": {
      "status": "supported|unsupported|unknown",
      "stability": "stable",
      "source": "acp.loadSession"
    },
    "resume": {
      "status": "supported|unsupported|unknown",
      "stability": "preview",
      "source": "acp.session/resume"
    },
    "close": {
      "status": "supported|unsupported|unknown",
      "stability": "preview",
      "source": "acp.session/close"
    },
    "fork": {
      "status": "supported|unsupported|unknown|experimental",
      "stability": "draft",
      "source": "acp.session/fork"
    }
  }
}
```

Useful Matrix surfaces:

- provider capability report
- run trace metadata
- `/v1/providers` or equivalent provider info endpoint if one exists
- cleanup/session proof records
- adapter diagnostics

## Specific Requests

1. Track `session/list` as stable ACP capability.
2. Track `session_info_update` as stable ACP capability.
3. Track session config options as stable ACP capability if Matrix exposes model/mode/reasoning selectors.
4. Track `session/resume` as Preview, not stable.
5. Track `session/close` as Preview, not stable.
6. Track `session/fork` as Draft/experimental, not stable.
7. For each provider/adapter, expose whether Matrix support is direct ACP support, provider-native mapping, Matrix emulation, or unsupported.
8. Avoid silent fallback: if a capability is missing, return typed `unsupported`/`blocked` evidence.

## Noema Product Impact

Noema will use this to decide:

- whether interrupt/resume is a valid capability for a provider
- whether cleanup evidence is strong enough for production claims
- whether temporary interpreter work can happen in a forked context
- whether a run should be blocked/diagnostic instead of launched
- whether a capability can be included in provider capability reports

## Acceptance Criteria

- Matrix documents which ACP lifecycle capabilities it supports directly.
- Matrix records capability stability: stable, preview, draft, matrix-specific, unsupported.
- Provider-specific adapters report capability truth without Noema parsing provider strings.
- Preview/draft features are opt-in or clearly marked as experimental.
- Unsupported capabilities produce durable typed evidence rather than silent success.

## Non-goals

- Matrix should not implement Noema cognition.
- Matrix should not interpret Noema sidecar capsules.
- Matrix should not decide whether Noema suggestions are correct.
- Matrix should not turn Draft ACP features into stable Matrix promises without capability gating.

## Matrix maintainer response

Status: accepted and implemented.

Matrix now has a protocol-neutral lifecycle capability report. The provider
capability SSOT is exposed through `/v1/session-actions` with
`action=capabilities`.

Current ACP lifecycle model:

- `list`: stable, source `zed_acp_session_list_rfd`
- `info_update`: stable, source `zed_acp_session_info_update`
- `load`: stable, source `zed_acp_schema_loadSession`
- `cancel`: stable, source `zed_acp_schema_session_cancel`
- `resume`: preview, source `zed_acp_rfd_session_resume`
- `close`: preview, source `zed_acp_rfd_session_close`
- `fork`: draft, source `zed_acp_rfd_session_fork`
- `delete`: draft, source `zed_acp_rfd_session_delete`

Each capability includes:

- `supported`
- `status`
- `stability`
- `source`

Provider observations from real installed agents:

- `opencode`: ACP, `list/load/info_update/resume/fork/cancel` supported;
  `close/delete` unsupported.
- `codex-acp`: ACP, `list/load/info_update/close/cancel` supported;
  `resume/fork/delete` unsupported.
- `gemini`: ACP, `load/cancel` supported; `list/info_update/resume/close/fork/delete`
  unsupported.

Unsupported capabilities now return durable typed evidence rather than silent
success. Preview and draft features are not promoted to stable Matrix promises.

Validation:

- Unit tests added for Zed object-style capabilities and lifecycle stability
  metadata.
- Real HTTP capability probes were executed against installed `opencode`,
  `codex-acp`, and `gemini`.
- Deploy preflight, lint, tests, GoReleaser snapshot, and local PAL install pass.
