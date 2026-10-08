# Sviluppo PAL — 02-capacity

Stato: sviluppo implementato; gate locali e prove Linux completati, verifica CI nativa e consegna release in corso.

Autorizzazione: richiesta esplicita dell’utente del 2026-10-08 per tutti e quattro gli sviluppi e requisito PAL Linux/Windows/macOS.

## Ambito e accettazione

Capacità: osservazione del filesystem reale del workspace tramite PAL Linux/Windows/macOS, fonte/timestamp/volume e indisponibilità esplicita. Ammissione con concorrenza e prenotazioni disco delimitate al runtime; rilascio su ogni esito, nessuna pulizia automatica e nessun saldo provider dedotto. Test nativi per ogni OS e collisioni fra workspace sullo stesso volume.

## Consegna

Verifiche, documentazione, release e installazione locale. Archiviare in closed solo dopo decisione e prove registrate.

## Evidenze dello sviluppo

Vedi [ledger PAL](../docs/governance/pal_implementation_2026-10-08.md) e [guida operativa](../docs/wiki/PAL-Execution-and-Observability.md). Le qualifiche fisiche dei driver opzionali sono distinte per piattaforma.
