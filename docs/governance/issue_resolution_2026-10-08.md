# Verifica completa delle segnalazioni Matrix — 2026-10-08

## Ambito e criteri

Baseline: `main` 5f45cac, release precedente v0.1.51. Verificati tutti i file
non archiviati di `issues/`, il relativo registro/revisione, la policy di
`improvements/issues/`, gli audit e il debito in `docs/governance/`, le analisi
locali in `.agent/local/project-analysis/`, roadmap e copertura dei protocolli.
GitHub non presenta issue aperte alla verifica del 2026-10-08.

La decisione riguarda l'invariante Matrix, non il funzionamento interno di un
progetto cliente. Gli originali sono conservati con una nota finale del
manutentore; le schede gestite sono archiviate in `issues/closed/`. Questa pagina
è il riepilogo corrente: i precedenti audit restano fotografie della loro data.

## Correzioni e sviluppi accolti

| Segnalazione / fonte | Decisione e intervento | Prova |
| --- | --- | --- |
| EP10 del 6 ottobre: quota durante il turno detta preflight | Bug confermato. `session/prompt` distingue `provider_api_error` / `provider_runtime_failed` da setup; evento `provider.runtime.failed`. Restano codice RPC e diagnostica redatta e limitata. | Errore RPC dopo un tool eseguito: evento runtime e un solo wakeup terminale; nessun reset inventato. |
| EP11 del 6 ottobre: silenzio e attese ambigue | Corretto l'abbinamento dei tool per sessione remota e la duplicazione degli upsert. `cause=unknown` separa una diagnosi dai fatti osservati; `pending_complete=false` esplicita una finestra incompleta. | Padre/figlio con stesso ID; risultato già ricevuto nella finestra; cronologia troncata. |
| Report del 7 ottobre: run bloccata senza notifica utile | Accolto un avviso generico opzionale: `activity_notice_seconds`, `run.attention_required`, `activity_unobserved`. Una notifica per intervallo di silenzio, riarmata dall'attività; nessuna cancellazione, retry o fallback automatici. `matrix run wait --on-attention` ritorna anche su questo avviso. | Timer, arresto, run ancora running, socket Unix nativo, successivo terminale e riconciliazione dopo crash. |
| Report del 7 ottobre: trace remoto presente ma mirror logico vuoto dopo cancel | Causa riproducibile: identità salvata solo al ritorno del turno. Ora salvata al `SetHeader` del provider, prima del risultato ordinato. Non implica che un harness possa riaprire la sessione. | Session manager reale con router causale: header, inspect prima del risultato, cancel, risultato senza ID; mirror conservato. |
| Cancel e callback tardivi, rilevato durante la correzione | Header, identità logica e modello aggiornati sotto lo stesso lock delle transizioni terminali; non possono riscrivere una copia stale `running` sopra un cancel. I decoratori conservano la correlazione logica. | Aggiornamenti concorrenti e cancel: stato terminale e unico wakeup conservati. |
| Report del 7 ottobre / backlog aprile: stderr voluminoso e retry | Capture raw limitata, riga limitata a 4096 byte, massimo 64 righe per processo più riepilogo di soppressione; diagnostica e messaggi di errore limitati/redatti, taglio marcato. | Riga da 1 MiB, 10.000 retry, segreti in JSON e diagnostica. |
| EP01 / audit fase 7: nuovo agente supervisionato dopo startup | Per gli agenti ACP ws gestiti, prima risoluzione avvia la supervisione usando il contesto del daemon; prenotazione evita doppi figli. Non sostituisce configurazioni di un figlio già attivo durante il turno. | Otto risoluzioni concorrenti dopo registrazione: un solo avvio; daemon fermo rifiuta l'avvio. |
| Backlog aprile: endpoint esterni generici | ACP ws/unix senza comando usa l'indirizzo dichiarato; supervisor/doctor non richiedono un eseguibile locale inesistente. Un transport non implementato rimane un errore esplicito, non viene inventato. | Endpoint esterno risolto senza spawn; managed e transport sconosciuto distinti. |
| Audit sicurezza G12: valore cifrato spostabile tra chiavi | Confermato e corretto: ENCV2 lega il valore alla chiave con AAD. Conversione una tantum transazionale di ENCV1/plaintext, backup compatto verificato e check dello spazio prima delle scritture. Nessun decoder legacy nel percorso normale. | Sostituzione fra chiavi rifiutata; backup originale verificato, rollback totale su errore, conversione idempotente e rifiuto per spazio insufficiente. |
| CLI wait/ack JSON su stderr, rilevato nella prova reale | Corretto il fallback Cobra: risultati su stdout, diagnostica su stderr. | Eseguibile reale, wait con attention e ack ripetuto idempotente; test senza SetOut. |
| Documentazione ACP v2 e registri storici | Rimosse affermazioni attuali false: v2 negoziato esiste, archivi ora versionati, EP10 storico della revisione distinto da EP10-20261006. Gli snapshot upstream conservano la data. | Codice e suite v2 con processi stdio di test; nessuna attestazione commerciale v2 dedotta. |

