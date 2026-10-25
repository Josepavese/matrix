# VICKY: turno di valutazione JSON interrotto da permission_denied senza operazione

Nota di agente sviluppatore esterno Half Pocket/VICKY, 25 ottobre 2026.
Ho scritto soltanto questa segnalazione non committata in `issues/`.
Nessun sorgente, installazione, commit o remoto di questo repository è stato modificato.

## Ambiente verificato

- Consumer reale: VICKY, VPS dedicata, Hub 0.4.11/schema13, SDK ufficiale 0.56.0-dev.1.
- VICKY backend51/web50/lettore29, API0.22, SQLite14, inventario Core65.
- Sorgente del lettore corretto: `af4646d` nel repository VICKY.
- Matrix eseguito dalla release Hub: `matrix 0.1.59`, commit
  `2f10dd98c217ad14344172514213b572661ba69b`, build `2026-10-23T01:18:20Z`.
  Versione letta sul VPS con `/var/lib/agent-hub/current/bin/matrix version`.
- Checkout Matrix consultato in sola lettura: `499e063cdcd8a9ae2f7f0fb132662f4b0154a052`.
  Non è stato assunto equivalente al binario VPS.
- Modello richiesto e confermato: `deepseek/deepseek-flash`; selezione
  `provider_confirmed`, nessun fallback. Catena Hub -> Matrix -> OpenCode -> API DeepSeek.
- Solo documenti e aziende sintetici dichiarati. Nessun dato cliente reale nel caso.

## Esito reale e riferimenti conservati

Il lettore conserva materiale, interpretazioni e risposte nelle ricevute immutabili
VICKY. Un job può iniziare una sola inferenza nuova; il successivo riprende gli
stessi input e risultati. Una chiamata dal risultato incerto non viene duplicata.

1. Job `70e12e8c-0fc0-4c75-a4dd-beb334e18e2b`, attempt1, `completed`.
   Quattro interpretazioni salvate, una risposta di estrazione, esito `continuing`.
   Tre documenti riusati da revisioni pubblicate e un originale nuovo interpretato.
2. Job successivo `0d6ba3d0-473f-43a8-a6b5-da2896dc84f8`, attempt1,
   `needs_reconciliation`, diagnostico consumer `provider_command_incomplete`.
   Creato1792954893605, aggiornato1792955030637: circa137 secondi,
   quindi non è dimostrato un timeout del budget300s.
3. Stessa prenotazione owner `70e12e8c-0fc0-4c75-a4dd-beb334e18e2b`:
   quattro documenti completati, una inferenza completata e una senza risultato
   acquisito, nessuna proposta ammessa. Input correnti, dati e report5 preservati.
4. Conversazione del secondo job:
   `e02ae5cf2244f7d5ab80a9b93fdc6a92fc4f733b1ac10b4e`.
   Turno `e9268b9ec5c71f33a4ca09519d7a5628eabeeedf82f63e2b`,
   position38, terminale `failed`, `error_code=tool_permission_denied`, answer vuoto.

Eventi letti dall'API owner Hub, filtrati per il turno esatto:

```json
{"sequence":188,"kind":"running","data":{"state":"running"},"created_at":1792954900925}
{"sequence":190,"kind":"permission_denied","data":{"operation":""},"created_at":1792955028777}
{"sequence":191,"kind":"failed","data":{"answer":"","code":"tool_permission_denied","state":"failed"},"created_at":1792955029775}
```

L'evento189 conferma il modello richiesto. **L'operazione è vuota**:
non ho dedotto che fosse `pwd`, una shell, una lettura o altro. Non ho ancora
il run_id Matrix o il remote_session_id dal contratto pubblico Hub del turno.
Questa lacuna di osservabilità va distinta dalla policy di diniego.

## Cosa è stato escluso nel consumer

Durante il primo collaudo erano emersi due difetti VICKY, già corretti:
ricostruzione del tipo pubblico SDK `ProviderResult` per i risultati ripresi e
conservazione dei metadati completi della fonte del parser. Non sono attribuiti
a Matrix.93 test worker passano e la stessa risposta reale di estrazione è stata
rigiocata offline sul VPS attraverso SDK056 e il consumer corretto, senza nuove
chiamate modello o scritture owner. Il primo job29 completa il documento e cede
il passo come previsto. Il blocco successivo è un turno distinto, di valutazione,
che l'API Hub dichiara esplicitamente fallito per permission_denied.

## Contratto necessario / decisioni aperte al proprietario

Il caso deve produrre JSON da dati tipizzati già forniti: non richiede accesso
al filesystem o esecuzione di comandi. Serve chiarire e qualificare il percorso
corrente per **inferenza senza strumenti nativi**, circoscritto al run/ruolo:

- Se Matrix/OpenCode lo supporta già, indicare configurazione e contratto pubblici
  da consumare in Hub, senza modifiche globali o deduzioni da variabili locali.
- Altrimenti esporre una capacità esplicita che non annunci strumenti al modello
  per questo ruolo, mantenendo gli agenti operativi distinti. Un suggerimento nel
  prompt non garantisce deterministicamente l'assenza di chiamate a strumenti.
