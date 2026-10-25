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
