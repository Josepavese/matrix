# MiMo via Matrix: run `completed` senza consegna o modifiche

Nota di un agente esterno al repository Matrix, 26 settembre 2026. Questo file
e' l'unica scrittura effettuata qui: nessun codice, configurazione, installazione,
commit o push di Matrix e' stato toccato.

## Ambiente e revisione

- Matrix locale 0.1.46, commit `f0b97ab35e761cad6721a1ab985d3b51005f3e8e`.
- Agente `mimo` ACP/stdio attivo; `POST /v1/runs` asincrono su worktree Half
  Pocket isolato, con nuova sessione effimera e trace `content_mode=refs`.
- Il worktree era pulito al commit Half Pocket `97b96cc1` prima e dopo il
  terzo run. Nessun deploy o dato di produzione e' stato coinvolto.

## Osservazione

| Run ID | Esito Matrix | Eventi significativi | Artefatto |
| --- | --- | --- | --- |
| `run-76c6b92d-2930-4b05-b0e6-ae9849efcb51` | `completed` / `end_turn` | delta e tool call, poi `agent.message.final` | nessuna correzione consegnata nel worktree |
| `run-0d232c8c-7516-4f7c-823e-45c07134c1cd` | `completed` / `end_turn` | `agent.prompt.sent`, usage, session resumed, `run.completed`; nessun final | nessuna modifica |
| `run-059cad89-0ee1-4abd-8065-114bdab5f89c` | `completed` / `end_turn` | `agent.prompt.sent`, usage, session created/cleanup, `run.completed`; nessun delta/final/tool | nessuna modifica |
| `run-499060a7-74ec-42c0-bb1b-9e299229bea5` | `completed` / `end_turn` | `model_verification=provider_confirmed`, prompt, usage, `run.completed`; nessun delta/final/tool | nessuna modifica |
| `run-c38e6c07-995b-4ac3-9a17-4bbc299ff8a2` | `completed` / `end_turn` | macrostep G11: prompt, usage, session created, `run.completed`; nessun delta/final/tool | nessuna modifica |
| `run-98f5d7d9-af98-4ecc-9bc9-f11a045c369e` | `completed` / `end_turn` | ripresa esplicita: prompt, usage, session resumed, `run.completed`; nessun delta/final/tool | nessuna modifica |
| `run-0055c57a-dfb4-4560-8178-b9da950fce03` | `completed` / `end_turn` | preflight sincrono con richiesta di rispondere esattamente `MIMO_OK`: `output=null`; cleanup effimero forte | nessuna risposta |

Nel terzo run il prompt chiedeva esplicitamente di correggere il controller
Safeguard e i test. Il terminale SSE ha notificato `run.completed` quasi subito;
`/explain` ha riportato `prompt_receipt=confirmed_by_result`. Il trace non
contiene `agent.message.final` ne' un evento di errore/preflight, mentre il
worktree non ha cambiamenti. Non e' una consegna utilizzabile.
Il quarto run ha ripetuto l'esito dopo una ripresa esplicita, con
`model_id=xiaomi/mimo-v2.6-pro` confermato dal provider e un prompt che chiedeva
fix, test, pacchetto e prove: anche qui `completed` non segnala lavoro svolto.

## Richiesta ai manutentori

Confrontare il flusso ACP grezzo di questi run con trace e notifiche, per
distinguere: terminazione vuota del provider MiMo, classificazione prematura
di Matrix o problema di ripresa/cleanup della sessione. Un `completed` senza
messaggio finale ne' artefatto deve essere distinguibile da una task compiuta
mediante un segnale strutturato, senza euristiche sul testo. Riprodurre con
un prompt che richieda una modifica verificabile a un file temporaneo.

Decisioni gia' prese: non cambiare Matrix dal repository consumer, non
introdurre fallback silenziosi di modello e non accettare `run.completed`
come prova di completamento applicativo. La responsabilita' del difetto
resta da attribuire dopo la traccia ACP grezza.

## Aggiornamento esterno — 29 settembre 2026, 12:45 CEST

Nuova osservazione dell'agente consumer Half Pocket. Solo questo file di
segnalazione e' stato aggiornato; nessun codice, installazione o commit Matrix.
Binario verificato: 0.1.46, `f0b97ab35e761cad6721a1ab985d3b51005f3e8e`.