- Chiarire se il diniego ACP debba essere una risposta negativa al provider o
  un errore terminale del run. Il diniego osservato può essere corretto: non è
  stato dimostrato che Matrix abbia concesso un'operazione o violato la policy.
- Conservare una diagnosi strutturata utile: operazione/categoria se conosciute,
  stato esplicito se sconosciute, riferimento al run e alla sessione; non inferire
  comandi da testo libero né esportare segreti o prompt integrali.
- Consentire al chiamante di distinguere un turno terminale fallito da un effetto
  ancora incerto, così da riconciliare l'intento senza duplicare inferenze.

Decisioni già prese: nessun ampliamento dei permessi per sbloccare il test;
nessun modello alternativo, API diretta che salti Matrix/OpenCode o bypass del
diniego; nessun nuovo messaggio del modello mentre si osserva questo handle.
Input, ricevute, job e turno falliti sono conservati. Il precedente test tecnico
`bd1ba739-8000-428b-9bb4-8b6a852f5e00` è un'altra esecuzione, già chiusa con audit
per i difetti consumer: non va confuso con questo blocco pendente.

## Prove e criteri di accettazione

Prove private sulla workstation:
`/media/jose/Data/vicky-documents-20261020/completion-20261025/`:

- `preparation-live-job-details.json`: due job e ricevuta `continuing` reale;
- `preparation-current-stage-metadata.json`: legame fra job e fasi della stessa richiesta;
- `preparation-second-turn-owner-history.json`: turno e prompt sintetico esatto;
- `preparation-planning-turn-events.json`: sei eventi owner del turno;
- `preparation-vps-matrix-version.json`: versione del binario effettivo;
- `preparation-offline-replay-proof.json`: difetti consumer già isolati e corretti.

Non copiare prompt o credenziali nel repository: il prompt sintetico resta nel
banco privato e nell'owner Hub. Nessun payload cliente è necessario per riprodurre.

Qualificare il percorso dichiarato con il provider reale, il modello effettivo,
la policy degli strumenti e l'esito JSON. Se si esercita intenzionalmente un
diniego, verificarne stato e diagnosi senza spacciarlo per analisi completata.
Poi verificare dal consumer VICKY la ripresa governata e l'ammissione di una
proposta non revisionata; revisione umana e pubblicazione restano passaggi separati.

## Disposizione Matrix — 25 ottobre 2026

**Decision:** accepted in part and closed with declared limits. Due capacita'
generiche, entrambe per endpoint del ruolo e non globali: un **modo di sessione
ACP obbligatorio** applicato e verificato dal provider, e una **diagnosi
strutturata** del diniego con esito terminale distinto dall'incerto. Pubblicate
nella release immutabile v0.1.60. L'accettazione non impegna Matrix a garantire
cio' che non puo' osservare, ne' a costruire ogni parte della richiesta: sotto
e' indicato cosa e' stato costruito, cosa e' stato risposto con un limite e cosa
resta al consumer.

**Reason:** il turno di valutazione non richiede filesystem ne' comandi; il
difetto non era la policy di diniego, era che (a) mancava un modo deterministico
per dichiarare al ruolo "nessuno strumento nativo" senza suggerimenti nel prompt,
e (b) un turno terminato dopo un diniego senza risposta veniva riportato
`completed`, con l'evento `permission_denied` privo di operazione e nessun codice
che distinguesse il terminale fallito dall'effetto incerto. Un diniego corretto
resta una risposta negativa al provider: Matrix non ha concesso nulla e non lo
dichiara come analisi completata.

**Scope:** contratto `MATRIX_ACP_SESSION_MODE` per endpoint (ACP-only, un solo
modo, validato al lancio, rifiutato se duplicato o in conflitto con il contratto
Codex); applicazione e verifica da parte dell'adapter ACP su `session/new` e su
ogni ripresa, con conferma dichiarata dal provider e fail-closed prima del
prompt; eventi `permission.requested`/`permission.resolved` con `operation`,
`operation_state` (`known`/`unknown`, sempre presente), `tool_call_id`,
`session_id`, `denial_reason` e `provider_response`; nuovo codice di fallimento
provider `provider_tool_permission_denied`; regola di esito terminale in
runtrace; `explain` EN/IT. Documentazione pubblica:
[docs/matrix_agent_session_mode.md](../../docs/matrix_agent_session_mode.md).

**Evidence:** qualifica reale sul binario **v0.1.60 installato**, `MATRIX_HOME`
isolata, OpenCode 1.18.31 e `deepseek/deepseek-flash` confermato dal provider
senza fallback, entrambi i run attraverso l'API pubblica autenticata
(evidenze in `/tmp/matrix-v0.1.60-qualification/`, sintesi
`20-qualification-summary.json`):

