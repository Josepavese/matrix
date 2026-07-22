# Issue: Codex ACP perde whitespace nei delta e aggrega contenuti non finali nel messaggio finale

Data: 2026-07-22

Stato: closed

Origine: integrazione Half Pocket Desk

Provider osservato: Codex via ACP

## Sintesi

Durante l'uso di Codex attraverso Matrix, Half Pocket Desk ha ricevuto testo
con parole unite e un `agent.message.final` contenente, nello stesso payload:

1. un aggiornamento operativo intermedio;
2. la risposta conclusiva destinata all'utente;
3. un warning tecnico del runtime Codex.

Matrix non genera questi testi, ma l'attuale pipeline altera i confini dei
delta e non conserva una distinzione affidabile tra risposta finale e contenuti
accessori. Il consumer non puo' ricostruire correttamente tale distinzione con
euristiche generiche.

## Problema 1: whitespace perso ai confini dei delta

Esempio osservato nel portale:

```text
Leggoilcontestooperativorichiesto,senzamodificaredati.
```

Il metodo `appendMessageDelta` in
`internal/logic/runnotifier/notifier.go` applica attualmente:

```go
content := strings.TrimSpace(update.Content)
```

Il trimming di ogni singolo chunk e' distruttivo. Per esempio, i chunk:

```text
"Ciao "
"Jose"
```

vengono esposti come:

```text
"Ciao"
"Jose"
```

e il consumer che rispetta il contratto incrementale ottiene `CiaoJose`.
Inserire uno spazio nel frontend non e' una soluzione valida, perche' romperebbe
correttamente parole suddivise in sub-token, punteggiatura, newline e codice.

## Problema 2: semantica non affidabile di `agent.message.final`

Esempio completo osservato:

```text
Leggo il contesto operativo richiesto, senza modificare dati.Ciao Jose, sono qui. Nessuna modifica effettuata.Warning: Failed to save the conversation transcript; Codex will continue retrying. Error: thread-store internal error: No space left on device (os error 28)
```

Interpretazione dei tre segmenti:

- `Leggo il contesto...` e' un aggiornamento operativo intermedio;
- `Ciao Jose...` e' la risposta conclusiva destinata all'utente;
- `Warning: Failed to save...` e' una diagnostica Codex dovuta al filesystem
  locale pieno, non una parte della risposta.

Nella pipeline ACP, `simpleObserver.appendMessageChunk` in
`internal/providers/agents/router_observer_content.go` concatena ogni chunk in
un unico buffer tramite:

```go
o.content += text
```

`GetContent()` restituisce poi quel buffer, al netto dei soli think block;
`acp_adapter.go` lo assegna a `ConversationResult.Output`; infine
`internal/logic/runtrace/lifecycle.go` pubblica l'intero output come
`agent.message.final`.

Se ACP consegna aggiornamenti operativi, risposta conclusiva e diagnostica
come `agent_message_chunk`, Matrix li aggrega senza un confine semantico e li
presenta tutti come risposta finale. Nascondere i delta in Desk non risolve
quindi da solo il problema: anche il payload finale puo' essere contaminato.

Il warning sullo spazio disco nasce da Codex e non da Matrix. La problematica
Matrix e' la sua classificazione o propagazione dentro il canale canonico della
risposta finale.

## Comportamento atteso

- `agent.message.delta.message` conserva esattamente il contenuto incrementale
  ricevuto dal provider, inclusi spazi iniziali/finali e newline.
- Matrix non applica `TrimSpace` ai frammenti di messaggio. Eventuali
  normalizzazioni possono avvenire soltanto su campi non incrementali e senza
  alterare il contenuto ricostruibile.
- `agent.message.final.message` contiene soltanto la risposta conclusiva
  destinata all'utente.
- Aggiornamenti operativi, reasoning/progress e diagnostiche runtime sono
  esposti con eventi o metadati tipizzati separati e non confluiscono nel
  messaggio finale.
- Se il protocollo ACP usato non espone ancora una distinzione sufficiente,
  Matrix deve conservarne i blocchi/metadati disponibili oppure dichiarare
  esplicitamente il limite del contratto; il consumer non deve essere costretto
  a riconoscere frasi o warning tramite regex.

## Criteri di accettazione proposti

1. Con chunk `"Ciao "` e `"Jose"`, la sequenza eventi conserva lo spazio e la
   ricostruzione produce esattamente `Ciao Jose`.
2. Con chunk `"prima riga\n"` e `"seconda riga"`, il newline resta intatto.
3. Con sub-token `"con"` e `"testo"`, Matrix non inserisce artificialmente uno
   spazio e la ricostruzione produce `contesto`.
