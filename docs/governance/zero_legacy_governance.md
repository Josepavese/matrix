# ZERO-LEGACY Governance

ZERO-LEGACY is a project release gate. Active Matrix surfaces use one canonical
contract and fail closed when they encounter a retired one.

## Scope

The policy covers runtime code, persisted Vault configuration, installers,
commands, active documentation and examples, CI, release artifacts, and local
deployments. A dependency being globally installed does not make it supported.

Historical evidence may retain legacy names under `issues/closed/` and
`docs/governance/releases/`. Tests may name a retired contract only to prove
that the single rejection boundary blocks it.

## Rules

1. No runtime fallback, compatibility alias, dual read/write path, or silent
   conversion may preserve a retired contract.
2. Detection happens before launch or persistence and returns an actionable
   error naming the canonical replacement.
3. Migration is explicit, one-way, and atomic: back up the Vault, install the
   canonical artifact inside the PAL, rewrite the canonical entry, verify it,
   then remove the retired package. Migration code is not a runtime fallback.
4. New installs and upgrades must be self-contained in the PAL and must not
   depend on an unrelated global package.
5. Rejected or non-Matrix defects do not justify temporary product, CI, or
   diagnostic surfaces.
6. There are no permanent exceptions. A release-blocking migration seam must
   be minimal, fail closed, carry its removal condition, and be enforced by a
   governance budget.

## Protocol evolution

ACP, A2A, and every future protocol adapter implement one current stable
protocol contract at a time. A protocol upgrade removes the superseded wire
shape, fallback, capability advertisement, tests, and active documentation in
the same change.

Draft or unstable protocol features may exist only behind an explicitly named,
non-default experimental surface. They must never be used as an implicit
fallback for the stable protocol. Receiving a retired wire shape fails closed
at the adapter boundary and points to the stable protocol replacement.

## Codex identity contract

`codex` is the only public Matrix agent ID. `codex-acp` is the canonical ACP
registry/provider identifier and may appear in adapter paths and diagnostics;
it is not accepted as a routing, install, or persisted agent ID.

The canonical Codex ACP provider is installed by Matrix into the PAL. The
retired Zed-scoped provider is accepted only as input to the centralized
rejection check and its tests. It must never be launched.

## Release evidence

A ZERO-LEGACY change is releasable only when:

- `governance_check` passes with the pattern budgets in
  `governance/manifest.toml`;
- unit and race tests prove canonical acceptance and legacy rejection;
- the release artifact installs without a repository clone;
- local deploy evidence shows the daemon and canonical provider running from
  PAL-owned paths; and
- the retired global package is absent after the canonical smoke test passes.
