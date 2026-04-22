# Docs inconsistency: `matrix_v2_protocol_neutral_runtime.md` session action list misses new lifecycle actions

Date: 2026-04-22
Reporter: Noema integration
Priority: low
Type: documentation consistency

## Summary

Matrix implemented the new lifecycle capability layer and the API reference correctly documents:

- `capabilities`
- `fork`
- `reconcile`

as valid `/v1/session-actions` actions.

However, `docs/matrix_v2_protocol_neutral_runtime.md` still has an older action list that says:

```text
action: currently cancel, delete, cleanup, switch, list, status, new, or name
```

This conflicts with the same document later, and with `docs/wiki/API-Reference.md`.

## Current Evidence

Correct references:

- `docs/wiki/API-Reference.md`, `POST /v1/session-actions`, parameters table:
  `new`, `list`, `switch`, `cancel`, `delete`, `cleanup`, `name`, `capabilities`, `fork`, `reconcile`
- `docs/matrix_v2_protocol_neutral_runtime.md` later states that channels and HTTP can request `action=capabilities`, `action=fork`, and `action=reconcile`.
- Installed `matrix orchestration capabilities` exposes `session.lifecycle` with `capabilities`, `fork`, and `reconcile`.

Inconsistent reference:

- `docs/matrix_v2_protocol_neutral_runtime.md`, typed action envelope section.

## Requested Fix

Update the old action list in `docs/matrix_v2_protocol_neutral_runtime.md` so the document has a single consistent contract.

Suggested wording:

```text
action: currently cancel, delete, cleanup, switch, list, status, new, name,
capabilities, fork, or reconcile
```

## Acceptance Criteria

- No Matrix docs page presents the old lifecycle action set as complete.
- `matrix_v2_protocol_neutral_runtime.md` and `docs/wiki/API-Reference.md` agree.
- Future Noema integration docs can cite Matrix docs without caveats.

## Maintainer Response

Accepted and implemented.

`docs/matrix_v2_protocol_neutral_runtime.md` now lists the full
`/v1/session-actions` action set: `cancel`, `delete`, `cleanup`, `switch`,
`list`, `status`, `new`, `name`, `capabilities`, `fork`, and `reconcile`.

The same section now documents fork-only safety fields: `make_active`,
`restore_parent`, and `input`.
