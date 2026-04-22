# Robustness: `/v1/session-actions` capabilities on unknown agent should return typed error, not HTTP 500

Date: 2026-04-22
Reporter: Noema integration
Priority: medium
Type: API robustness

## Summary

During Noema verification of the new lifecycle capability layer, this request returned HTTP `500`:

```bash
curl -X POST http://127.0.0.1:9091/v1/session-actions \
  -H "Content-Type: application/json" \
  -d '{
    "channel_id": "noema.verify.codex-acp",
    "action": "capabilities",
    "target": "codex-acp"
  }'
```

The local Matrix agent id is `codex`, not `codex-acp`, so the target was invalid.

The problem is not the unsupported provider state itself. The problem is the shape of the failure for supervisory clients.

Noema should be able to distinguish:

- known provider, capability unsupported
- unknown provider id
- provider startup/probe failure
- Matrix internal error

without treating all of them as server faults.

## Requested Behavior

For unknown `target` in `action=capabilities`, please return a typed client-safe response.

Possible HTTP/API shape:

```json
{
  "action": "capabilities",
  "unsupported": true,
  "error": {
    "code": "agent_not_found",
    "message": "agent id is not registered",
    "target": "codex-acp"
  }
}
```

HTTP status could be `404` or `400`; the important part is that the body is machine-readable and not a generic `500`.

## Why It Matters To Noema

Noema will query Matrix capability reports before choosing provider mode:

```text
fork -> interrupt/resume -> live attach -> static capsule -> observe-only
```

If a typo or stale provider id returns `500`, Noema cannot safely decide whether it should:

- retry
- mark provider unsupported
- fail configuration
- ask the operator to fix the agent id

Typed errors improve production diagnostics and prevent false capability assumptions.

## Acceptance Criteria

- Unknown target for `action=capabilities` does not return generic `500`.
- Response includes a stable machine-readable code such as `agent_not_found`.
- Noema can classify the error as configuration failure, not provider capability failure.

## Maintainer Response

Accepted and implemented.

`action=capabilities` now classifies unknown agent ids as a typed client-safe
configuration error:

```json
{
  "action": "capabilities",
  "unsupported": true,
  "error": {
    "code": "agent_not_found",
    "message": "agent id is not registered",
    "target": "codex-acp"
  }
}
```

The HTTP surface returns `404` for this typed result instead of a generic
`500`. Provider capability absence remains distinct from unknown provider id.
