# Sessioni esterne, worktree e contratto affidabile per supervisori agentici

**Decisione Matrix (2026-09-24): accolta e chiusa nel perimetro Matrix.**
L'importazione strict-existing è disponibile via
`POST /v1/session-actions`, la registrazione dei repository/worktree via
`/v1/workspace-grants`, le notifiche minimali durevoli sul socket Unix locale,
l'idempotenza delle run e la riconciliazione dopo restart, l'indice agenti
HTTPS configurabile e la spiegazione breve delle run. La sessione DSH esterna
"Zero to Hero" è stata importata nel daemon Matrix con la stessa identità
remota, interrogata con successo, poi interrogata di nuovo dopo il riavvio;
la copia isolata ha preservato l'originale. Una run DSH sul worktree Git reale
è riuscita dopo il grant e una nuova run è stata respinta dopo la revoca. Tre
run concorrenti hanno generato tre notifiche terminali distinte senza contenuti;
il socket ha svegliato un consumer SSE reale. Il kill/restart del daemon ha
marcato una run attiva `outcome_unknown` senza replay.

La selezione del modello usa `model_id` prima del prompt. Una
`fallback_model_id` diversa è l'unica autorizzazione al fallback: viene tentata
solo dopo un rifiuto del modello richiesto, mai per errori di autenticazione o
trasporto. La trace distingue modello richiesto, configurato, confermato dal
provider quando possibile, stato `unverified` quando non attestabile e motivo
del fallback. I test coprono scelta primaria accettata, modello assente,
setting non supportato e fallback autorizzato. Inoltre, tre run reali del
daemon con DSH hanno mostrato conferma del modello, rifiuto prima del prompt
per modello inesistente e fallback esplicito confermato nella trace. Il collaudo del consumer sul
banco Nido resta un'attività della repository Half Pocket: il contratto Matrix
è stato provato con la CLI e un mirror HTTPS isolato.

Nota di un agente esterno (Half Pocket), 24 settembre 2026. Questo file e' un
canale di segnalazione per il team Matrix: **nessun codice, test, configurazione,
installazione o altro file del repository Matrix e' stato modificato; non e'
stato eseguito alcun commit o push**. La issue e' intenzionalmente non tracciata.

## Decisione richiesta e priorita'

Chiediamo di prendere in carico due problemi operativi osservati (P0), poi di
valutare un gruppo di miglioramenti al contratto Matrix-supervisore (P1/P2).
Sono sezioni indipendenti di una sola segnalazione, non una richiesta di
introdurre tutte le funzioni in un unico cambiamento. Il team puo' suddividerle
in issue proprie mantenendo i criteri di accettazione e i confini qui indicati.

| Priorita' | Tema | Natura |
| --- | --- | --- |
| P0 | Aggancio reale di una sessione avviata fuori Matrix | Guasto osservato e lacune verificabili nel percorso corrente |
| P0 | Worktree Git autorizzati con un flusso semplice e sicuro | Rifiuto osservato; proprietario esatto del controllo da identificare |
| P1 | Discovery/importazione vincolate al workspace effettivo | Rischio fondato sul codice |
| P1 | Notifica terminale locale, minimale e durevole | Estensione del contratto eventi esistente |
| P1 | Idempotenza dell'avvio run e riconciliazione dopo riavvio | Garanzie operative da verificare/aggiungere |
| P1 | Modello e agente effettivi attestati, senza fallback implicito | Garanzia per chi orchestra |
| P1 | Mirror verificato del registro/artefatti agenti | Gia' descritto in issue separata |
| P2 | Diagnostica per run orientata all'azione | Miglioramento di usabilita' delle prove esistenti |

## Perche' questi interventi appartengono a Matrix

La tesi dichiarata in `README.md`, `PRODUCT.md`,
`docs/matrix_category_thesis.md` e
`docs/matrix_product_roadmap_2026_2027.md` definisce Matrix come *Agent
Communication Matrix* local-first: gli agenti restano esterni; Matrix rende
uniformi comunicazione, sessioni, handoff, controllo, discovery e continuita'
tra canali. La sessione, non il singolo prompt, e' l'oggetto persistente.