- **Inferenza senza strumenti**, run
  `run-0f41a069-a3df-4483-a0a0-9bb047967214`: `completed`, `end_turn`, output
  JSON con `summary`/`steps`/`risks`, **zero** eventi `permission.requested` e
  `permission.resolved`. Il contratto di lancio registra
  `configured=json-planning`, `enforcement=required`; la sessione registra la
  conferma del provider `effective_mode=json-planning`,
  `evidence_source=session/set_config_option`, `verification=provider_confirmed`.
  Sessione logica `5b4f4e77-bc29-4c2c-b336-f4ee71071a7b`, remota
  `ses_e8b2283d8ffeioiJVMcSHeXq0B`.
- **Diniego intenzionale** (`agent.trust_mode=false`), run
  `run-5e774d34-389d-48bc-a47d-7942cadce009`: `failed`,
  `failure_code=provider_tool_permission_denied`, `stop_reason=cancelled`,
  nessun `run.completed`. L'evento porta
  `operation=pwd`, `operation_state=known`, `session_id` e
  `tool_call_id=call_00_a3569vp5pNPqsrt3vB502875`; la risoluzione riporta
  `decision=denied`, `provider_response=selected`,
  `denial_reason=trust_mode_off`. `explain`: il turno e' terminale,
  `uncertain=false`, `prompt_receipt=unverified`.
- **Contrasto con v0.1.59**: lo stesso diniego sull'installazione precedente
  (run `run-daa3bde9-a832-48ae-b49c-9f5569d115df`) produceva
  `permission.resolved` con il solo `{"decision":"denied"}` e un run
  `completed`. Quel difetto e' il caso chiuso da questa release.

Regressioni: validazione del contratto di modo, applicazione/verifica su
entrambe le superfici ACP, fail-closed, diagnostica permessi v1/v2, operazione
nota/ignota con stato esplicito, motivi di diniego, classificazione provider,
regola di esito terminale, explain EN/IT e test opt-in reale OpenCode. Gate:
`scripts/release_gate.sh` = `RELEASE_GATE_OK` sul commit
`9fdf7e5`, CI esatta
[40232619150](https://github.com/Josepavese/matrix/actions/runs/40232619150)
verde, release
[v0.1.60](https://github.com/Josepavese/matrix/releases/tag/v0.1.60) verificata
(88 controlli, 0 falliti) e installata con `readiness --strict` = `ready`.
Verbale: [2026-10-25-v0.1.60](../../docs/governance/releases/2026-10-25-v0.1.60.md).

**Limits (non osservato = non superato):** il contratto non inventa un
`provider_command_incomplete` ne' un errore ACP strutturato dove il provider non
lo espone: la classificazione provider legge il testo con veto di
autenticazione. La regola di terminale richiede il `denied` strutturato e non
deduce un diniego dal testo; un `agent.message.*` o un output non vuoto conta
come consegna. I record gia' memorizzati come completati ricevono spiegazione e
codice corretti, ma lo stato memorizzato non viene riscritto. Il corpo HTTP
sincrono di errore mantiene la forma generica (HTTP 500, nessun campo `code`):
il codice stabile e' sull'evento `run.failed`, sulla notifica locale e in
`explain`. Nessun intervento su VPS VICKY, Hub o consumer: `run_id` e
`remote_session_id` del turno devono essere esposti dal contratto pubblico Hub,
e la ripresa governata con ammissione di una proposta non revisionata resta il
passo consumer. La qualifica Matrix non sostituisce la revisione umana.

### Cosa e' stato costruito, limitato o non costruito

- **Costruito (generico e verificabile):** contratto `MATRIX_ACP_SESSION_MODE`
  per endpoint, con applicazione su `session/new` e ripresa, verifica della
  dichiarazione del provider e fail-closed prima del prompt; diagnostica
  strutturata dei permessi (`operation`, `operation_state`, `tool_call_id`,
  `session_id`, `denial_reason`, `provider_response`); codice
  `provider_tool_permission_denied`; regola di esito terminale sulla sola
  condizione strutturata diniego-senza-risposta; `explain` EN/IT; documentazione
  pubblica e qualifica reale sul binario installato (run
  `run-0f41a069-…` e `run-5e774d34-…` sopra).
- **Risposto con un limite:** la «capacita' che non annuncia strumenti al
  modello» e' il modo di sessione del provider applicato e confermato, **non**
  una garanzia di Matrix sul corpo della richiesta: Matrix non ispeziona il
  payload del provider e non lo dichiara. Il `run_id`/`remote_session_id` del
  turno e la ripresa governata restano il contratto pubblico Hub/VICKY; la
  qualifica Matrix non sostituisce la revisione umana ne' l'adozione consumer.
- **Non costruito, per scelta:** nessun codice o nome dedicato a VICKY, nessun
  alias del codice consumer `tool_permission_denied`, nessun branch dedicato al
  provider o al canale, nessuna euristica sul testo libero per dedurre
  l'operazione, nessuna riscrittura degli stati storici gia' memorizzati, nessun
  ampliamento dei permessi o bypass del diniego. Un rifiuto legittimo resta una
  risposta negativa al provider: la regola terminale non lo trasforma in errore.

I limiti tecnici sopra restano la specifica di cio' che e' stato costruito; questa
ripartizione e' la decisione di cosa non e' stato costruito.
