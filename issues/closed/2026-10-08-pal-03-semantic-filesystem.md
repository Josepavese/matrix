# Sviluppo PAL — 03-semantic-filesystem

Stato: gestita; sviluppo, gate locali e CI nativa Linux/Windows/macOS completati. Release v0.1.53 pubblicata e verificata; aggiornamento locale in attesa dei task già attivi.

Autorizzazione: richiesta esplicita dell’utente del 2026-10-08 per tutti e quattro gli sviluppi e requisito PAL Linux/Windows/macOS.

## Ambito e accettazione

Filesystem semantico in sola lettura: fs.FS comune, viste tipizzate di agenti/run/workspace, credenziali escluse, sommari solo opt-in. Accesso autenticato e backend di mount PAL Linux/macOS/Windows con prerequisiti espliciti (FUSE/WinFsp o equivalente). Proteggere traversal/symlink, permessi, finestre incomplete e ciclo di vita; sostituire il demo statico senza fingere mount riusciti.

## Consegna

Verifiche, documentazione, release e installazione locale. Archiviare in closed solo dopo decisione e prove registrate.

## Evidenze dello sviluppo

Vedi [ledger PAL](../../docs/governance/pal_implementation_2026-10-08.md) e [guida operativa](../../docs/wiki/PAL-Execution-and-Observability.md). Le qualifiche fisiche dei driver opzionali sono distinte per piattaforma.

Pubblicazione e stato dell’installazione: [verbale v0.1.53](../../docs/governance/releases/2026-10-08-v0.1.53.md).