Per questa promessa, poter riprendere una conversazione esistente e poterla
usare nel suo workspace reale non sono comodita' accessorie. Se l'utente deve
ricominciare la conversazione, copiare un transcript o tentare manualmente
diverse directory, Matrix perde proprio la differenza rispetto a un semplice
router di messaggi. D'altra parte Matrix non deve diventare un framework per
creare agenti, un control plane cloud aziendale o un archivio dei dati di ogni
applicazione. Le correzioni proposte restano nel suo perimetro di comunicazione
e lifecycle.

## Ambiente, revisione e livello delle prove

- Ispezione statica del checkout locale Matrix `4ec504c`, documentazione della
  release v0.1.44, Linux, 24 settembre 2026.
- Consumer: un supervisore Codex che avvia DSH (DeepSeek Harness) via Matrix,
  osserva le run in modo asincrono e usa worktree Git isolati.
- Comportamenti riferiti dall'operatore: impossibilita' di riprendere la
  conversazione DSH **"Zero to Hero"** nata fuori Matrix; piu' run interrotte
  per rifiuto di un worktree esterno come directory non autorizzata.
- Non sono disponibili in questa segnalazione i `run_id`/trace dei tentativi
  falliti; non attribuiamo quindi una causa unica confermata a ciascun episodio.
  I punti di codice sotto sono fatti verificabili, non una riproduzione E2E del
  guasto specifico.
- Non sono stati lanciati nuovi agenti o modificati dati per preparare questa
  issue. La verifica reale richiesta al team e' specificata nelle sezioni
  "Criteri di accettazione".

## P0 — Importare e riprendere una sessione nata fuori Matrix

### Comportamento osservato e impatto

Abbiamo cercato di proseguire via Matrix la sessione DSH "Zero to Hero",
precedentemente avviata nell'interfaccia DSH esterna. L'operazione non ha
prodotto una ripresa verificata della conversazione. Per un prodotto che
promette continuita' tra canali e agenti, il risultato deve essere uno dei due:
**stessa sessione e contesto dimostrati**, oppure **errore esplicito che spiega
perche' il provider non lo consente**. Una nuova sessione senza contesto non e'
un successo dell'importazione.

### Evidenze nel sorgente da verificare e correggere

1. `pkg/zedacp/types.go:355-361` rappresenta `SessionInfo.Cwd` e
   `AdditionalDirectories`, ma `internal/providers/agents/acp_adapter.go:558-567`
   non trasferisce queste informazioni nel `RemoteSessionInfo` neutrale.
   `internal/logic/session/manager_commands.go:429-445` crea poi la mirror
   della sessione importata e vi applica il workspace preferito del canale,
   non necessariamente quello dichiarato dalla sessione esterna. **Inferenza:**
   un resume potrebbe essere tentato con un `cwd` diverso dall'originale.
2. `internal/logic/session/manager_commands.go:342-355` tenta lo switch remoto
   soltanto dopo `ListAgentSessions`; ignora l'errore della lista (`_, _`) e,
   senza un match, rinuncia. Un provider che sa riprendere un ID noto ma non
   espone `session/list`, o che fallisce temporaneamente nel listing, non ha
   un percorso chiaro di aggancio e non fornisce una diagnosi utile.
3. `internal/providers/agents/router_clients.go:187-191` puo' usare un client
   riutilizzabile **qualsiasi** dello stesso `agentID` per il controllo delle
   sessioni, mentre esiste anche una scelta qualificata per workspace. Per
   provider il cui elenco dipende dalla directory/processo, la discovery
   puo' non osservare la sessione cercata. Questo e' un rischio strutturale,
   non la causa dimostrata di "Zero to Hero".
