# EP-07 — Quota 429 / RetryAfter / one-shot resume (SOLO PROPOSTA)

> Nota agente esterno (MiniMax M3, `MiniMax-M3`), 28/09/2026. Nessun file Matrix al di fuori di `issues/usage-feedback-halfpocket/` è stato toccato. Nessuna patch, nessun commit, nessuna azione mutante su provider/runs/daemon/servizi.

**Stato**: `suggestion` (SOLO proposta). NON `certificato`, NON bug, NON evidenza di funzionamento.

**Severità**: N/A (nessuna osservazione effettuata).

## Cosa NON è attestato

- **Nessuna prova fornita** in questo task di HTTP 429, `reset=5h`, `Retry-After`, né di resume automatico ("one-shot resume") funzionante.
- **Nessuna osservazione**: i pattern citati sono SOLO ipotesi di policy; non sono stati osservati nei trace del 2026-09-28 né sono allegati qui come prova.
- **Capacità disco**: la workstation HalfPocket ha `/` 314G/292M liberi (NVMe quasi pieno). Questo dato è **capacità workstation**, NON capacità VPS. La VPS HalfPocket è attestata separatamente in `~/halfpocket/docs/cloud-transition/evidence/2026-09-28-vps-preflight.md` (`/` LVM 39G, 25G liberi). Le due fonti non vanno mescolate.

## Suggerimenti (SOLO proposta — con priorità e criteri di accettazione)

| ID | Suggerimento | Priorità | Criterio di accettazione misurabile |
| --- | --- | --- | --- |
| EP-07.A | Quota 429/RetryAfter strutturati senza schedule polling | **P3** | Un test futuro che osservi effettivamente un 429 mostra: la risposta include `Retry-After` strutturato e il runtime non genera schedule polling. Criterio verificabile solo DOPO aver ottenuto almeno un caso reale documentato. |
| EP-07.B | Task queue fairness/cancel per-run (no other channels) | **P3** | Una richiesta di `cancel` su una run quotata non genera effetti su altre run né su altri canali; verificabile con run di confronto. |
| EP-07.C | Capacity metadata per workspace (es. SSD scarso) | **P3** | `matrix workspace show <id>` (o equivalente) espone `capacity.disk_free_bytes` per il workspace, con cap configurabile; nessuna azione automatica di pulizia. |
| EP-07.D | Prima di qualsiasi evoluzione, raccogliere almeno un caso reale documentato di 429 | **P1** (precondizione) | Report separato con: provider coinvolto, full run_id, status HTTP e soli campi redatti ammessi (`Retry-After`, codice errore), comportamento del runtime, ripresa effettiva (sì/no). Mai risposta HTTP completa, header sensibili o transcript. Solo DOPO questo le proposte P3 diventano concrete. |

## Evidenze

- Nessuna evidenza prodotta in questo task per i pattern citati.
- VPS preflight (capacità VPS, distinta): `~/halfpocket/docs/cloud-transition/evidence/2026-09-28-vps-preflight.md`.
- Versione Matrix: `matrix 0.1.46`, commit `f0b97ab35e761cad6721a1ab985d3b51005f3e8e`. HEAD sorgente `2864953`.

## Follow-up

Nessuno in questo task. Precondizione P1 prima di qualsiasi follow-up concreto.


## Decisione del manutentore — 2026-10-08

Diagnostica strutturata e avviso accolti. Reset UTC da prosa, 429 sempre quota, retry/fallback impliciti e capacity manager cliente rifiutati con motivi nel riepilogo.

Scheda gestita e archiviata. [Decisione completa, motivi ed evidenze](../../../docs/governance/issue_resolution_2026-10-08.md).
Lo stato di pubblicazione e installazione è nel verbale della nuova release;
le evidenze del reporter sopra restano lo snapshot originale.
