# EP-01 — Registrazione agent ACP via nativo funziona, registry standard no

> Nota agente esterno (MiniMax M3, `MiniMax-M3`), 28/09/2026. Nessun file Matrix al di fuori di `issues/usage-feedback-halfpocket/` è stato toccato. Nessuna patch, nessun commit, nessuna azione mutante su provider/runs/daemon/servizi.

**Stato**: `suggestion` (evoluzione) + `certificato` (sequenza osservata limitata a quanto sotto).
**Severità pratica**: media onboarding agent di terze parti. NON bug ACP.

## Sintesi

`matrix agent set-binary/enable` registra l'agent nel Vault SSOT (override `active: true`). `matrix agent doctor` handshake PASS; `matrix agent show` vede subito l'override. Il daemon runtime invece non espone ancora l'agent come eseguibile ACP finché non si fa idle+restart: `POST model req` riceve `HTTP 409 "model_id is supported only for ACP agents"`.

## Cosa è attestato vs cosa no

- **Attestato**: sequenza CLI nativa (`set-binary` + `enable` → SSOT aggiornato) e risposta `409` durante finestra runtime non rinfrescato.
- **NON attestato come prova universale**: l'attivazione per-agent "richiede restart daemon" non è classificata come problema intrinseco ACP. È stato osservato nel caso specifico MiMo/HalfPocket. Può essere una scelta implementativa del runtime Matrix o una conseguenza del caso d'uso (catalog agent non presente → cold load).
- **NON attestato**: assenza di DSH dal catalogo globale ACP. L'assenza è osservata nel runtime HalfPocket del PM; non è stata fatta verifica dedicata su un catalogo globale ACP.

## Evidenze

- Run: `52b0a4ef-933a-4d2b-bd34-79a8ec667edb`, `325ed3ce-ca00-4a6e-ba7a-25bfabd70eb2`, `40ca971c-79eb-4890-b979-ea3f249c06cd` (ID proxy MiMo, NON terza run Matrix), `b8b2257b-394a-4714-b75a-cc886b291e93` (in corso G11).
- Report: `~/halfpocket/docs/cloud-transition/evidence/2026-09-28-mimo-matrix-cwd.md` 26–30, 104–129.
- Versione Matrix: `matrix 0.1.46`, commit `f0b97ab35e761cad6721a1ab985d3b51005f3e8e`, built `2026-09-24T13:52:20Z`. HEAD sorgente `2864953` (documentale).

## Suggerimenti (con priorità e criteri di accettazione)

| ID | Suggerimento | Priorità | Criterio di accettazione misurabile |
| --- | --- | --- | --- |
| EP-01.A | Distinguere `status=pending_apply` da `status=error` nel recapito | **P2** | Per un agent appena registrato, `matrix agent show <id>` mostra `runtime.status=pending_apply` (NON `error`) entro la stessa finestra di `set-binary`; il 409 sparisce dopo restart/idle senza riapparire per `enable` idempotente. |
| EP-01.B | Hot-reload agent senza restart daemon, o restart scoped al solo child | **P3** | Dopo `matrix agent enable <id>` su agent non in runtime, una nuova run viene servita dal child entro 30 s senza riavvio del daemon Matrix (verificabile con `readlink /proc/<pid>/cwd` del child pre/post). |
| EP-01.C | Messaggio di errore più preciso sul 409 | **P3** | Il 409 include il nome agent e l'azione risolutiva esplicita ("riavvia daemon" o "riavvia child"), non solo la stringa generica. |
| EP-01.D | Verifica dedicata dell'assenza DSH nel catalogo ACP globale | **P3** | Report dedicato (NON questo task) con query su catalogo e confronto runtime, prima di trattare DSH come "non supportato ACP" in policy. |

## Follow-up

Nessuno in questo task.


## Decisione del manutentore — 2026-10-08

Registrazione e registro vivo già corretti; completati avvio supervisionato tardivo ed endpoint esterni.

Scheda gestita e archiviata. [Decisione completa, motivi ed evidenze](../../../docs/governance/issue_resolution_2026-10-08.md).
Lo stato di pubblicazione e installazione è nel verbale della nuova release;
le evidenze del reporter sopra restano lo snapshot originale.