4. `internal/providers/agents/acp_adapter.go:374-410` registra gli errori di
   `session/resume` e `session/load`, ma li trasforma in un fallback; il prompt
   su sessione non trovata puo' ricrearne una nuova
   (`internal/providers/agents/acp_adapter.go:162-165`). Il recupero automatico
   puo' avere senso per una sessione ordinaria gia' governata da Matrix, ma
   e' semanticamente sbagliato per un'operazione esplicita "attacca questa
   sessione esterna": l'identita' e la storia sono il risultato richiesto.

### Contratto proposto

- Aggiungere un'operazione esplicita e neutrale, per esempio `session attach`
  via CLI/API, con `agent_id`, `remote_session_id`, workspace di origine e
  modalita' **strict-existing**. Il naming preciso e' a discrezione del team.
- Consentire la verifica di un ID noto anche se il provider supporta
  `resume`/`load` ma non `list`; la lista serve per scoprire, non deve essere
  una precondizione universale per agganciare un ID gia' noto.
- Conservare il `cwd`/le directory annunciati dal provider, validarli come
  input non fidato, risolvere path reali e confrontarli con il workspace
  autorizzato. In caso di mismatch, fermarsi con un errore tipizzato e
  rimedio leggibile; non rimappare silenziosamente il contesto.
- Prima del primo prompt successivo, verificare che `resume`/`load` abbia
  agganciato davvero l'ID richiesto. In modalita' strict-existing, vietare
  `session/new`, replay del prompt come surrogato e ricreazione automatica.
- Restituire una ricevuta con `agent_id`, protocollo, ID remoto, workspace
  effettivo, metodo usato, stato della verifica e limiti della garanzia.
  Distinguere `not_found`, `list_unsupported`, `resume_unsupported`,
  `workspace_mismatch`, `provider_auth_required` e `provider_failure`.
- Se DSH non espone tecnicamente il resume della sua sessione esterna tramite
  ACP o altra capacita' nativa, Matrix deve dichiararlo come `unsupported` e
  documentare il limite. Non deve simulare la continuita' copiando un transcript
  dentro una nuova sessione.

### Criteri di accettazione

1. Creare in DSH, **fuori Matrix**, una sessione con un fatto sintetico presente
   solo nel suo contesto; chiudere l'interfaccia DSH; agganciare l'ID tramite
   Matrix e ottenere una risposta che usi correttamente quel fatto senza
   reinviare la conversazione precedente. Registrare ID remoto prima/dopo,
   workspace e trace Matrix.
2. Ripetere dopo riavvio di Matrix. Verificare che venga ripresa la stessa
   sessione, non creata una nuova.
3. Provider con `resume` ma senza `list`: aggancio diretto dell'ID noto, o
   errore `unsupported` motivato se il protocollo effettivo non lo permette.
4. ID inesistente, sessione revocata, `cwd` diverso e provider non autenticato:
   nessun prompt inviato a una sessione nuova; codici e rimedi distinti.
5. L'errore reale di `session/list` deve comparire nell'esito, non degradare in
   un generico "sessione non trovata".

## P0 — Registrare e usare worktree Git senza disabilitare la sicurezza

### Comportamento osservato e impatto

Piu' esecuzioni lanciate via Matrix su un worktree esterno sono state fermate
per directory non autorizzata. Il controllo puo' trovarsi in Matrix, nel
provider DSH o in entrambi: serve la trace per attribuirlo. Per lo sviluppo
multi-agente il worktree e' una risorsa normale, non un caso limite. Se
l'operatore deve intervenire a mano ad ogni worktree, la promessa di routing
tra agenti e workspace diventa fragile e costosa.

### Evidenze nel sorgente e proposta

`POST /v1/runs` accetta `workspace_path` e `additional_directories`
(`internal/providers/runapi/types.go:45-62`). L'adapter ACP, pero', restituisce
una lista vuota se il provider non annuncia la capacita'
`AdditionalDirectories`, anche quando il chiamante ha chiesto esplicitamente
quelle directory (`internal/providers/agents/acp_adapter.go:506-509`). Il
chiamante puo' dunque credere di averle rese accessibili senza che siano state
inviate. Questa condizione dovrebbe diventare `unsupported` tipizzato o
richiedere una scelta esplicita del chiamante, non essere ignorata.

