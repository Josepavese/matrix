# VICKY: finale OpenCode presente, run Matrix senza evidenza di turno

## Origine e perimetro

Nota di un **agente esterno**, proveniente dal goal VICKY/SDK Half Pocket.
In questo repository è stato scritto soltanto questo file non committato in
`issues/`: **nient'altro del repository è stato toccato**. Codice, installazione,
configurazioni e agenti Matrix non sono stati modificati. Non è una decisione
di accettazione o una correzione eseguita dal maintainer.

Data: **21 ottobre 2026**, Europe/Rome. Osservazioni native del 20 ottobre
2026 23:25–23:35 UTC. Tutti gli input sono sintetici, senza dati cliente.

## Ambiente e revisioni

- VPS dedicata VICKY, Ubuntu 24.04, Linux amd64.
- Core installer `0.1.116`; Agent Hub `0.2.2`, schema 10, inventario generazione15.
- Hub installato esclusivamente tramite Core con operazione
  `vicky-hub-model-evidence-022-20261021`; ricevuta `completed`.
- Artifact Hub SHA256
  `478a733ff7aca4310843d75beede3646feed0165ed4697c4c1e46ba446df572a`.
- Matrix nativo: `0.1.54`, commit
  `c6ee5fea5150b34a1d083fdf9af7bf394c28c9f5`, build `2026-10-09T08:19:20Z`.
- Checkout Matrix locale soltanto ispezionato: commit
  `f00f326ef57cef570599eabc8299303c5dae85e9`,
  `fix: keep local artifact installation separate from Vault initialization`.
  **Il difetto non è stato qualificato su questo HEAD né su un'altra release.**
- Agente OpenCode, protocollo ACP; provider diretto DeepSeek,
  selettore `deepseek/deepseek-flash`. Nessun fallback, OpenRouter o LLM simulato.
- L'OpenAPI OpenCode riporta una versione generica `1.0.0`: non è stata usata
  come prova della versione dell'eseguibile dell'agente.
- Hash del binario Matrix, configurazione Hub ed enrollment identici prima e
  dopo l'aggiornamento Hub0.2.1→0.2.2. Nessuna nuova configurazione agente.

## Osservazione

Una richiesta di estrazione Knowledge con lo schema reale
`vicky.document.polizza` (14 campi) e testo sintetico viene ammessa dall'Hub.
La proiezione dell'evento autenticato Matrix conserva:

```json
{
  "requested_model": "deepseek/deepseek-flash",
  "selected_model": "deepseek/deepseek-flash",
  "confirmed_model": "deepseek/deepseek-flash",
  "verification": "provider_confirmed",
  "verification_reason": "",
  "evidence_source": "session/set_config_option",
  "fallback_used": false,
  "fallback_reason": ""
}
```

Il run Matrix `run-ed2ada06-9e53-45be-9325-68a248d045ba` dichiara:

- stato `failed`;
- `failure_code=run_no_turn_evidence`;
- `stop_reason=end_turn`;
- `prompt_receipt=unverified`;
- causa: provider terminato senza output, tool call o messaggio.

Inizio run: `2026-10-20T23:25:37.226714329Z`;
aggiornamento terminale: `2026-10-20T23:25:55.829336877Z`.
La prima pagina nativa di 8 eventi contiene avvio, routing, prompt, identità
workspace, `model.selection` e tre `agent.thought.delta`; non è stata utilizzata
la reasoning privata come risposta o prova di completamento.

L'Hub termina correttamente il turno come `failed`, codice pubblico
`runtime_failed`, risposta vuota. Il consumer strutturato rifiuta con
`structured_hub_turn_failed`, senza creare risultati o ritentare la mutazione.

Una successiva osservazione **readonly** dell'API OpenCode, sul listener
loopback posseduto dallo stesso UID del workspace del runtime, mostra invece:

- sessione remota identica al `remote_session_id` del trace Matrix;
- directory sessione identica al workspace dell'enrollment;
- esattamente un messaggio utente e un messaggio assistant;
- byte del messaggio di estrazione utente identici a quelli inviati all'Hub
  (SHA256 confrontato, dopo il prefisso aggiunto dall'orchestratore);
- trace `context.kind=hub.turn` con conversazione/turno identici alla ricevuta
  Hub, senza ricavare l'associazione dal testo della risposta;
- assistant con `providerID=deepseek`, `modelID=deepseek-flash`, `finish=stop`,
  nessun errore; parti `step-start`, `reasoning`, **`text`**, `step-finish`;
- parte `text` finale di **1.579 caratteri**, JSON parsabile con
  `schema`, `requestSha256`, `fields`, 14 campi;
- SHA256 dei byte della risposta:
  `3f5ccfe2cde6449f98ca4dc9550170ce2f60899e420e110b9b4ec45dda32b224`.

Questo prova la discrepanza tra la consegna dichiarata da Matrix e il messaggio
finale nella sessione OpenCode. **Non identifica ancora se la causa sia
l'emissione ACP OpenCode, il trasporto, il consumo delle notifiche o una gara
alla terminazione.** Non è stata catturata una traccia completa del trasporto ACP.

## Problema distinto già gestito nell'owner Knowledge

Il modello ha usato `segmentId` negli ancoraggi invece della chiave di protocollo
`segment`. Il validatore lo rifiuta (`structured_fields`): la risposta presente
non è quindi un'estrazione accettabile, anche risolvendo la consegna ACP.
Knowledge rende esplicito `anchorShape` nel contratto e aggiunge la regressione
sull'alias errato; **non** introduce un alias/normalizzazione di compatibilità.
La chiusura di questa segnalazione Matrix deve provare la consegna della risposta,
non la correttezza semantica del suo JSON o dei dati assicurativi.

## Riproduzione e prove disponibili

Scratch privato di qualifica sulla workstation:
`/media/jose/Data/vicky-documents-20261020/provision/hub022/`.

- `live-input.json`: contratto/schema/testo sintetico esatti dell'invio originale.
- `live-journal.json`: conversazione, turno e chiave idempotente originali;
  stato dell'osservatore distinto dal terminale del run.
- `live-events.jsonl`: feed pubblico Hub con attestazione e fallimento.
- `structured_live.py`: harness del consumer attraverso Hub HTTPS, con sessione
  operatore esistente; **non** rappresenta IAM di un worker autonomo in produzione.
- `provider_observation.py`: osservazione readonly, verifica binding del trace,
  listener posseduto dal worker, sessione/directory e hash del messaggio.
- `observed-provider-output.json`: osservazione finale con exit0; contiene il
  testo sintetico finale, nessuna reasoning o credenziale.
- `native-before.json`, `native-after.json`, `native-plan.json`,
  `native-apply.json`: provenienza/installazione Hub e confronto binari/config.

Per l'ispezione nativa sono stati usati `matrix fs list runs` e
`matrix fs read runs/<percorso-semantico>/status.json` come utente worker,
quindi le API autenticate readonly `/v1/runs/<id>/explain` e
`/v1/runs/<id>/trace`. Token letti soltanto dal file protetto dell'enrollment
nel processo diagnostico remoto, mai stampati né copiati in questa nota.
Le prove restano nella loro custodia privata, non sono copiate nel repository.

## Richiesta e criteri di chiusura per il maintainer

1. Stabilire se il comportamento della release nativa indicata è già risolto;
   in tal caso indicare release, checksum/provenienza e prova reale pertinente.
2. Riprodurre e qualificare il tratto ACP→eventi Matrix, inclusa la consegna finale
   alla chiusura del turno e l'ordine degli eventi.
3. Preservare il guard `run_no_turn_evidence` per un turno realmente privo di
   output: `end_turn`, reasoning o modello configurato non bastano a completare.
4. Provare sia un finale testuale reale consegnato sia il caso senza alcuna
   evidenza; niente estrazione di reasoning come risposta o euristiche sul testo.
5. Conservare contratti generici Provider/Adapter e PAL; niente branch dedicato
   a VICKY, a questo channel o a una polizza.
6. Consegnare la correzione nel percorso Matrix ufficiale. Nessun intervento di
   questo agente su codice/installazione Matrix è autorizzato da questa nota.

Decisioni aperte: componente responsabile, eventuale fix già esistente,
release da qualificare e autorizzazione alla sua adozione sulla VPS VICKY.
Il percorso assicurativo completo resta non qualificato; il goal VICKY può
proseguire sui componenti indipendenti senza aggirare questo esito.

## Presa in carico maintainer — 21 ottobre 2026

**Accepted; qualificazione nativa ancora aperta.** Individuati e corretti
[difetti generici di consegna Matrix](2026-10-21-acp-delivery-barrier.md):
scadenza del drain con finale ancora in coda, overflow silenzioso, aggiornamenti
accodati assegnabili al turno successivo e guasto del collegamento dopo ack v2.
Regressione negativa sulla baseline f00f326 e positiva sulla correzione.

OpenCode locale 1.18.31, ACP stdio e DeepSeek diretto, con lo schema/testo
sintetico originale, consegna 14 campi e 1.577 byte prima di `end_turn`.
La selezione è confermata dal provider senza fallback. Nessun contenuto di
reasoning è stato usato come risposta o copiato nelle evidenze pubbliche.

Questo qualifica il percorso Matrix locale; **non identifica retroattivamente
la causa sulla VPS**. La versione nativa OpenCode non era registrata e manca
la traccia ACP dell'episodio. Restano da verificare adozione della release Matrix
corretta e consegna nativa sulla VPS, con inventario delle versioni e, se il
problema ricorre, aggiornamenti ACP correlati al termine del prompt. Nessun
workflow VICKY o deploy sulla sua VPS eseguito dal maintainer senza il mandato
specifico previsto da `improvements/issues/README.md`. La correttezza semantica
Knowledge resta responsabilità del consumer.


## Disposizione Matrix — 21 ottobre 2026

Gestita nel perimetro Matrix e archiviata. I difetti generici di consegna sono
corretti e la v0.1.56 installata ha completato il collaudo sintetico originale
sulla stessa sessione remota, con finale JSON di 14 campi e lettura dal Vault.
Il primo collaudo v0.1.55 aveva fallito esplicitamente per saturazione; entrambe
le misure sono conservate nei verbali, senza cancellare il risultato negativo.

L'utente ha affidato adozione e test della VPS al team esterno VICKY. Nessuna
operazione è stata eseguita sulla VPS. La causa esatta dell'episodio storico,
senza wire trace/versione agente, rimane non attribuita; l'archiviazione indica
il completamento delle correttive Matrix, non la qualifica esterna del flusso.

Evidenza: [verbale v0.1.56](../../docs/governance/releases/2026-10-21-v0.1.56.md).

## Qualifica esterna VPS VICKY — 21 ottobre 2026

**Risolta anche sulla VPS; confermata la chiusura della segnalazione.**
Aggiornamento dell'agente esterno VICKY, su mandato esplicito dell'utente:
modificati soltanto questa nota già archiviata e il relativo indice `issues/`.
Nessun sorgente, installazione locale o configurazione Matrix di questo
repository toccato; nessun commit/push Matrix eseguito dall'agente esterno.

Adottata sulla VPS la release ufficiale **Matrix0.1.56**, commit
`87918ff932be19431ce1cd2b07cadb50f3f820f2`, build `2026-10-21T08:57:32Z`.
Archivio upstream Linux amd64 SHA256
`176430aee74ade7ef995b4156627e6eb5fd6d9800205ad625e24d315e9ec4894`;
il suo binario coincide con quello locale e con l'eseguibile effettivo del
processo worker sulla VPS, SHA256
`a719a89d7f232f94d73c31b318671f803b4a65248e03825436be53fa5bf8e343`.

Consegna mediante pacchetto ufficiale Half Pocket Hub0.2.6, SHA256
`97f9ed880aaff5fb74fb4d3e2491cc26f7f50a559b783a7a7030bb28362cbf6f`,
Core0.1.119/generazione25, operazione `vicky-matrix056-hub026-20261021`
`completed`. Lifecycle ordinario con drain e backup dell'owner. Delta dal
pacchetto Hub0.2.5: soltanto `bin/matrix` e manifest di versione; binario Hub e
controller invariati, configurazione/enrollment con fingerprint identiche.
[Provenienza pubblica immutabile](https://software-preview.halfpocket.net/releases/vicky/matrix056-20261021/publication.json).

Nuovo invio esplicito di qualifica dopo il terminale fallito, nella stessa
conversazione Hub `e91da1261c30062f51fc889a68a6780c472b1042e160245d`:

- turno `788fa61ae1d288d299d53b53c7e60a90be5bf33024e19264`, `completed`;
- finale autenticato Hub di **1.569 byte**, SHA256
  `4c21fc72e1afe6aeb0dac35721e9986527597c25a6893e9956c3738d9c565b26`;
- modello richiesto/selezionato/confermato `deepseek/deepseek-flash`,
  `provider_confirmed`, evidenza `session/set_config_option`, nessun fallback;
- `Knowledge HubProvider` accetta il finale consegnato e `Extractor` valida
  lo schema corrente di 14 campi con citazioni: 9 estratti e 5 dichiarati mancanti;
  `complete=false`, `reviewState=unreviewed`, importi/date normalizzati verificati;
- nessun finale recuperato dall'API OpenCode, nessuna reasoning usata come risposta;
- aziende, profili, documenti, analisi, report e PDF confrontati prima/dopo e
  invariati; run/turno originali falliti preservati.

Questa prova chiude il criterio esterno di consegna, senza attribuire
retroattivamente la causa dell'episodio storico privo di wire trace. Le
regressioni locali del guard senza output restano quelle del maintainer; non
si dichiara un nuovo test negativo nativo non eseguito. Input esclusivamente
sintetico, stessa sessione operatore: non qualifica il servizio Knowledge nativo,
l'IAM worker autonomo, la revisione o la pubblicazione assicurativa completa.
Il goal VICKY resta attivo; nessun intervento sul percorso Mail.

Prova tecnica compatta:
`/home/jose/halfpocket/docs/platform-sdk/vicky-matrix056-proof-20261021.json`
e copia VICKY `docs/qa/matrix056-20261021.json`. Evidenze private limitate nel
banco `/media/jose/Data/vicky-documents-20261020/matrix056-20261021/`, senza
credenziali o reasoning copiati nel registro pubblico.

Correlazione nativa readonly aggiuntiva: trace Matrix del run
`run-333bcbc7-6e4e-4607-8b09-e6855e62c8b3` con `status=completed`, contesto
`hub.turn` associato esattamente al nuovo turno e `remote_session_id` identico
all'episodio originale (`ses_ea45d78afffeKltlUbxg7ZDGO5`). Non è stata aperta
una conversazione remota sostitutiva; nessun contenuto privato del trace è
copiato nella nota.

Prova condivisa Half Pocket:
[commit ee4d81f6](https://github.com/Josepavese/gestionale-ai-halfpocket/blob/ee4d81f61a0910c45491182f4c79f9011f137612/docs/platform-sdk/vicky-matrix056-proof-20261021.json),
origin/main verificato sullo stesso hash. Questo aggiornamento del registro
Matrix resta non committato dall'agente esterno; lo staging già presente del
team proprietario è preservato.