4. Un turno Codex con aggiornamento intermedio e risposta conclusiva espone
   l'aggiornamento come progress/delta tipizzato e soltanto la risposta nel
   `final`.
5. Una diagnostica runtime come `Failed to save the conversation transcript`
   non compare in `agent.message.final.message`.
6. Il contratto e' coperto da test automatici sia sul notifier sia
   sull'adapter ACP e documentato per i consumer di `/v1/runs/{id}/events`.

## Impatto sui consumer

Il difetto produce testi illeggibili e rende impossibile mostrare con certezza
la sola risposta conclusiva. Un workaround in Desk puo' nascondere lo streaming
o filtrare warning noti, ma non puo' determinare in modo generale dove finisce
un aggiornamento operativo e inizia la risposta finale.

## Perimetro di questo report

Questo file documenta il problema per il team Matrix. Non sono state apportate
modifiche al codice, ai test, alla configurazione o al repository Git di Matrix.

## Risposta maintainer

- Decisione: accepted.
- Motivo: il trimming dei chunk violava il contratto incrementale ACP; inoltre
  Matrix non conservava `messageId` e la fase strutturata pubblicata dal provider
  Codex canonico, rendendo il final semanticamente ambiguo.
- Scope: decoder ACP, observer/final aggregation, run notifier, documentazione
  eventi, protocol coverage e governance budget.
- Evidence: test notifier/adapter/decoder mirati e
  `bash scripts/deploy_preflight.sh` con `DEPLOY_PREFLIGHT_OK`; real-agent smoke
  sul provider Codex canonico con `end_turn`, `messageId`, fasi `commentary` /
  `final_answer` e token file/terminal verificati.

## Risoluzione

- `agent.message.delta.message` conserva ora esattamente whitespace e newline
  ricevuti; nessun `TrimSpace` viene applicato ai chunk.
- Il decoder ACP conserva `messageId`; l'observer propaga anche
  `_meta.codex.phase` come `message_phase` e `message_classification`.
- `commentary` viene esposto come `agent.message.progress`.
- `final_answer` viene esposto come delta finale e, quando presente, e' l'unico
  contenuto usato per `ConversationResult.Output`, content blocks finali e
  successivo `agent.message.final`.
- Un chunk senza fase ricevuto dopo un final esplicito diventa
  `agent.runtime.diagnostic`, non entra nel final ed e' nascosto dalla timeline
  frontend normale per default.
- Se un provider non espone alcuna fase finale, Matrix mantiene il fallback ACP
  append-only e marca i chunk `unclassified`; non tenta regex o inferenze sul
  testo. Se una fase finale arriva dopo chunk non classificati, il final usa
  comunque solo i chunk `final_answer`.

Test aggiunti:

- spazio finale: `"Ciao " + "Jose" == "Ciao Jose"`;
- newline: `"prima riga\n" + "seconda riga"` resta invariato;
- sub-token: `"con" + "testo" == "contesto"`;
- commentary/final/warning Codex separati nel notifier e nell'adapter ACP;
- round-trip ACP di `messageId` e contenuto esatto.

Real-agent evidence:

- agente: `codex`;
- entrypoint: `/home/jose/.nvm/versions/node/v22.12.0/bin/node`
  `/home/jose/.local/share/matrix/agents/codex/node_modules/@agentclientprotocol/codex-acp/dist/index.js`;
- provider: `@agentclientprotocol/codex-acp@1.1.4`, protocol version 1;
- comando: `MATRIX_SMOKE_TEST=1 MATRIX_REAL_ACP_MIN_PROVIDERS=1`
  `MATRIX_REAL_ACP_PROVIDERS='codex=env CODEX_CONFIG={"service_tier":"fast","model":"gpt-5.4","model_reasoning_effort":"xhigh"} /home/jose/.nvm/versions/node/v22.12.0/bin/node /home/jose/.local/share/matrix/agents/codex/node_modules/@agentclientprotocol/codex-acp/dist/index.js'`
  `go test ./tests/integration -run 'TestSmoke_RealACPProviderLifecycleCompliance/codex' -count=1 -timeout 6m -v`;
- risultato: PASS, `stop=end_turn`, due `messageId` distinti, fasi
  `commentary` e `final_answer`, token LLM/file/terminal presenti;
- capability osservate: `mcpCapabilities.acp=false`; `session/resume` dichiarato
  ma non applicabile alla sessione effimera senza rollout persistito.

Riferimenti upstream verificati il 2026-07-22:

- https://agentclientprotocol.com/protocol/v1/prompt-turn
- https://github.com/agentclientprotocol/codex-acp/blob/v1.1.4/src/ContentChunks.ts
- https://github.com/agentclientprotocol/codex-acp/blob/v1.1.4/src/CodexEventHandler.ts