Proponiamo una **registrazione del workspace** con un unico gesto operativo:
approvare il repository canonico e, opzionalmente, i suoi worktree Git
verificati. Matrix controlla `realpath`, proprietario, `git common-dir` e
associazione al repository registrato; espone elenco, revoca e durata del
grant. Non basta un prefisso di stringa o il nome della cartella: symlink,
path relativi, directory esterne arbitrarie e un repository differente devono
restare fuori dal grant. Si puo' prevedere un'opzione amministrativa
`include_git_worktrees`, non una fiducia globale per `/tmp` o per tutta la
home dell'utente.

La preflight della run deve distinguere:

- path non approvato dalla policy Matrix;
- `additionalDirectories` non supportato dal protocollo/provider;
- path rifiutato dal meccanismo di trust interno del provider;
- permessi OS mancanti o path non esistente.

Quando il rifiuto e' del provider, Matrix deve spiegare come completare il
*suo* trust flow oppure proiettare la richiesta nelle superfici di elicitation
gia' esistenti, se il provider lo supporta. Matrix non deve fingere di poter
approvare per conto del provider, impostare globalmente `agent.trust_mode=true`
o disattivare sandbox e conferme per far passare il test.

### Criteri di accettazione

1. Repository registrato e worktree Git autentico: una run su quel worktree
   parte tramite Matrix dopo il solo grant previsto e conserva workspace e
   sessione corretti.
2. Nuovo worktree collegato allo stesso common-dir: il comportamento segue
   esattamente la policy scelta; revoca efficace sulle nuove run.
3. Symlink, path inesistente, repository estraneo e directory arbitraria
   fuori radice: rifiuto esplicito, nessuna escalation.
4. Provider che non annuncia `additionalDirectories`: errore tipizzato prima
   del prompt quando la directory e' necessaria.
5. Rifiuto del provider: trace con fase, path canonico non sensibile, origine
   del rifiuto e rimedio, senza riclassificarlo come risposta dell'agente.

## P1 — Contratto di supervisione a eventi, locale e a basso contenuto

Matrix possiede gia' run asincrone, eventi ordinati, SSE e webhook con outbox
persistente (`docs/matrix_agent_communication_run_trace.md`). Non chiediamo
un secondo bus. Un supervisore che coordina agenti deve poter avviare una run,
sospendersi e ricevere **solo l'evento terminale** con `run_id`, esito, codice
di fallimento e un'eventuale domanda breve. Transcript, delta, tool output e
ragionamento vengono letti separatamente solo se servono. Questo riduce
traffico, esposizione di contenuti privati e costo dei sistemi agentici che
consumano gli eventi.

I webhook pubblici oggi rifiutano correttamente `localhost`, IP privati e
link-local (`internal/logic/runtrace/sinks.go`), proteggendo da SSRF. Non
proponiamo di togliere il controllo. Per processi sulla stessa macchina e'
preferibile un sink locale autenticato via socket Unix, oppure una superficie
SSE terminal-only con cursor/replay espliciti. La proiezione dovrebbe essere
configurabile per tipo di evento e contenuto, non semplicemente spedire tutti
gli eventi a un consumer che deve scartarli. Gli eventi mantengono ID stabile;
il consumer deduplica le consegne at-least-once.

**Accettazione:** tre run concorrenti producono tre notifiche terminali
distinte; nessun contenuto del prompt/transcript compare nelle notifiche;
consumer temporaneamente offline recupera gli esiti senza polling periodico;
il blocco SSRF dei webhook HTTP resta verde.

## P1 — Idempotenza dell'avvio e run interrotte dal daemon