## Tutte le schede di feedback

| File nell'archivio `issues/closed/usage-feedback-halfpocket/` | Esito |
| --- | --- |
| `ep-01-agent-acp-native-registration.md` | Accolto: rimedio registrazione, registro vivo e nuova supervisione. DSH mancante nel catalogo locale non dimostra un errore generico. |
| `ep-02-mimo-cwd-via-env-wrapper.md` | Già corretto: cwd strutturale per run e child; workaround ritirato. Accettazione reale del 2 ottobre conservata; nuovo smoke reale con file casuale. |
| `ep-03-workspace-id-inventati-vs-add.md` | Mapping e fail-closed già corretti. ID inventati dal chiamante respinti correttamente. EP03.C commit nel ROOT: attribuzione a Matrix non dimostrata, non cancellata l'evidenza cliente. |
| `ep-04-cli-vault-get-typed-parse.md` | Getter di stringa su record strutturato era uso errato. Summary e limiti CLI già introdotti; non si rende ogni getter un parser permissivo. |
| `ep-05-sse-run-notifications-positivo.md` | Funzione già operativa: socket, cursor, replay e ack; domanda elicitation limitata. Estesa con avviso nonterminale esplicito. |
| `ep-06-model-request-vs-actual.md` | Requested/selected/provider-confirmed e fallback opt-in già implementati. Valori env/header redatti salvo richiesta esplicita. Il nome fisico/fatturato del modello non è attestabile da Matrix. |
| `ep-07-quota-429-retry-after.md` | Propagazione errore strutturato e avviso accolti. Le osservazioni successive confermano quota/retry, non un `Retry-After` strutturato. Nessun orario UTC dedotto dalla prosa, retry o fallback impliciti. |
| `ep-09-mimocode-live-context-unsupported-2026-09-29.md` | Doctor distingue compatibilità, accettazione provider e prova di delivery. Limite MiMo live-attach conservato come limite del provider; non un bug da aggirare generando una nuova conversazione. |
| `ep-10-mid-run-quota-classified-preflight-2026-10-06.md` | Corretto; ID canonico EP10-20261006, distinto dall'EP10 storico della revisione PM (contratto di delivery). |
| `ep-11-long-silence-stall-observability-2026-10-06.md` | Corretto entro i dati osservabili: finestre, sessioni, causa sconosciuta e avviso; nessuna causa provider inventata. |
| `README.md`, `registro-episodi-2026-09-28.md`, `REVISIONE-PM-2026-09-28.md` | Interamente revisionati e conservati. EP08 porta errata e EP09a tipo `trace_policy` errato erano errori del chiamante; discovery/schema già corretti. EP09b è il limite MiMo; EP10 storico è coperto dal contratto generico di delivery. |

