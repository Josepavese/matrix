# README and Wiki refresh — 2026-10-09

Scope: documentation, editable technical diagrams and the ACP ecosystem grid.
No runtime implementation change or new binary release is claimed.

## Changes

- README defines Matrix's position, agent/model distinction, communication
  patterns and concrete onboarding with explicit project and authenticated API.
- Replaced the three raster infographics and their placeholders with two SVG
  diagrams: architecture and delegation. Removed unsupported competitor claims
  and absolute transcript-transfer promises.
- Added 47 wrapping ACP ecosystem cards from Zed's source directory: 42 supplied
  icons and five name cards. Assets are self-contained local SVGs; provenance
  lives in [SOURCES.md](../assets/readme/agents/SOURCES.md).
- Updated all existing Wiki pages and added delegation/notification and
  session/recovery guides. The PAL guide covers the shared contracts and the
  distinction between native tests and physical driver qualification.
- Corrected default HTTP authentication, CLI workspace channel requirements,
  logical/remote session distinction, metadata snapshot semantics and Vault
  encryption prerequisites. Provider credits remain distinct from host capacity.

## Verification

- Local Markdown links and heading anchors checked, including source attribution.
- SVG XML parsed; no scripts or external resource references in imported cards.
- Bash blocks syntax-checked without executing configuration/install/task commands;
  symbolic CLI placeholders normalized only for syntax checking. JSON blocks parsed.
- Browser preview at desktop default and 390×844: both diagrams loaded, all 47
  cards loaded, mobile grid used three columns, no page-wide horizontal overflow.
  Navigation README → Wiki → Getting Started verified by clicking links.
- `git diff --check` and the manifest governance checker passed. This is
  documentation evidence, not a new provider/container/platform certification.

## Newly found runtime defect

The installed v0.1.53 `matrix run submit` was tested with a non-existent probe
agent and a non-private prompt. It returned HTTP 400 before provider dispatch:
`channel_id` is missing from its client payload. The source also omits async
selection and does not accept the async 202 response. At the time of the documentation audit the issue was open; its later correction is recorded in
[run-submit-missing-channel.md](../../issues/closed/run-submit-missing-channel.md).
The documentation's runnable delegation flow uses explicit HTTP instead.

## Publication

The versioned Wiki remains under `docs/wiki/`. GitHub's separate Wiki feature
was disabled when inspected; repository settings were not changed. Documentation
is published with the repository's main branch rather than a separate Wiki store.
