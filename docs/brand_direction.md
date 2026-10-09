# Matrix Brand Direction

## Positioning

Matrix is a local communication hub for existing coding agents: people, scripts
and supervisory software share run, session and workspace contracts through
ACP/A2A adapters.

Tagline: **Your agents. One surface. Local-first.**

## Writing

- Start with a concrete task and explain which component executes it.
- Distinguish agent, model, run, logical session and provider session.
- Describe handoff as an operational brief, not a full transcript transfer.
- Separate implemented behavior, negotiated capabilities and planned work.
- Explain local control without implying cloud inference or telemetry stays local.
- State platform prerequisites and physical qualification separately.
- Avoid blanket competitor comparisons and universal compatibility promises.

## Visuals

The README uses two editable technical SVGs under `docs/assets/readme/`:

- `architecture.svg`: channels → local Matrix daemon → external agents; model,
  tool and account ownership stays with the agent/provider.
- `delegation.svg`: submit → work → relevant event → inspect/intervene, with
  Unix notification and cross-platform HTTP availability made explicit.

The wrapping agent grid uses locally embedded registry icons and SVG name cards
under `docs/assets/readme/agents/`. Preserve the provenance in `SOURCES.md` and
distinguish ecosystem compatibility from tested Matrix integrations.

Use clear arrows, a limited number of nodes, large labels and descriptive alt
text. Each diagram also needs a nearby prose explanation. No generated logos,
ambiguous provider/protocol mappings or screenshots of private transcripts.

Palette: ink `#0B1020`, teal `#00D1B2`, blue `#3B82F6`, cloud `#F5F7FB`.
Use vectors for technical diagrams; keep text editable and check rendering at
desktop and mobile widths after changes.

## Wiki

The versioned Wiki under `docs/wiki/` is linked from the README. Keep onboarding,
delegation/recovery, reference contracts and optional PAL features connected.
Roadmaps remain design direction rather than evidence of shipped behavior.