Le due schede root del 2 e 7 ottobre sono archiviate nello stesso `closed/`.
Le note del 1 ottobre già chiuse ricevono il riferimento all'accettazione del 2
ottobre: il vecchio «NON fatto» resta testo storico, non stato corrente.

## Proposte rifiutate e motivi

| Proposta / interpretazione | Motivo |
| --- | --- |
| Leggere database, log privati o request body del harness per riconoscere quota e retry | Accoppiamento a implementazioni esterne, rischio di importare conversazioni/segreti, nessun contratto ACP/A2A. Matrix usa eventi strutturati e ammette ciò che non osserva. |
| Trattare ogni HTTP 429 come credito esaurito; convertire «reset 02:32» in UTC; retry/fallback automatici | 429 può essere concorrenza o rate limit; senza timezone e campo strutturato la scadenza è ignota. Retry/fallback modificano consumo, modello e continuità senza una decisione esplicita del chiamante. |
| Esportare il saldo residuo come se fosse una proprietà standard ACP/A2A | L'uso del contesto/costo del turno non è il saldo del piano. Nessun dato osservato nei report fornisce un saldo comune. Un eventuale connettore di account è una funzione separata, opzionale e con credenziali proprie. |
| Recupero remoto garantito quando il provider rifiuta load/resume/live context | Matrix conserva identità e diagnostica e può tentare solo operazioni pubblicate dal peer. Una nuova conversazione non viene presentata come riuso remoto riuscito. |
| Derivare branch/HEAD/common git dir, gestire checkpoint Git e validare deliverable specifici HalfPocket | Matrix orchestra messaggi/sessioni/workspace; non gestisce il workflow VCS del progetto cliente. Sono disponibili metadati del chiamante e contratto generico di delivery. |
| Non conservare alcuna API key / rendere ogni record vault pubblico | Il vault cifrato è la SSOT delle credenziali autorizzate. Si elimina l'esposizione involontaria, non la capacità di autenticare i provider. |
| Sandbox OS per ogni agente/validatore e quota disco/capacità globale | Richiedono una policy di isolamento e risorse di sistema, non una correzione locale di osservabilità. L'esecuzione autorizzata resta sul conto utente; non si promette isolamento inesistente. La capacità del progetto cliente resta al suo orchestratore. |
| Fork SDK A2A / riscrittura dei body per emettere `artifacts: []` | Deviazione SHOULD già documentata e testata nell'SDK fissato; il MUST di omettere artifacts quando esclusi è rispettato. Il costo di mantenere un fork non è giustificato da un guasto osservato. |
| Riscrivere FUSE come filesystem semantico, introdurre collector e nuovi sink OS | Roadmap futura senza guasto attuale né requisito di accettazione. Le superfici native e il logging con retention coprono i problemi riportati. |

## Audit e analisi fuori da `issues/`

