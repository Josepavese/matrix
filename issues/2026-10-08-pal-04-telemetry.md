# Sviluppo PAL — 04-telemetry

Stato: sviluppo implementato; gate locali e prove Linux completati, verifica CI nativa e consegna release in corso.

Autorizzazione: richiesta esplicita dell’utente del 2026-10-08 per tutti e quattro gli sviluppi e requisito PAL Linux/Windows/macOS.

## Ambito e accettazione

Telemetria: esportazione opzionale verso collector standard, file JSONL locale autorevole, metadati operativi selezionati senza prompt/transcript/segreti. Coda e timeout limitati, stato/dropped/failure osservabili, collector assente non blocca Matrix. HTTP PAL Linux/Windows/macOS; prova con ricevitore reale compatibile e test di privacy/cancellazione.

## Consegna

Verifiche, documentazione, release e installazione locale. Archiviare in closed solo dopo decisione e prove registrate.

## Evidenze dello sviluppo

Vedi [ledger PAL](../docs/governance/pal_implementation_2026-10-08.md) e [guida operativa](../docs/wiki/PAL-Execution-and-Observability.md). Le qualifiche fisiche dei driver opzionali sono distinte per piattaforma.