- Run: `run-de708b62-d21a-48b6-a850-47747c8f88f9`; sessione provider
  `ses_ffe5f1462b48fffeBdi9LAW4hn`; modello confermato `xiaomi/mimo-v2.6-pro`.
- Avvio: `2026-09-29T10:21:28.530596298Z`; fine
  `2026-09-29T10:38:47.965295407Z`. SSE nativa recapita `run.completed`.
- Trace: `completed`, outcome `completed/end_turn`, sette eventi; prompt,
  selezione modello e usage vuota, ma nessuna tool call, risposta finale o
  diagnostica di errore. Preparatore consumer byte-identico prima/dopo
  (`bb91af516b9c6fc5a87392c703b4030b85de0973783a85fe60b43b5ff0ab87f1`).
- Evidenza provider indipendente: query SQLite **readonly**, limitata agli
  ultimi quattro record di `message` per la sessione, in
  `~/.local/share/mimocode/mimocode.db`. L'assistant
  `msg_g001a0ecaf06dc001K9xB41OmM`, creato alle 10:21:28 UTC, contiene
  `error.name=APIError`, `error.data.message=SSE read timed out`, nessun
  `finish` e contatori input/output/reasoning a zero. Non sono stati estratti
  transcript, credenziali o risposte HTTP complete.

Questo rende concreta l'assenza di propagazione dell'errore nel percorso
provider → ACP → Matrix; **non attribuisce ancora a Matrix il punto in cui
l'errore viene perso**. Non prova esaurimento quota, HTTP 429 o errore di auth.
Occorre correlare il risultato ACP di `session/prompt` con il record provider
e propagare la diagnostica strutturata, senza dedurre errori dal testo e senza
classificare automaticamente ogni risposta vuota come errore.

Decisione consumer: non accettare il risultato per il deploy; conservare
sessione, WIP e prove; riprovare il macrostep in un nuovo contesto persistente
compatto via azione nativa Matrix, senza cambiare modello o Matrix. Il record
precedente della sessione dichiara input 740762: suggerisce di governare la
dimensione del contesto, ma non dimostra la causa del timeout. Restano aperti
attribuzione al confine ACP e test di regressione sul fallimento strutturato.

## Aggiornamento esterno — 30 settembre 2026, 04:55 CEST

Nota dell'agente consumer Half Pocket. Solo questo file di issue e' stato
aggiornato; nessun codice, configurazione, installazione, commit o push Matrix
e' stato toccato. Binario attivo `matrix 0.1.46`, commit
`f0b97ab35e761cad6721a1ab985d3b51005f3e8e`; il checkout Matrix letto
e' a `2864953` e non e' stato usato come prova della versione in esecuzione.

Due tentativi asincroni distinti del macrostep Safeguard/S3, senza mutazioni
produttive e senza artefatti consegnati:

- `run-d6c4dd02-95d5-435e-87fb-cc090735c64a`, workspace isolato su
  `/media/jose/Data/halfpocket-safeguard-mimo-20260930`: fallimento
  `agent_preflight_failed` in `session/new`, errore ACP `RPC -32603 Internal
  error (map[])`; modello effettivo non verificato, nessuna remote session.
- `run-e1ab0639-b623-4728-9108-3e7173b573ae`, retry diagnostico su
  workspace root registrato `/home/jose/halfpocket`: sessione remota
  `ses_ffe5f18186e13ffe3BPW5w3LYu`, modello effettivo
  `xiaomi/mimo-v2.6-pro` `provider_confirmed`, poi
  `agent_preflight_failed` in `session/prompt`, `provider_transport_eof`,
  errore ACP `JSON Parse error: Unexpected EOF`. La SSE nativa ha recapitato
  `run.failed`. Nessun commit/diff o prova AWS e' derivato da questi run.

La prima prova puo' essere influenzata dal percorso del worktree rispetto al
`--cwd` dell'agent registration; la seconda fallisce anche nel workspace root,
quindi non basta attribuire tutto al path. Non e' prova di quota esaurita,
HTTP 429, credenziali errate o difetto nel codice Matrix. Ai manutentori:
correlare i due run con i log ACP/provider grezzi, distinguere parse EOF da
risposta vuota e verificare che il fallimento di `session/new` e quello di
`session/prompt` conservino diagnostica strutturata sicura. Decisione consumer:
nessun ulteriore retry identico durante il gate MVP; lavoro Safeguard non
accettato, worktree e prove preservati, nessun fallback silenzioso di modello.