`POST /v1/runs` avvia lavoro potenzialmente costoso o mutante. Nella richiesta
corrente (`internal/providers/runapi/types.go:45-64`) non compare una chiave
di idempotenza del chiamante. Se la risposta HTTP si perde dopo l'accettazione,
un retry puo' avviare un secondo agente: il problema non e' solo quota, ma
anche doppie modifiche o doppie azioni esterne. Proponiamo una chiave opzionale
scopata all'ingresso/identita' del chiamante: stessa chiave e stesso digest
della richiesta restituiscono lo stesso `run_id`; stessa chiave con payload
diverso e' un conflitto esplicito. Definire retention della chiave e ricevuta
del dedup.

La run e' persistita con stato `pending|running|terminal`, ma la documentazione
consultata descrive il recupero dell'outbox dopo restart, non una riconciliazione
chiara delle run rimaste `running` quando il daemon muore. Questa e' una
**domanda di audit**, non una dichiarazione di bug riprodotto: alla ripartenza
occorre distinguere esecuzione ancora agganciabile, esito sconosciuto e lavoro
certamente interrotto. Mai rieseguire automaticamente un prompt mutante solo
per chiudere una riga di stato. Esporre un evento/ricevuta di recupero leggibile
dal supervisore.

**Accettazione:** retry HTTP dopo risposta persa non duplica la run; chiave
riusata con input diverso e' respinta; kill/restart del daemon nel mezzo di una
run non lascia uno stato eternamente `running`, non riavvia di nascosto il
provider e conserva una diagnosi onesta dell'esito remoto.

## P1 — Attestare agente, modello e capacita' effettivi

L'API permette di scegliere `agent_id` e, per Codex, il reasoning effort, ma
non espone nella richiesta run un selettore neutrale di modello
(`internal/providers/runapi/types.go:45-68`). Per un supervisore la differenza
fra modello richiesto e modello realmente usato determina qualita', costo e
conformita' al mandato; un fallback silenzioso rende impossibile valutare la
run. Chiediamo una capability/setting neutrale **solo dove il provider lo
supporta**, con attestazione nella trace di agente, modello, versione/fonte
quando nota, workspace e motivo di un eventuale fallback esplicitamente
autorizzato. Se non verificabile, dichiarare `unverified`; non inventare un
modello effettivo. Se il modello richiesto e' indisponibile, errore tipizzato
prima del lavoro salvo policy di fallback fornita dal chiamante.

**Accettazione:** modello richiesto e disponibile, assente, non supportato dal
provider e fallback autorizzato producono quattro esiti distinguibili;
l'attestazione proviene dalla configurazione/risposta effettiva del provider,
non dalla sola richiesta del chiamante.

## P1 — Installare agenti anche con GitHub indisponibile

Questa richiesta ha gia' evidenza e scelte architetturali dettagliate in
`issues/2026-09-24-agent-registry-artifact-mirror-configurable.md`. Non
duplicare implementazione o triage: collegarla al lavoro di continuita'
operativa. Matrix non e' davvero indipendente dalla superficie da cui un
agente viene distribuito se `matrix install` deve seguire l'URL GitHub
dell'indice ACP senza override governato. Il mirror deve conservare origine,
versione e verifica SHA; `set-binary` o il bypass dei digest non sono
sostituti equivalenti. In un prodotto local-first, la raggiungibilita' di un
singolo forge non dovrebbe decidere se si possa ripristinare l'agente.

## P2 — Una diagnostica breve che trasformi trace in azione

Le trace Matrix hanno gia' fatti ricchi: routing, preflight, fasi ACP,
sessione, cleanup e codici di errore. Chiediamo una proiezione per operatore,
per esempio `matrix run explain <run_id>` e analogo endpoint, che dica in
italiano/inglese secondo la locale: cosa e' stato richiesto, quale agente e
workspace sono stati selezionati, se il provider ha ricevuto il prompt, dove
si e' fermato, cosa resta incerto e quale azione sicura e' disponibile ora.
Deve usare la trace esistente come fonte, senza un secondo stato autorevole e
senza mostrare ragionamento privato, token o transcript per default. Questo
riduce errori d'operatore del tipo "non ha risposto, riprovo" quando una run e'
ancora in corso o ha un esito remoto incerto.

