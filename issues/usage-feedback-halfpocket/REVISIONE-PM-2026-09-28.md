# Verifica PM e fonti preesistenti — 28 settembre 2026

Nota di un agente esterno (Codex, PM Half Pocket). Scritture limitate a
`issues/usage-feedback-halfpocket/`; nessun altro file di Matrix modificato,
nessun commit, push o intervento su codice/installazione. Archivio non tracciato.

## Baseline verificata

- `matrix version`: **0.1.46**, commit
  `f0b97ab35e761cad6721a1ab985d3b51005f3e8e`, build
  `2026-09-24T13:52:20Z`.
- HEAD del sorgente letto: `2864953dad43cb9e12892b0cc3bd95377f7e0f1f`.
  La revisione del checkout non va confusa con quella del binario installato.
- Nessun transcript acquisito per questa verifica: solo campi di stato,
  selezione modello e riferimenti degli artefatti.

## Capacita' gia' presente: cursor SSE durevole

`internal/providers/runapi/notifications.go` contiene gia' `after` e
`Last-Event-ID`, il filtro `run_id`, il recupero da store e gli ID SSE numerici.
Il cursor non e' una nuova funzione da sviluppare. L'evolutiva EP-05 riguarda
ergonomia CLI/SDK e integrazione dell'ingresso Codex, valorizzando questi
contratti. Reconnect, deduplicazione ed esito dopo un crash del destinatario
restano da provare end-to-end: il semplice recapito terminale non li dimostra.

Priorita' **P1** per la robustezza dell'adattatore; accettazione: interrompere
il destinatario dopo ricezione ma prima della consegna, riaprire dal cursor
persistito e ottenere un solo esito logico, senza transcript e senza polling
del supervisore. La fattibilita' dell'ack va verificata, non assunta.

## Prove terminali aggiuntive

Le notifiche Unix SSE sono state recapitate a questa task Codex con soli
`run_id` ed esito. Il trace terminale conferma `completed` e
`model_verification=provider_confirmed` per i run seguenti; modello richiesto
ed effettivo coincidono:

| Run ID | Modello | Workspace Matrix |
| --- | --- | --- |
| `run-46664ab3-af46-4432-af4b-71fcaa71ee31` | `minimax-coding-plan/MiniMax-M3` | `halfpocket-matrix-feedback` |
| `run-5a346416-b104-44f9-95b0-ccfe9853b107` | `deepseek/deepseek-flash` | `halfpocket-deepseek-policy` |
| `run-b8b2257b-394a-4714-b75a-cc886b291e93` | `xiaomi/mimo-v2.6-pro` | `halfpocket-mimo-s3` |
| `run-867a0b2d-47d3-45cd-b643-8e0509090dc8` | `zai-coding-plan/glm-5.3` | `halfpocket-glm-vertical-runtime24` |

Questo prova il completamento del run e la selezione attestata dal provider,
non il completamento del prodotto, l'installazione VPS o l'identita' fisica
del modello sul server del fornitore.

CORREZIONE terminologica PM: i nomi JSON reali del trace 0.1.46 sono
`requested_model`, `effective_model`, `model_verification`. Le diciture
`model_request`/`model_effective` nelle altre schede sono sintesi, non nomi
da copiare in un client. I tre livelli gia' esistono: l'evoluzione EP-06
riguarda esposizione/documentazione e limiti della prova, non la loro
introduzione ex novo. `model_fallback_used` assente nel trace non prova un
valore false.

## Segnalazioni preesistenti: collegare, non duplicare

- [OpenCode/MiniMax: output non concluso dopo completed](../2026-09-25-opencode-minimax-end-turn-with-unfinished-output.md).
- [MiMo: completed senza consegna](../2026-09-26-external-halfpocket-mimo-completed-without-deliverable.md).
- [Workspace grant e risoluzione path](../2026-09-27-external-workspace-grant-id-does-not-resolve-path.md).

Le tre note restano intatte e non tracciate. Non si presume che siano ancora
riproducibili o risolte: ogni retest deve riportare revisione, run e prova.

## EP-08 — Endpoint sbagliato nella verifica PM (caller error)

Tentativi GET HTTP `/v1/runs/<id>/trace` su `127.0.0.1:9090` hanno restituito
`Connection reset by peer`. Il sorgente `cmd/matrix/constants.go` distingue
JSON-RPC `9090` da HTTP Matrix `9091`; ripetere sul `9091` ha funzionato.
Nessun bug Matrix dimostrato; nessuna modifica alla configurazione effettuata.

Suggerimento **P3**: discovery strutturata degli endpoint e trasporti effettivi
per il client. Prima verificare le discovery gia' esistenti e documentarle;
non richiedere un secondo contratto. Accettazione: un nuovo supervisore trova
l'endpoint corretto senza indovinare la porta o parsare il log, rispettando
autenticazione e limiti dell'esposizione.

## EP-09 — Tipo sbagliato di trace_policy (caller error)

Il 28 settembre 2026 un POST /v1/runs con JSON sintatticamente valido ma
`trace_policy: "default"` e' stato rifiutato con HTTP 400,
`Bad Request: invalid json`, prima di ottenere un run ID. Il client PM aveva
usato un tipo errato: il contratto corrente accetta un oggetto, ad esempio
`{"content_mode":"refs"}`. Corretto il tipo, il POST ha accettato il run
`run-2e860898-dc04-4c39-9aa6-6f05a1a9d5f5`. Nessuna patch Matrix necessaria.

Suggerimento **P2**: distinguere errore sintattico JSON da errore di schema,
con field path/tipo atteso senza eco di body o valori. Accettazione: JSON
rotto resta errore sintattico; JSON valido con trace_policy stringa indica
`trace_policy`, tipo oggetto atteso, e non avvia il provider. Verificare e
riusare lo schema/discovery esistente prima di proporre un nuovo contratto.

## EP-10 — Completed non certifica accettazione o correttezza del report

MiniMax M3, run `run-2e860898-dc04-4c39-9aa6-6f05a1a9d5f5`, ha terminato
con modello attestato ma report non committato e confronto spazio errato:
967/986 MiB erano descritti sotto la soglia 512 MiB. Revisione mirata nella
stessa sessione, `run-1b4c7eb7-6a01-4c9d-bc54-e75085df2af0`, ha corretto
la premessa e pubblicato il commit `a463f997`. La provenienza di alcuni
database QA gia' rimossi resta incompleta: il generatore esatto non trovato
non autorizza a dichiararli rigenerabili per supposizione.

Errore dell'esecutore/limite di verifica, **non bug Matrix dimostrato**.
Il recapito terminale e la selezione modello funzionano. La review del PM
deve distinguere esito del protocollo da artefatto accettato: anche una
risposta completed richiede verifica di diff, prova, commit e input.

Suggerimento **P1**, facoltativo per il supervisore: dichiarare prima del run
un contratto di consegna con artifact path/hash e validatore deterministico
del chiamante, mantenendo distinti protocol_status e acceptance_status.
Accettazione: caso completed senza artifact/commit viene segnalato come
consegna incompleta, senza inventare un errore del provider e senza importare
transcript. Per cancellazioni: piano esatto e provenienza verificabile,
preferibilmente quarantena recuperabile; non un generico permesso basato
sul nome 'cache' o 'test'. Eventuali guardie native vanno verificate prima
di proporne di duplicate. Non e' stata modificata la logica Matrix.
