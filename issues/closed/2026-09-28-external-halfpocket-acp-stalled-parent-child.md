# ACP MiMo/OpenCode: run ancora running senza avanzamento parent/child

**Decisione Matrix (2026-10-01): chiusa nel perimetro derivabile.**
`Trace.Stall` (`internal/logic/runtrace/stall.go`) espone da `/v1/runs/{id}/trace` -
l'export che il consumer gia' leggeva - ultima attivita' osservata, attesa corrente
e richieste pendenti, con attribuzione parent/child da confronto strutturale
(`Run.RemoteSessionID` contro gli altri `session_id` osservati, nessun nome di
agente). La vista si calcola dagli eventi GREZZI prima di `applyTracePolicy`: dopo,
un run in `content_mode=refs` perderebbe proprio l'attribuzione di sessione.
`window_truncated` e' esplicito - una finestra troncata puo' sottostimare, mai
inventare. L'attesa di approvazione umana e' visibile: il subscriber registrava solo
l'apertura e ora accoppia apertura/chiusura, a costo zero in `runapi`. 12 revert
comportamentali (R1-R12), verifica indipendente 4/4. Commit `664ac61`.

NON fatto, e non derivabile senza inventare: distinguere deadlock del client,
provider in retry/quota e attesa di risposta LLM. Quando il peer tace i suoi eventi
dicono solo che tace: la vista riporta `provider_turn` e non indovina. Idem lo stato
del watchdog (e' runtime, non trace). E non c'e' riproduzione end-to-end con agente
reale: lo scenario di prova resta quello halfpocket del 28 settembre.

Nota di un agente esterno, 28 settembre 2026. Questo file non tracciato e' la
sola scrittura nel repository Matrix: nessun codice, configurazione,
installazione, commit o push modificato. La responsabilita' dello stallo non
e' attribuita senza traccia ACP grezza.

## Ambiente e prova

Matrix installato 0.1.46, commit f0b97ab35e761cad6721a1ab985d3b51005f3e8e.
Il sorgente letto in sola lettura e' alla revisione
2864953dad43cb9e12892b0cc3bd95377f7e0f1f: non va confuso con il binario.
Due worktree Half Pocket esclusivi; POST /v1/runs async, trace content_mode=refs,
notifiche terminali native Unix SSE. Nessun dato di produzione coinvolto.

- MiMo run-0e1dd298-8d38-4924-bdef-16d818841c92, iniziato 15:43:35 UTC,
  xiaomi/mimo-v2.6-pro provider_confirmed. Parent remoto
  ses_ffe5f17c5064affekRykKO4J3F: assistant senza finish dalle 16:07:42 UTC;
  ultimo child ses_ffe5f1743db77ffeOTqzAe9EZR termina stop alle 16:23:37 UTC.
  Ultima modifica file circa 16:07 UTC; otto file WIP, nessun nuovo commit.
  Ultimo evento Matrix 2487 alle 16:07:42, tool completed.
- DeepSeek run-c1380b67-d11d-456f-8bed-49929a7cbf72, iniziato 19:13:31 UTC,
  deepseek/deepseek-flash provider_confirmed, API diretta. Parent remoto
  ses_f168fc1a4ffeR1WPgrdgaDJQ7Q resta nel tool task running; child
  ses_f168f5f83ffe376P4oq51chTSH ha quattro read running dalle 19:14:32 UTC,
  senza successivi aggiornamenti. Ultimo evento Matrix 2206 alle 19:14:27 UTC.
  Il worktree resta pulito. La permission parent alle 19:14:25 era resolved,
  quindi non e' dimostrata un'attesa di approvazione parent.

Alle 20:39 UTC entrambi sono ancora running senza completed_at significativo.
Il controllo e' puntuale, non polling; il trace contiene la finestra intera
di 1000 eventi mantenuti, non una prima pagina da scambiare per il tail.
Riscontro metadata-only in lettura dei DB provider: tempi, tipi tool, stato,
finish e modello, senza messaggi, ragionamenti, input/output o segreti.
I runtime figli Matrix esistono e non hanno processi tool discendenti: questo
non prova, da solo, che una richiesta di rete o un handler non sia in attesa.

## Recupero e decisioni gia' prese

Custodito WIP MiMo in bank privata sul disco Data, patch SHA256
77da2f8a89eb8079d149796108e95adf69caebb0c3e59957e946360385e35351.
Nessun reset o modifica concorrente. POST actions cancel puntuale sui due run
alle 20:42:27 UTC: accepted, provider cancellation signal sent;
run.cancel.signal completed e run.cancelled, entrambi recapitati ai due
osservatori nativi. Nessun riavvio daemon o kill globale.

Ripresa contestuale con medesimi modelli, scope esclusivi e richiesta di non
usare task/sottoagenti nativi per questo tentativo diagnostico. Non e' un fix
Matrix, ne' la dimostrazione che i sottoagenti siano la causa comune. Impostato
activity_timeout_seconds=900 nella nuova richiesta, da verificare nel runtime;
non dichiarato collaudato qui. Nessun fallback silenzioso o risultato accettato
perche' il provider esiste o il run e' running.

## Richiesta ai manutentori ed evolutive

1. Riprodurre parent task -> child con letture parallele ACP. Verificare
   correlazione sessionId/requestId, risposte fs/read_text_file, permission
   child, propagazione degli errori e prompt terminale. Distinguere deadlock
   client, provider in retry/quota e attesa di risposta LLM.
2. Esporre diagnostica nativa compatta: ultima attivita' osservata, attesa
   corrente e pending request/tool/permission per parent e child. Il consumer
   non dovrebbe leggere i DB privati per capire se un run avanza.
3. Verificare timeout di inattivita' end-to-end con run interrotto nel tool e
   dopo tool completed ma prima del prossimo delta. Esito terminale strutturato
   e wakeup solo con run_id/esito, non transcript; preservare sessione e WIP.
4. Chiarire nella documentazione che session active e run running non sono
   attestazioni di progresso, e offrire recupero cancellazione/ripresa senza
   doppio prompt o perdita del contesto.

Non abbiamo modificato Matrix o il provider per nascondere lo stallo. Servono
le prove grezze a monte prima di attribuire la causa o dichiarare una correzione.

## Ripresa riuscita, attestazione Matrix non valorizzata

Le riprese consegnano codice e terminali: DeepSeek
run-0499b7fb-f1b4-4f85-a88c-575c72e14f1a completed 20:50:35 UTC; MiMo
run-4ff9a3a3-c3ac-4dae-8430-3ae872b30231 completed 20:54:21 UTC. Stessi
remote_session_id parent precedenti, native Unix SSE recapitate a Codex.
Questo dimostra il recupero operativo di questi tentativi, non la causa.

Per entrambi il trace finale ha effective_model null e
model_verification unverified, sebbene sia richiesto lo stesso selettore e il
riscontro dopo il terminale sui soli metadata provider delle risposte finali
mostri rispettivamente deepseek/deepseek-flash e xiaomi/mimo-v2.6-pro. Non
viene inventata un'attestazione provider_confirmed. Verificare anche la
propagazione dell'attestazione modello nelle sessioni riprese; contesto e
consegna sono recuperati, ma il contratto di prova del consumer rimane incompleto.
