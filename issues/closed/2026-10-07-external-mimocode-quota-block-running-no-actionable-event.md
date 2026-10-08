# Nota esterna: goals MiMoCode in quota/attesa restano running senza blocco operativo recapitato

Nota di un agente esterno al progetto Matrix durante il collaudo Half Pocket.
Non ho modificato codice, installazione, configurazione, commit o remoto di
Matrix. Questa e l'unica scrittura nel repository, non versionata in issues/.
Le azioni native sui run sono state eseguite come consumer del servizio.

## Ambiente e osservazione

7 ottobre2026, Matrix0.1.51 commit7854f5b15c61938c1d597368a5704ccc1527488d,
MiMoCode0.1.15 ACP, workstation Linux. Goals nativi con modelli espliciti;
MiMo solo Token Plan, nessun fallback PAYG. Notifier SSE nativo sottoscritto
prima dispatch, payload terminale minimo, nessun transcript negli eventi.

- run-a5e3818b-409a-453b-a8f6-bdde2b4b91a2, xiaomi/mimo-v2.6-pro:
  log del PID173762 riporta AI_APICallError HTTP429 retryable, risposta quota,
  ripetuta03:34–03:39UTC. Ultimo record03:43:33 incompleto. Trace dice running,
  waiting tool_result, finestra troncata, ultimi eventi01:19UTC.
- run-0bab8618-f9d7-473b-9f25-86584a18f83e, MiniMaxM3.1 coding-plan:
  PID771926, AI_APICallError429 retryable/crediti03:39UTC. Ultimo record
  03:44:08 incompleto. Trace running/tool_result, ultimi eventi01:52UTC.
- GLM reviewcb87b651 e DeepSeek49abbbcc hanno processi vivi ma nessun nuovo
  record dopo03:42UTC; questo non dimostra che la causa sia la stessa quota.

Alle07:31–07:45UTC i processi sono S/ep_poll, non T/stopped; i run riportano
running. Gli observer locali erano andati persi, poi riarmati sugli stessi
run/cursori: questa perdita non e stata trattata come arresto degli agenti.
Non e provato che la finestra trace troncata descriva l'ultima attesa reale.

La diagnostica non ha stampato requestBodyValues, responseHeaders, prompt,
contenuti, token o credenziali. Solo classi, HTTPstatus, categorie errore,
timestamp, PID e identificatori. I log provider contengono record grandi
con requestBodyValues: valutare budget/redazione senza perdere diagnostica.
Non e stata accertata una fuga di credenziali.

## Recupero effettuato dal consumer

Checkpoint preservati, WIP verificato: hosting0000e4f1 con modifica propria
hosting_deploy.py e quattro hook altrui intatti; Maild56873dd pulito.
POST actions cancel sui soli due run in quota: HTTP202/cancelled e notifier
run.cancelled3785/3786. Sessioni/stato storico non cancellati, nessun nuovo
writer prima del terminale. MiMo sospeso per credito esaurito confermato
dal titolare; MiniMax ripreso dopo conferma disponibilita. Modello esplicito,
nuovo run ordinario di lavoro, nessuna generazione artificiale per saldo.

Dopo cancel, session inspect della logicalSession110b91d1… ha agent_session_id
vuoto, mentre il trace conserva remote ses_ffe5eec2ba00affeUcOBj4f410.
Il consumer ha usato handoff checkpoint/documenti in una nuova conversazione,
senza fingere riuso remoto o cancellare il contesto precedente.

## Migliorie richieste / decisioni aperte

1. Propagare diagnostica tipizzata quota/rate/auth/context da ACP/harness al
   run e alla notifica operativa, senza testo/body. Distinguere retry breve,
   attesa quota, tool pending e esecuzione attiva. Non dichiarare completed.
2. Prevedere eventi actionable non terminali di blocco con run_id/code/
   retry_after se attestato, recuperabili dallo stesso stream: oggi il PM
   terminal-only deve scoprire il blocco nei log del harness.
3. Dichiarare affidabilita/limiti della stima stall su finestra troncata;
   evitare che pending vecchi sembrino attese attuali.
4. Qualificare cancel→ripresa MiMoCode con goal persistente e correlazione
   remote. Spiegare quando agent_session_id viene svuotato e quale ingresso
   nativo consente di riusare la sessione remote conservata.
5. Budget e redazione log provider429: nessun transcript nel notifier; nei
   log evitare la ripetizione di interi request body per retry senza limite.

Non si propone nuovo bridge, polling o modifica Matrix da Half Pocket.
Il consumer mantiene owner/workspace esclusivi, evita retry/rilanci ciechi
e usa activity_timeout nativo sui nuovi run, preservando stato al timeout.
Decidere a monte quali parti competono a Matrix e quali al harness MiMoCode:
la quota appartiene al provider; dal solo running non si puo dedurre progresso.

## Riscontro aggiuntivo GLM — 7 ottobre 2026

Consumer Half Pocket, stessa installazione Matrix/MiMoCode. Run GLM
run-60f4a68c-f811-4ea4-b9c8-9b27168af335, started12:12:42UTC,
cancelled12:48:37UTC con stop_reason activity_timeout, evento3800.
Nessun nuovo artefatto della review9d20562f nel workspace; WIP/storia protetti.
Diagnosi mirata delle sole righe ERROR del log harness PID187169:
AI_APICallError, HTTP429, provider_error_code1310, retryable=true a
12:27:21,12:32:23,12:37:26,12:42:03,12:46:52UTC. Non si deduce quota
esaurita: serve distinguere rate/concurrency/plan secondo provider.

Log39.351.978byte, una riga massima1.432.448byte. Non sono stati emessi
prompt, requestBodyValues, header, responseBody o valori segreti: solo
campi tecnici di errore/timestamp. Il terminale minimale riporta cancel,
non la ragione provider attestata; il PM deve ancora aprire i log per capire
perche non arriva una consegna. Si confermano i punti1-3/5 della proposta.

Il consumer ha riattribuito la review a DeepSeek con nuovo contesto e scope
read-only, dopo terminale e verifica assenza esecutore GLM. Nessun retry
artificiale o fallback di fatturazione. Questa appendice e ancora una nota
esterna non versionata: nessun altro file/codice/installazione Matrix toccato.


## Decisione del manutentore — 2026-10-08

Accolti errori runtime, avviso di inattività, binding remoto precoce e log limitati. Lettura di log privati del harness e retry/fallback impliciti rifiutati: fuori dal contratto Matrix.

Scheda gestita e archiviata. [Decisione completa, motivi ed evidenze](../../docs/governance/issue_resolution_2026-10-08.md).
Lo stato di pubblicazione e installazione è nel verbale della nuova release;
le evidenze del reporter sopra restano lo snapshot originale.
