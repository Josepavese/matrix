# EP-05 — SSE run-notifications recapitano a Codex senza intermediati (POSITIVO)

> Nota agente esterno (MiniMax M3, `MiniMax-M3`), 28/09/2026. Nessun file Matrix al di fuori di `issues/usage-feedback-halfpocket/` è stato toccato. Nessuna patch, nessun commit, nessuna azione mutante su provider/runs/daemon/servizi.

**Stato**: `certificato` (POSITIVO recapito nativo) + `suggestion` (ingress adattatore, primitive CLI/SDK).
**Severità**: nessuna. Pattern da preservare.

## Sintesi

Matrix espone Unix socket `run-notifications.sock` con SSE; recapito eventi terminali a Codex via `send_message_to_thread` con solo `run_id`/esito, nessun transcript provider né intermediati. `matrix agent doctor` handshake PASS per agent ACP.

## Verifica indipendente PM

Riportata in `~/halfpocket/docs/cloud-transition/evidence/2026-09-28-deepseek-opencode-policy.md` 141–148: `cab3788c…` termina `completed`, `model_verification=provider_confirmed`, stessa sessione logica `ad6da3b1-b82f-43d9-8cd4-d020c11873f3`.

## Run ID attestati terminali

- GLM `cef15023…`
- M3 `54f552c5…`
- DeepSeek `98447c88…`, `cab3788c…`

## Policy esistente (NON bug)

- Webhooks localhost vietati → protezione SSRF intenzionale. NON bug.
- Source Matrix: pattern Unix-socket + SSE già attivo (sola lettura).

## Gap di ingress attuale

Adattatore minimo `Codexsend_message_to_thread` + SSE inline Python. NON bridge autonomo.

## Suggerimenti (con priorità e criteri di accettazione)

| ID | Suggerimento | Priorità | Criterio di accettazione misurabile |
| --- | --- | --- | --- |
| EP-05.A | Primitive CLI/SDK: `run submit + wait events/cursor/ack/idempotency delivery/elicitation summary short` | **P1** | Una run emette eventi terminali ricevuti via `wait events` con `run_id`+esito (NO transcript), e `ack` con idempotency key ricevuto esattamente una volta lato Matrix. |
| EP-05.B | Niente polling; durabilità reconnect/restart con delivery dedup | **P1** | Riavvio del client SSE → eventi già recapitati NON sono re-inviati (dedup), eventi non ancora recapitati vengono recuperati senza richiedere nuova run. |
| EP-05.C | Native user-questions strict scoping | **P2** | Le domande sollevate dal runtime arrivano come eventi `elicitation` distinti dagli eventi terminali e sono rispondibili via SDK senza ricorrere al transcript. |
| EP-05.D | Mantenere Unix-socket + SSE come canale positivo, non sostituire con webhook localhost | **P2** | Test di policy: tentativi di registrazione webhook su `127.0.0.1`/`localhost` continuano a essere rifiutati (protezione SSRF invariata). |
| EP-05.E | Bridge Codex minimo resta adattatore, non viene promosso a canale autonomo senza design review | **P3** | Qualsiasi estensione del bridge è soggetta a review che distingua "adattatore CLI" da "canale di delivery primario". |

## Evidenze

- Trace Matrix terminali; report PM citato.
- Versione Matrix: `matrix 0.1.46`, commit `f0b97ab35e761cad6721a1ab985d3b51005f3e8e`. HEAD sorgente `2864953`.

## Follow-up

Nessuno in questo task.


## Decisione del manutentore — 2026-10-08

Socket/cursor/replay/ack/elicitation già implementati; estensione opt-in con wakeup attention_required.

Scheda gestita e archiviata. [Decisione completa, motivi ed evidenze](../../../docs/governance/issue_resolution_2026-10-08.md).
Lo stato di pubblicazione e installazione è nel verbale della nuova release;
le evidenze del reporter sopra restano lo snapshot originale.
