# Reversioni — wizard di onboarding

Ogni riga è un esperimento **eseguito**: la reversione compila e il test indicato
cade. Il metodo è quello del repo: si copia il file, si applica la reversione, si
compila, si esegue il test, si ripristina **dalla copia** (mai con `git`).

Le reversioni R1–R4 vivono tutte in `auth_flow_ownership.go`, perché è lì che sta
il vocabolario dell'ownership: una riga sola per esperimento.

| nome | cosa reverte | test che cade | l'altro lato (resta verde) |
|---|---|---|---|
| R1 | l'ownership ignorata nello starter: `startDeclaredMethod` non chiede più all'handler | `TestCodexDeviceStarterRunsForItsOwnHandler` | `TestVendorStarterStaysUnreachableForAReusedMethodID` |
| R2 | ogni handler dichiara di possedere lo starter (`!ok` → `owned=true`) | `TestFallbackHandlerOwnsNoFlow` | — |
| R3 | ogni handler dichiara di possedere lo stage del provider | `TestProviderAuthStageStaysGenericWithoutAnOwner` | — |
| R4 | ogni handler dichiara di preparare la selezione | `TestSelectionPreparationBelongsToTheCodexHandler` | — |

Sonde sul gate (albero sintetico in `/tmp`, non il repo): un file di produzione
con `if name == "brandnewagent"` **e** con `agentID == nuovaCostante` fa fallire
`identity_comparison_shape` con due finding distinti — la prima su un letterale
che nessuno ha enumerato, la seconda su una costante nuova. Il budget resta a
`max = 0` con otto eccezioni riviste, tre in meno di prima.

## I sette dispatch per nome e dove è finita ciascuna proprietà

Il wizard non chiede più **chi** è l'agente: chiede all'handler che il registry ha
risolto per l'agente scelto che cosa possiede. La proprietà è la stessa in tutti e
sette i casi — `auth_flow_ownership.go` — e cambia solo quale capacità la esprime.

| sito | prima | adesso |
|---|---|---|
| `wizard_steps.go:109` | `selected.ID == agentCodex` → preparazione codex | `prepareSelection` sull'handler risolto |
| `wizard_steps.go:190` | `AgentName == agentOpencode` + provider → prompt OpenRouter | `ProviderAuthPrompt` dell'handler che serve quel provider |
| `wizard_steps.go:194` | stessa condizione → prompt API key | idem, dentro lo stesso handler |
| `wizard_agent_selection.go:40` | `selected.ID == agentCodex` dopo l'attivazione | `prepareSelection` sull'handler risolto |
| `wizard_auth_flow.go:9` | `AgentName == agentOpencode && method.ID == "quick_login"` | `StartDeclaredMethod` dell'handler OpenRouter |
| `wizard_auth_flow.go:12` | `AgentName == agentCodex && method.ID == "chatgpt"` | `StartDeclaredMethod` dell'handler codex |
| `wizard_auth_flow.go:61` | `AgentName != agentOpencode` come guardia di instradamento | `HandleProviderAuthInput` dell'handler che possiede lo stage |

Verdetto: **7 sanati, 0 legittimi, 0 impossibili.**

Le due condizioni che restano sui nomi sono **dati**, non dispatch, e restano
dichiarate nel manifest:

- `auth_handler.go` registra `"codex"` e `"opencode"` come chiavi della mappa che
  lega un flow agli agenti che lo usano. Toglierle vorrebbe dire togliere l'unico
  punto in cui il flow viene scelto: è il mestiere del registry, non una branch
  sull'identità dentro il flusso.
- `wizard_codex.go` tiene `agentCodex` perché il flow codex nomina il **proprio**
  binario e la **propria** voce di configurazione. È il soggetto del file, non una
  decisione presa su un altro agente.

Un agente nuovo che non ha un handler dedicato riceve il fallback, che non
implementa nessuna delle tre capacità: resta sul percorso generico senza che nulla
nel wizard lo nomini. È la proprietà che `TestFallbackHandlerOwnsNoFlow` pina.

## Limiti dichiarati

- Nessuna di queste prove dice che il wizard funzioni su Windows o Darwin: la
  compilazione incrociata è verificata, l'esecuzione no. Questi test non usano
  `sh`, symlink o filesystem, quindi non si skippano: girano identici ovunque.
- Il dispatch verso i flow vendore resta ancorato agli id di metodo dichiarati
  (`chatgpt`, `quick_login`), che il manifest continua a rivedere come vocabolario
  chiuso. Se un giorno quegli id arrivassero dal provider invece che dal codice,
  la revisione del manifest andrà rifatta.