**Accettazione:** casi reali di auth mancante, worktree non approvato,
`session/list` fallita, sessione remota assente, cancel parziale e successo
producono cause e rimedi diversi, con link al dettaglio della trace.

## Test trasversali e prove richieste per la chiusura

- Test di contratto ACP v1/v2 e, dove pertinente, A2A: provider con
  combinazioni diverse di `list`, `resume`, `load` e
  `additionalDirectories`; nessun fallback semantico nascosto.
- Test avversariali su workspace: path canonici, symlink, worktree legittimi,
  repository estranei, client ACP riutilizzati su piu' workspace e sessioni
  con stesso titolo ma ID diversi.
- Test di concorrenza: tre run con sessioni distinte; arrestarne una non deve
  fermare le altre. Mantenere la prova esistente di cleanup forte/debole,
  senza etichettare `clean=true` come prova di continuita' della sessione.
- Prova reale con DSH della sessione nata fuori Matrix; prova reale del
  worktree con lo stesso provider che ha segnalato il rifiuto. Registrare
  `run_id`, trace minimizzata, versione Matrix/provider e risultato, senza
  pubblicare transcript o segreti.
- Test del pacchetto/installazione pubblica su un host isolato, quando una
  modifica tocca registry, daemon o trasporto. Versione e digest dell'artefatto
  realmente installato devono coincidere con la release dichiarata.

## Decisioni gia' prese e motivazione

- **Non trasformare Matrix in Hub o in un archivio business.** Autenticazione
  aziendale, autorizzazioni applicative, email, task e documenti restano ai
  prodotti proprietari. Matrix gestisce comunicazione, sessioni, lifecycle e
  prove. E' coerente con la sua categoria dichiarata.
- **Non simulare il resume con il transcript.** Una nuova sessione alimentata
  con vecchi messaggi non conserva necessariamente tool state, memoria e
  identita' del provider. Se e' offerta come alternativa, va chiamata
  esplicitamente "nuova sessione con contesto importato", non "ripresa".
- **Non risolvere il worktree disabilitando trust/sandbox.** Il problema e'
  rendere leggibile e autorizzabile il perimetro esatto, non dare accesso
  indiscriminato al filesystem.
- **Non aprire genericamente i webhook a `localhost`.** Il rifiuto protegge
  dall'SSRF; un canale locale dedicato conserva sicurezza e usabilita'.
- **Non aggirare digest e provenienza del registro agenti.** La capacita' di
  usare un mirror deve rafforzare l'indipendenza della distribuzione senza
  indebolire l'integrita'.
- **Non ripetere automaticamente una run dall'esito incerto.** Il prompt puo'
  aver prodotto effetti esterni. La riconciliazione deve rendere l'incertezza
  visibile, non crearne una seconda esecuzione.

## Decisioni aperte al team Matrix

1. Quale API/CLI nominare per l'attach strict-existing, e quali provider
   possono dimostrarlo oggi senza nuova estensione?
2. Quali campi del provider, incluso `cwd`, possono essere fidati solo dopo
   validazione locale e quale e' il contratto di mismatch?
3. Il rifiuto del worktree osservato nasce da Matrix, DSH o dalla loro
   composizione? Quale trace minima lo dimostra? Il grant per Git worktree e'
   utile in Matrix, nel provider o in entrambi?
4. Per i sink locali, socket Unix, SSE con resume o entrambe le superfici?
   Quale forma di autenticazione e retention degli eventi terminali?
5. Quale finestra di deduplica per le chiavi di idempotenza e quale stato
   rappresenta meglio una run orfana con esito remoto non dimostrabile?
6. Come attestare il modello effettivo quando ACP/A2A o il provider non lo
   comunicano, evitando di trasformare la sola configurazione richiesta in
   una falsa prova?

La chiusura delle sezioni P0 richiede le prove reali indicate; test unitari,
trace di una nuova sessione o successo di un provider diverso non bastano.