| Fonte | Verifica / stato corrente |
| --- | --- |
| `.agent/local/project-analysis/backlog-tecnico.md` (12 proposte) | Shutdown tramite signal provider, auth, errori tipizzati, wiring app, configurazione SSOT, unix, endpoint esterni, code ordinate, push/SSE, rimozione tipi morti, stderr e NVM sono presenti. Rafforzati errori runtime, endpoint, stderr e continuità. Migrazione wholesale di ogni `fmt.Errorf` senza difetto è respinta. |
| `cosa-manca.md`, `stato-attuale.md`, `fix-completati-2026-04.md` | Vault cifrato, backup/restore, amministrazione persistente, doctor/readiness, governance dei test e installer esistono. FUSE è sperimentale; un servizio esterno reale Telegram non è certificato da prove offline. |
| `test-findings-open-issues.md` | Lock readonly CLI, primo turno lento e output vuoto sono snapshot aprile, superati dalle correzioni storiche. Non promettiamo latenza universale: nuovo probe MiMo misura il proprio caso; timeout del client sincrono cancella legittimamente la richiesta. Usare async/wait. |
| `roadmap-operativa.md`, `ssot-architecture-roadmap.md`, `logging-ssot-roadmap.md`, `README.md`, `filosofie-goal-non-goal.md` | Riconciliati con codice e roadmap. Non sono ticket attivi. Sink secondario, isolamento OS e test Telegram con account reale rimangono sviluppi/qualifiche separati. |
| `phase3_audit_2026-10-01.md` | Body limitato, schema, delivery, revert e tetti già corretti; mantenuta provenienza storica. |
| `phase7_audit_2026-10-02.md` | Env allowlist e redazione CLI già presenti; completati late supervised enable ed endpoint esterni. Finestra ack opt-in, domanda 200 rune e config viva sono scelte dichiarate, non prove di carico/sandbox. |
| `security_review_2026-09-22.md`, `security_threat_model_2026-09-23.md` e issue sicurezza chiusa | Controlli/auth/tool gating/integrità/redirect/tempfile/shell/config già corretti dalle release successive; G12 ancora presente viene corretto con AAD e conversione sicura. Pinning npx/uvx è responsabilità della distribuzione dichiarata. Revisione umana, soak e qualifiche Windows complete non possono essere dichiarate eseguite da questa verifica. |
| `agnosticism_audit_baseline_2026-10-01.md`, `code_debt_register.md`, `mutation_baseline_2026-09-23.md` | Confini provider neutrali preservati; crescita funzionale misurata, nessun aumento delle soglie di warning/branch/file/funzione. Mutation score è uno snapshot, non un bug funzionale. |
| `docs/protocol_coverage.md`, roadmap prodotto e timeout policy | Copertura v2 aggiornata rispetto al codice; limite SDK artifacts esplicito. Nessun timeout assoluto di default. Retry di dial Unix/vault e grandi fasi future restano watch-item senza guasto corrente riprodotto. |
| `improvements/issues/README.md` e commenti sorgente | Policy, nessuna scheda aperta. Nessun TODO/FIXME applicativo nel codice corrente. Nessun workflow esterno eseguito per collaudare Matrix. |

## Evidenze e limiti del collaudo

- Test causali con race detector: runactivity, providerdiag, providerfailure,
  runtrace, session, agentmgr, runapi e CLI.
- Smoke reale MiMo ACP su workspace temporaneo: `session/new`, resume,
  selezione esplicita `deepseek/deepseek-flash`, tool call e lettura di un token
  casuale non presente nel prompt; `end_turn`, 6,46 s. Il primo tentativo con
  `--model` su `mimo acp` è stato rifiutato dalla CLI; la selezione corretta usa
  il metodo ACP. Nessuna conversazione utente modificata.
- La conferma della selezione non attesta il modello fisico/fatturato.
- Nessuna quota è stata consumata artificialmente per provocare un 429;
  regressione quota riprodotta con errore RPC causale, distinta dalla prova reale.
- Stato consegna, CI, release e installazione: completamento registrato nel
  verbale della release. Fino a tali verifiche il codice resta candidato.


### Prova integrata sul daemon reale isolato

Il nuovo eseguibile ha eseguito run asincrone con MiMo/DeepSeek diretto usando
il socket di notifica nativo e le API autenticate. Risultati: attention mentre
la run resta running; successivo completamento; ack e retry dello stesso ack
senza duplicazione; header remoto visibile prima del cancel; cancel terminale;
import esplicito della sessione di prova e ripresa strict con **stesso ID remoto
e logico**, selezione `provider_confirmed` e nuova lettura dell'artefatto casuale.
La prova è ripetuta dopo la conversione del vault isolato. Nessun lavoro del
progetto HalfPocket, nessuna conversazione utente e nessun provider di fallback.

La precedente selezione SDK accettata è separata da questa attestazione Matrix.
Il formato ENCV2 è letto solo dalla nuova versione: un rollback applicativo usa
il backup precedente insieme alla chiave originale. La copia conserva record
cifrati/segreti e non va pubblicata; resta nel PAL home o nella directory assoluta
scelta esplicitamente con `MATRIX_VAULT_MIGRATION_BACKUP_DIR`.
