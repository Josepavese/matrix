# Agnosticism Audit — Baseline (PRE-fix)

Revisione: `2864953dad43cb9e12892b0cc3bd95377f7e0f1f` (HEAD)
Data audit: 2026-10-01, 13:5x UTC
Stato albero di lavoro: **PULITO** (`git status --porcelain` senza file modificati, `git diff --stat` vuoto)
Natura dell'audit: **PRE-fix**. Nessuno dei quattro writer aveva salvato modifiche quando l'audit è stato eseguito.
Rilevanza: questo documento è la fotografia del codice *prima* delle correzioni delle dieci issue.
Serve a dimostrare, sul diff finale, che nessun fix ha introdotto un ramo per nome di agente.

Autore: verifier (revisore indipendente). Questo file è l'unica scrittura prodotta dall'audit:
nessun file di produzione e nessun test è stato modificato per produrlo.

---

## 1. Mandato e criterio di verdetto

Vincolo dell'utente, esplicito: chiudere le issue **senza patch ad hoc per agenti specifici**,
mantenendo Matrix una libreria agnostica.

Criterio applicato:

- **LEGITTIMA** — il nome dell'agente compare come *dato*: valore di default, voce di registro,
  configurazione dell'operatore, stringa di aiuto CLI, nome di protocollo, commento.
  Il comportamento del codice non cambia in funzione del nome.
- **SOSPETTA** — il nome dell'agente compare come *controllo di flusso*: `if`/`switch`/allowlist/
  tabella di casi/`Matches` su identità di agente o di provider **dentro la logica**.
  Il comportamento cambia in funzione del nome, e un agente non previsto riceve una risposta diversa.

## 2. Perimetro e metodo

Perimetro di produzione: `cmd/`, `internal/`, `pkg/`. Esclusi `*_test.go`, `**/testdata/**`, `tests/`,
`issues/`, `docs/`, `README.md` e gli altri file di documentazione. I test e la documentazione sono
fuori perimetro per mandato: le loro occorrenze sono identificatori di test, non comportamento.

Comandi eseguiti (tutti in sola lettura):

```
rg -n -i -g '*.go' -g '!*_test.go' -g '!**/testdata/**' -g '!tests/**' \
   '\b(opencode|codex|mimo|minimax|claude|gemini|deepseek|halfpocket|aider|qwen|kimi|copilot|cursor)\b' \
   cmd internal pkg
rg -n -i -g '*.go' -g '!*_test.go' '\b(openai|anthropic|openrouter|zed-industries|ollama|lmstudio)\b' cmd internal pkg
rg -n -g '*.go' -g '!*_test.go' '(?i)(agentID|agentName|agent_id|AgentID|Name)\s*==\s*"|EqualFold\([^)]*(agent|Agent)|switch\s+[a-zA-Z_.]*(agent|Agent)'
git blame -L <range> -- <file>        # su ogni riga sospetta, per separare pregresso da nuovo
```

Tecnica anti-falso-positivo: il pattern `zed` è stato inizialmente scartato perché `authorized`,
`initialized`, `normalized` lo contengono; sostituito con pattern a confine di parola. La stessa
verifica a contesto ha escluso le occorrenze di `cursor`, che in questo repository sono variabili
di paginazione (`internal/logic/runtrace/notifications.go`, `internal/providers/runapi/*`,
`internal/providers/bolt/bolt.go`, `pkg/zedacp/types.go:345`), non il prodotto Cursor.

## 3. Prova negativa (il reperto più importante della baseline)

```
rg -n -i -g '*.go' -g '!*_test.go' 'mimo|minimax|halfpocket' cmd internal pkg   →  exit 1, ZERO occorrenze
```

Nel Go di produzione **non esiste alcuna occorrenza di `mimo`, `minimax` o `halfpocket`**.
Conseguenze dirette:

1. Non esiste un caso speciale MiMo pregresso da cui partire: qualunque ramo condizionale per MiMo
   introdotto durante la chiusura delle issue è, per costruzione, **nuovo** e va respinto.
2. Quattro delle dieci issue (09-26, 10-01 ×2, più il riferimento MiMo in 09-28) riguardano MiMo.
   La loro chiusura deve avvenire su cause generiche, non su un ramo per nome.
3. Unico URL di registro hard-coded nel Go di produzione: `internal/logic/agentmgr/registry_client.go:100`
   (`https://cdn.agentclientprotocol.com/registry/v1/latest/registry.json`). Nessun riferimento a
   registri privati dell'operatore (`software.halfpocket.net` compare **solo** dentro i file di issue).

## 4. Occorrenze SOSPETTE

### S1 — `internal/providers/exec/exec_unixlike.go:113-119` — **VIOLAZIONE DI AGNOSTICITÀ**

```go
113  // Also check in NVM environment if name is node/npm or an agent
114  if name == "node" || name == "npm" || name == "codex" || name == "gemini" || name == "claude" || name == "opencode" {
115      spec := middleware.CommandSpec{Runner: "which", Args: []string{name}, EnvIsolation: true}
116      if _, err := p.Exec(spec); err == nil {
117          return true
118      }
119  }
```

Cosa fa: `HasExecutable(name)` prova `exec.LookPath`; il secondo tentativo, quello che cerca
l'eseguibile dentro un ambiente NVM (`EnvIsolation: true`), è concesso **solo** se il nome è in una
allowlist di sei stringhe. Per ogni altro agente il secondo tentativo non esiste e la funzione
risponde `false` anche quando il binario è raggiungibile nell'ambiente NVM.

Blame: `fdc435d4` (2026-04-13) — pregresso, non introdotto dai writer attuali.

Consumatori (il verdetto non è teorico, è sul percorso di lancio/readiness):

- `internal/logic/agentmgr/supervisor.go:95` — gate prima dell'avvio del provider;
- `internal/logic/agentmgr/supervisor.go:112` — gate sul percorso ACP/stdio;
- `internal/logic/agentmgr/runtime_status.go:177` — stato di runtime dell'agente (`runtime_status`);
- `internal/logic/onboarding/wizard_codex.go:95`.

Verdetto: **violazione**. Un allowlist di nomi di agente nella risoluzione dell'eseguibile è
esattamente la scorciatoia che il vincolo dell'utente vieta: `mimo`, `kimi`, `zed` o qualunque agente
futuro ricevono una risposta diversa a parità di ambiente. Non è un dettaglio e non è addolcibile.

Nota tecnica per chi corregge: la scelta di isolare l'ambiente non è una proprietà del *nome*
dell'agente, è una proprietà della *configurazione* dell'agente — `configs/agents.json` porta già
`env_isolation: true` per `claude`, `gemini`, `opencode`. Il segnale giusto esiste già nel dominio.
La firma attuale è tuttavia `HasExecutable(name string) bool`
(`internal/middleware/process.go:50-51`), quindi una correzione agnostica richiede o il fallback
sempre attivo, o un parametro/contesto che porti la configurazione: non basta allungare la lista.

### S2 — `internal/providers/agents/router_observer_content.go:141-151` — SOSPETTA (fallback vendor)

```go
141  func acpMessagePhase(meta map[string]interface{}) string {
142      if phase, ok := meta["messagePhase"].(string); ok { ... }
145      if phase, ok := meta["message_phase"].(string); ok { ... }
148      codex, _ := meta["codex"].(map[string]interface{})
149      phase, _ := codex["phase"].(string)
150      return strings.TrimSpace(phase)
151  }
```

Cosa fa: dopo le due forme generiche, la classificazione della fase di un messaggio ACP legge una
forma **specifica di Codex** (`meta["codex"]["phase"]`). Il risultato alimenta
`messageClassification` (righe 153-162: `commentary`→`progress`, `final_answer`→`final`), cioè
distingue un messaggio di avanzamento da una risposta finale.

Blame: `812b0353` (2026-07-22) — pregresso.

Verdetto: **sospetta**. Il ramo è innestato in una funzione generica e produce comportamento diverso
in funzione del vendor che popola i metadati. Rilevanza diretta sulle issue 09-25 (OpenCode/MiniMax:
`run.completed` con output non concluso) e 09-26 (MiMo: run completato senza consegna): se la
correzione "distingue una risposta finale da un turno interrotto" si appoggia a questa forma codex,
diventa una patch ad hoc. Il fix deve basarsi sul contratto ACP generico.

### S3 — `internal/logic/agentmgr/installer.go:157` — SOSPETTA (tabella di casi nel percorso di installazione)

```go
157  if manifest.Distribution.Npx == nil || !agentidentity.IsCanonicalCodexPackage(manifest.Distribution.Npx.Package) {
158      // ... registrazione npx generica
163  }
164  // ... altrimenti: percorso dedicato agentinstall.InstallCanonicalCodex
```

Cosa fa: il percorso di installazione generico dirama sul fatto che il pacchetto npx sia
`@agentclientprotocol/codex-acp`: solo in quel caso usa l'installazione canonica dedicata, per tutti
gli altri registra il comando npx così com'è.

Blame: `dbf2ce54` (2026-08-04) — pregresso.

Verdetto: **sospetta, da non estendere**. È il precedente più vicino alle issue di area installazione/
lancio (09-30, 10-01 ×2). Il rimedio corretto per quelle issue è generico (vedi §6); questo `if` non
deve diventare il posto in cui si aggiunge un secondo nome.

### S4 — `internal/logic/agentlaunch/policy.go:23-29` — WATCH ITEM (superficie di rischio)

```go
23  var policyAdapters = []policyAdapter{codexPolicyAdapter{}}
27  func (codexPolicyAdapter) Matches(agentID string, _ middleware.ProtocolEndpoint) bool {
28      return strings.EqualFold(strings.TrimSpace(agentID), agentidentity.CanonicalCodexAgentID)
29  }
```

Cosa fa: `ResolveEndpoint` scorre un elenco di adapter e applica il primo che dichiara `Matches`.
Il pattern a registry è strutturalmente accettabile (un agente nuovo aggiunge un adapter invece di
modificare la logica comune), ma oggi l'unico adapter è Codex e il confronto è per nome.

Chiamato in modo generico da: `internal/providers/agents/router.go:276`,
`internal/logic/agentmgr/supervisor.go:105`, `cmd/matrix/agent_doctor.go:50`,
`cmd/matrix/agent_show.go:51`, `internal/providers/runapi/runs.go:93`.

Blame: `dbf2ce54` (2026-08-04), `a5b5aeb6` (2026-06-23) — pregresso.

Verdetto: **watch item ad alta priorità**. È il punto esatto in cui un adapter che riconosce
`opencode` o `mimo` realizzerebbe la patch vietata senza apparire come un `if` sospetto.
Va ispezionato nel diff finale: nessun nuovo adapter e nessuna nuova `Matches` per agente.

### S5 — `internal/logic/agentlaunch/codex.go:34` — SOSPETTA (gate per nome nel percorso run generico)

```go
34  if !strings.EqualFold(strings.TrimSpace(agentID), "codex") {
35      return "", errors.New("model_reasoning_effort is supported only when agent_id resolves to codex")
36  }
```

Cosa fa: la validazione di `model_reasoning_effort` rifiuta qualunque agente che non sia `codex`.
La funzione è invocata dal percorso di richiesta run generico:
`internal/providers/runapi/run_request.go:63` (`agentlaunch.CodexReasoningEffortArgs(agentID, ...)`).

Blame: pregresso (modulo `agentlaunch/codex.go`, capability CLI codex-only).

Verdetto: **sospetta ma motivata**: `-c model_reasoning_effort=` è un flag reale del CLI Codex, non
un'invenzione. Resta un gate per nome agente raggiungibile dal percorso generico. Se la correzione di
area lancio rende generico il passaggio degli argomenti per-run, questo gate va reso coerente con la
configurazione dell'agente, **non ampliato** con altri nomi.

### S6 — `internal/logic/onboarding/` — LEGITTIMA (sottosistema per-agente per progetto)

- `internal/logic/onboarding/wizard.go:308` — `if agentName == "codex" { envKey = "OPENAI_API_KEY" }`
- `internal/logic/onboarding/auth_handler.go:73-74` — mappa `{"codex": &codexAuthHandler{}, "opencode": &openrouterAuthHandler{}}`
- `internal/logic/onboarding/wizard_codex.go`, `wizard_openrouter.go`, `wizard_steps.go:11-12`

Verdetto: **legittima, rischio basso**. L'onboarding è, per progetto, il sottosistema che configura
*un agente specifico*: il dispatch per nome è il suo scopo, non una scorciatoia dentro logica
generica. Non è la sede delle issue in lavorazione. Da riverificare solo se un fix lo tocca.

### S7 — `internal/logic/agentmgr/registry_client.go:277` — SOSPETTA (alias codex nel client generico)

```go
277  if agents[i].ID == agentID && agentID != agentidentity.CodexRegistryID {
281  if alias := agentidentity.CanonicalRegistryAlias(agentID); alias != "" {
```

Cosa fa: il client del registro ACP tratta l'identificatore `codex-acp` come alias di `codex`
(risoluzione e messaggio d'errore), con il supporto di `internal/logic/agentidentity/codex.go`.

Blame: `dbf2ce54` (2026-08-04) — pregresso.

Verdetto: **sospetta, rischio basso**. È una migrazione di identità one-way documentata
(`ZERO-LEGACY`), centralizzata e protetta da un budget di governance (§7). Non è una patch di
comportamento per agente, ma è un caso speciale per vendor dentro un client generico e va letto nel
diff finale, non esteso.

### File `internal/logic/agentidentity/codex.go` (intero)

Modulo dedicato all'identità canonica Codex (`CanonicalCodexAgentID`, `CanonicalCodexPackage`,
`DeprecatedCodexPackage`, `ValidatePublicAgentID`, `ValidateRuntimeDefinition`).
Verdetto: **legittima con riserva** — è un confine di rifiuto centralizzato per un provider ritirato,
sotto budget di governance; non è logica generica che dirama per agente.

## 5. Occorrenze LEGITTIME (nessuna azione)

Valori di default e dati di registro — il nome è un dato, non un controllo di flusso:

- `cmd/matrix/constants.go:9` — `DefaultAgent = "opencode"`
- `internal/logic/session/manager.go:97-98` — `defaultAgentID = "opencode"`, `defaultActionAgentID = "gemini"`
- `internal/providers/a2a/server.go:35`, `internal/providers/matrixapi/server.go:38`,
  `internal/providers/runapi/types.go:100` — `defaultAgent = "opencode"`
- `internal/logic/agentmgr/registry_client.go:100` — `defaultRegistryURL`
- `internal/logic/system_tools/handlers.go:21,27` — testo di aiuto che elenca gli agenti installabili
- `configs/agents.json`, `configs/node_env.json:4`, `configs/locales/en.json`, `configs/locales/it.json`
  — configurazione dell'operatore, dati di installazione, stringhe UX
- `internal/providers/agents/router.go:358-359` — commento che nomina le mode di scrittura di alcuni
  agenti; le righe adiacenti non diramano per nome
- `pkg/zedacp/doc.go:1` — `zedacp` è il nome del protocollo (Zed Agent Client Protocol), non un ramo
- Soli commenti: `cmd/matrix/run.go:101`, `cmd/matrix/agent_capabilities.go:17,43`,
  `internal/logic/agentmgr/registry.go:95`, `internal/providers/agents/elicitation_handler.go:58`,
  `internal/logic/onboarding/auth_handler.go:12,43,61`
- `internal/providers/matrixapi/server.go:119,165,180` — endpoint e callback OAuth OpenRouter:
  nome di provider come rotta HTTP pubblica, non diramazione per agente

Nomi di provider (`openai`, `anthropic`, `openrouter`): presenti come chiavi di configurazione,
esempi di CLI (`cmd/matrix/config_get.go:13`, `config_set.go:10-11`, `config_delete.go:11`,
`config.go:17`) e flusso OAuth. Legittimi.

## 6. Cause radice già generiche (la buona notizia per l'agnosticismo)

Due dei sintomi in lavorazione hanno causa radice **indipendente dall'agente** già visibile in baseline:
questo significa che la correzione corretta è generica e non richiede alcun nome.

**Issue 09-30 — `matrix install` perde gli argomenti del binario.** Due perdite in serie:

1. `internal/logic/agentmgr/registry_client.go:64` — `BinaryDist` **ha** il campo `Args []string`;
   ma `ResolveAnyDistribution` ritorna `&ResolvedDist{Type: "binary"}` alla riga 241 **senza copiare
   `dist.Args`**.
2. `internal/logic/agentmgr/installer.go:150-153` — il ramo binario di `installResolved` ritorna
   `agentcfg.Config{Command: binaryPath, Kind: "acp", Transport: "stdio"}` **senza `Args`**, mentre il
   ramo npx (righe 159-162) li propaga (`Args: resolved.Args`).

Il sintomo riportato (`effective.args: []` dopo `matrix install opencode`, con il registro che
dichiara `args: ["acp"]`) è la conseguenza esatta. Vale per qualunque agente distribuito come binario
con argomenti: **nessun nome di agente serve per correggerlo**.

**Issue 09-25 — run completato con output non concluso / `end_turn`.** In
`internal/providers/runapi/runs.go:209` lo stop reason è **hard-coded**:

```go
209  _, err = s.runStore.Complete(exec.runID, res, "end_turn")
```

È l'unico chiamante di `Store.Complete` (`internal/logic/runtrace/lifecycle.go:81`), che a sua volta
applica `firstNonEmpty(stopReason, "end_turn")` (riga 126). Lo stop reason reale osservato dall'ACP è
registrato in `internal/providers/agents/router_observer.go:33-45` (`stopReason`, `terminal`) e **non
raggiunge mai lo store**. Anche qui: generico, e la correzione corretta deve *distinguere* gli esiti,
non rendere più rumoroso il caso `end_turn`.

**Issue 09-28 / 10-01 — il sintomo `-32603 Internal error (map[])` è riproducibile dal codice di Matrix.**
In `pkg/zedacp/jsonrpc.go:91-100` (`rpcErrorFromWire`) il testo dell'errore in ingresso è costruito così:

```go
95  text := fmt.Sprintf("RPC error %d: %s", err.Code, err.Message)
96  if err.Data != nil {
97      text = fmt.Sprintf("%s (%v)", text, err.Data)
98  }
```

Con `err.Code = -32603`, `err.Message = "Internal error"` e `err.Data` non-nil ma vuoto, l'output è
**esattamente** `RPC error -32603: Internal error (map[])`. Il `(map[])` non è testo del provider: è la
resa `%v` di una mappa vuota fatta da Matrix alla riga 97, appesa al messaggio alla riga 95, con il
risultato che payload strutturato e messaggio diventano indistinguibili. Difetto **generico**, nessun
nome di agente coinvolto.

Precisazione necessaria, perché cambia il bersaglio della correzione: la riga 64 dello stesso file
(`RPCError.Error()`: `return fmt.Sprintf("RPC error %d", e.Code)` quando `Message` è vuoto) è un difetto
**diverso** — perde il `Data` di un errore prodotto localmente e rende `RPC error -32603` senza
messaggio. Non è la riga che produce il sintomo `(map[])` osservato dai consumer. Chi chiude 09-28/10-01
deve toccare il percorso 91-100 (o comunque dimostrare con un test quale dei due percorsi genera la
stringa riportata); correggere solo la riga 64 lascerebbe il sintomo in piedi.

Verifica eseguibile da chiunque, senza agente reale: le due `Sprintf` alle righe 95 e 97 con gli input
sopra indicati hanno quell'output per costruzione.

## 7. Proposta di garanzia meccanica (non applicata — decisione del Lead)

Il repository possiede già il meccanismo giusto: i `pattern_budget` di `governance/manifest.toml`,
applicati da `scripts/governance_check/main.go:225-298`. Ogni budget cammina i `roots`, conta le
occorrenze letterali di ciascun pattern nei file di testo, esclude gli `allowed_files` e **fallisce se
il totale supera `max`**. Esempio in esercizio: `[pattern_budget.deprecated_codex_provider]`
(righe 143-153) con `max = 0` e i soli `internal/logic/agentidentity/codex*.go` come eccezione.

Un budget sulla forma "nome di agente come letterale nel Go di produzione" congela la baseline e
trasforma il vincolo dell'utente in una verifica automatica: qualunque fix che aggiunga `"mimo"`,
`"minimax"` o un nuovo ramo per nome fa fallire il preflight.

Conteggi misurati per la baseline (occorrenze letterali della stringa quotata, `internal/` + `cmd/`):

| letterale | Go di produzione | tutti i file di testo (incl. test) |
|---|---|---|
| `"opencode"` | 9 | 304 |
| `"codex"` | 7 | 225 |
| `"gemini"` | 3 | 28 |
| `"claude"` | 1 | 56 |
| `"mimo"` | 0 | 0 |
| `"minimax"` | 0 | 0 |

Attenzione: il checker conta i file di testo **senza escludere i `*_test.go`** (nessuna esclusione in
`checkPatternBudget`), quindi un budget tarato su `max` includerebbe i test e servirebbe una taratura
o un elenco di `allowed_files` per le directory di test. Non ho modificato
`governance/manifest.toml`: la decisione e l'implementazione non sono di questo ruolo.

## 8. Mappa issue → sito da non toccare per nome

| Issue | Area | Sito sospetto adiacente | Regola |
|---|---|---|---|
| 09-25 OpenCode/MiniMax `end_turn` | esito run | S2 (`meta["codex"]["phase"]`) | il fix non può appoggiarsi a S2 |
| 09-26 MiMo senza consegna | esito run | S2, S4 | nessun ramo per `mimo` |
| 09-27 grant `workspace_id` senza path | workspace | — | `workspacegrant/grants.go:128` è generico |
| 09-28 ACP stallo parent/child | esito run | S4 | nessun ramo per agente |
| 09-29 attestazione su sessione riusata | attestazione | S4 | nessun ramo per agente |
| 09-29 workspace id/path isolamento | workspace/sessione | — | generico |
| 09-30 args binario persi | installazione | S1, S3 | fix generico (§6) |
| 10-01 MiMo cwd/worktree | lancio | S1, S3, S4, S5 (+ `jsonrpc.go:91-100` area T1) | nessun ramo per `mimo`; cwd dal workspace del run |
| 10-01 MiMo session/new −32603 | lancio | S1, S3, S4, S5 (+ `jsonrpc.go:91-100` area T1) | nessun ramo per `mimo` |
| 09-23 attese a orologio nei test | test | — | fuori perimetro produzione |

## 9. Non verificato

- Nessun test eseguito in questo turno; l'audit è statico (grep + blame + lettura).
- Nessuna riproduzione dei sintomi (`end_turn`, `args: []`, `workspace path must be absolute`,
  `-32603 Internal error (map[])`): è il perimetro di task-7 e richiede dipendenze chiuse.
- L'audit copre il solo HEAD dichiarato. Se un writer stava scrivendo durante l'audit, le sue
  modifiche non sono coperte: serve il ri-audit sul diff finale, previsto da task-6.
- Non ho ispezionato il runtime dell'operatore (porte 9090/9091) e non ho eseguito il binario.
- La proposta §7 non è stata implementata né validata eseguendo `governance_check`.

## 10. Ri-audit finale (dopo i fix) — verifier, task-6

HEAD verificato: `6a4c745` (i dieci fix), `eff9f98` (budget meccanico), `96cd03a` (docstring del test
issue-8), `44e72b8` (inoltro dello stop reason attraverso i decoratori + copertura del percorso HTTP),
`d23a22b` (i due test dei decoratori). L'audit copre anche i file non committati presenti nell'albero,
dichiarati al Lead perché non miei: `runaction/delivery_proof_stop_reason.go` e `runapi/runs_test.go`
(poi committati in `44e72b8`) e i due file di test dei decoratori, che ho verificato mentre erano
ancora untracked e che sono stati committati in `d23a22b` **senza modifiche** — `git status` non li
segnala, quindi i byte su cui ho eseguito gli esperimenti sono esattamente quelli committati.

Ri-audit dopo `44e72b8`: le righe aggiunte dai suoi tre file non contengono **alcun** nome di agente o
provider; l'unica occorrenza è `"agent_id": "opencode"` in `runapi/runs_test.go`, cioè una fixture di
test, e i budget escludono per costruzione i `*_test.go`. I due file nuovi di produzione
(`delivery_proof_stop_reason.go`, l'aggiunta in `watchdog.go`) sono agnostici: inoltrano una capacità
dichiarata come metodo, senza sapere chi sia l'agente. Prova negativa e gate invariati: exit 1,
`GOVERNANCE_CHECK_OK`, `failures: 0`.

Esito: **nessun ramo e nessuna tabella per nome di agente o provider introdotti dai fix.**

| Prova | Comando | Esito |
|---|---|---|
| Nomi di agente/provider nelle righe AGGIUNTE del diff | `git diff -U0 -- cmd internal pkg \| rg '^\+' \| rg -i '<nomi>'` | una sola riga, ed è un commento in `internal/providers/exec/exec_unixlike_test.go` che descrive la allowlist RIMOSSA |
| Identificatori (`Codex`, `IsCanonical…`) nelle righe aggiunte | `rg -i 'codex\|IsCanonical\|CanonicalRegistryAlias'` sulle stesse righe | sempre e solo quel commento |
| File di produzione NUOVI (8) | grep di nomi su ciascuno | zero occorrenze |
| Prova negativa | `rg -i 'mimo\|minimax\|halfpocket'` in `cmd internal pkg scripts`, senza `*_test.go` | exit 1, **zero occorrenze** |
| Nuove tabelle in produzione | ispezione delle righe aggiunte con `[]string{`/`map[string]` | nessuna chiave di identità: copia di args, mappe di diagnostica, metadati di evento (vedi §6) |

`workspaceDirFlags` (`internal/providers/agents/acp_transport.go:124`,
`{"--cwd","--chdir","-C","-w","--working-directory"}`): **LEGITTIMO**, verificato nel contesto
(righe 90-124). Sono spelling di flag di riga di comando applicate a qualunque programma, non nomi di
agente o provider: `declaredWorkspaceDir` riconosce `-C /dir` e `-C=/dir` senza sapere chi sia
l'agente, e il consumatore `verifyWorkspaceAgreement` rifiuta con un errore che nomina i tre path.
Limite dichiarato: la copertura dipende dall'insieme di spelling (un `--workdir` non sarebbe
riconosciuto). È un limite di copertura, non una violazione di agnosticismo.

S1 è risolto in modo agnostico: `HasExecutable` non contiene più la allowlist e decide su una
proprietà osservabile dell'ambiente (`nvmInitScript()`), con il commento che lo dichiara. S2-S7
restano preesistenti e **non estesi** dai fix (verificato file per file): `router_observer_content.go`,
`agentmgr/installer.go`, `agentlaunch/policy.go`, `agentlaunch/codex.go`, onboarding,
`registry_client.go`.

Il vincolo è ora **meccanico**: `governance/manifest.toml` contiene tre `pattern_budget`
(`agent_name_literals_in_logic` max 16, `adhoc_agent_name_literals` max 0,
`agent_identity_branch_shape` max 2), tarati sul conteggio reale dell'albero finale e documentati nel
`reason`. Prova di dente sull'albero reale: un ramo finto `agentID == "brandnewagent"` — nome che non
esiste in nessuna lista — fa fallire il gate con `count 4 exceeds max 2`, e `"mimo"`/`"opencode"`
fanno fallire gli altri due; rimossa la sonda, `GOVERNANCE_CHECK_OK`. Limite residuo dichiarato nel
manifest: una allowlist su una variabile generica con nome nuovo (`if name == "newagent"`) non è
coperta, perché contare ogni `== "` colpirebbe `kind == "acp"` e il check verrebbe disattivato.

## 11. Verdetto di verifica indipendente (task-7) — verifier

Metodo: per ogni correzione ho **revertito io** la parte essenziale, eseguito il test che l'autore
presenta come prova, ripristinato e rieseguito. Ogni ciclo è chiuso con restore verificato via
`sha256` e `git diff --stat` vuoto. Un test che passa in entrambi i casi è dichiarato senza dente.

| Cluster | Esperimenti | Con dente | Senza dente / non provati |
|---|---|---|---|
| T1 esito/errori | F, G, I, J, **H2**, AA, AB, AC | F turno vuoto = fallimento (3 test); G niente `end_turn` inventato (4 test); I `(map[])` riprodotto alla lettera; J diagnostica strutturata (3 test); **H2** chiuso da `44e72b8`: il revert compila e fa fallire 4 sotto-prove sul percorso HTTP reale; AA watchdog che non inoltra → fallisce la sotto-prova `with_activity_watchdog` (`run record stop reason = "unreported", want the reason the provider reported`) e i test del decoratore; AB delivery proof che non inoltra → fallisce il suo test; AC inverso → `the decorator altered the reported reason: []string{"end_turn"}` | **H** (storico, a `eff9f98`): `runapi/runs.go:143` non era coperto da alcun test — **ora lo è**. Nessuno scoperto in questo blocco |
| T2 workspace | K, L, N, M2, O | K grant sul path risolto (sintomo `workspace path must be absolute`); L rifiuto tipizzato id+path (2 test, l'HTTP passa da 409 a 201 completed); N indice ambiguo; M2 affinità dell'indice; O evidenza prima del prompt (2 test) | **M**: la metà "path" di `sessionMatchesWorkspaceHints` (riuso della sessione ATTIVA) — 70 test passano senza |
| T3 args/cwd | A, B, C, D2, E | A risoluzione registro (4/4 test); B ramo binario (1 test dedicato); C riparazione idempotente (3 test); D2 cwd del figlio (2 test, `its working directory was inherited, not governed`); E gate pre-fork (2 test) | nessuno; ma B è coperto **solo** dal test dedicato: l'end-to-end è mascherato dalla riparazione (docstring corretta in `96cd03a`) |
| T4 attestazione | S, V, W4, X, Z | S conferma mai dedotta dalla richiesta (4 test, `a contradictory session model must fail closed`); V motivi distinti (2 test); W4 inoltro in `attachProofNotifier`; X inoltro nel watchdog (2 test); Z registrazione nel runnotifier (4 test) | **Y**: inoltro di `OnTurnStopReason` nel watchdog — il blocco non è in HEAD, quindi fuori dall'artefatto |
| T5 elicitation | Q, R | Q leak del registro (`registry leaked 50 expired entries`); R unsubscribe (`unsubscribe left 1 observer(s) registered`) | nessuno |
| T8 allowlist | P | P allowlist ripristinata: fallisce **solo** la sotto-prova con nome non elencato (`mimo`), mentre `node` resta verde | nessuno |

Gate sullo stato finale: `gofmt -l cmd internal pkg scripts` vuoto; `golangci-lint` **0 issues** sui 13
pacchetti verificati; `go test -count=1 -p 1` su 16 pacchetti → tutti `ok`;
`go test -race` su `elicitation`, `runtrace`, `session`, `runapi` → tutti `ok`;
`governance_check` → `GOVERNANCE_CHECK_OK` (10 pattern budget, 0 failures);
`code_governance` → `Hard Budget Failures: none`, `Quality Warnings: none`.

Non verificato, dichiarato senza addolcire:

- **Nessuna riproduzione end-to-end reale** di `-32603`, stallo, run vuoto con `end_turn`: servono un
  agente reale e il runtime dell'operatore, e le porte 9090/9091 sono fuori dal mio perimetro. Le
  riproduzioni ottenute sono a livello di test, con i messaggi d'errore verbatim del sintomo.
- **`runs.go:143`: buco CHIUSO in `44e72b8`, verificato da me.** Revertire `res.stopReason` in
  `"end_turn"` (revert H2) ora **compila** e fa fallire 4 sotto-prove, con messaggi verbatim
  `runs_test.go:1774: run record stop reason = "end_turn", want the reason the provider reported` e
  `runs_test.go:1834: run record stop reason = "end_turn", want "unreported"`. Il test legge
  `server.Store().LoadRun()` dopo `RegisterRoutes` + `mux.ServeHTTP` su `RunPathV1`: è il **percorso
  HTTP reale**, non un run sintetico — la domanda del Lead ha risposta affermativa, e la trappola di
  `jsonrpc.go:64` qui non si ripete.
- **I due test dei decoratori: RISOLTO in `d23a22b`.** `44e72b8` conteneva la produzione
  (`runaction/delivery_proof_stop_reason.go`, l'aggiunta in `runactivity/watchdog.go`) ma non i due
  test; li ho verificati mentre erano untracked e li ho segnalati come condizione bloccante per il tag.
  Il Lead li ha committati in `d23a22b` (`delivery_proof_stop_reason_test.go` +53,
  `turn_stop_reason_forwarding_test.go` +96) senza toccarli: `git status` non li segnala, quindi le
  prove AB e AC valgono per i byte committati. L'inoltro del watchdog (AA) era già coperto anche dal
  test HTTP committato (`activity_timeout_seconds` presente 3 volte in `HEAD:runs_test.go`).
- **Il punto 4 della descrizione del commit `6a4c745` è falso**: "Stalled parent/child runs expose
  last observed activity, current wait and pending requests" non è implementato — zero occorrenze in
  produzione di `last_activity|last_observed|current_wait|pending_request|wait_reason`, e `explain.go`
  non espone quei campi. Esiste solo il watchdog di inattività preesistente (`activity_timeout`).
- **Metà non provata della regola di affinità**: `internal/logic/session/manager_workspace.go:210-212`,
  il confronto sul path dentro `sessionMatchesWorkspaceHints` (riuso della sessione ATTIVA del
  canale). Revertito (esperimento M) i 70 test del pacchetto passano. La guardia gemella
  `sessionWorkspaceAffinityMatches` (`manager_workspace.go:129-137`, ripresa dall'indice) **è** coperta
  (esperimento M2, `plan would reuse a session of another workspace path`). Frase per l'evidenza: "se
  un domani un record legacy con `workspace_id` giusto e path di un altro workspace finisse nella
  sessione ATTIVA del canale, i test attuali resterebbero verdi".
- **Buco di copertura sulla cwd**: `internal/providers/agents/acp_adapter.go:27` (`Cwd: deps.Cwd`) e
  `internal/providers/agents/router.go:289` (`Cwd: cwd` nel `ConversationFactoryDeps`). Nessun test
  parte dal workspace del RUN e verifica che il valore arrivi fino a `deps.Cwd`: i test coprono
  `transportSpec.Cwd` (fork reale) e il livello factory. Frase per l'evidenza: "se un domani qualcuno
  passasse `cwd=""` da `createClient`, i test attuali resterebbero verdi". Non è una regressione: è
  copertura mancante, e il percorso è corretto per ispezione.
- **Limite residuo del budget** (§10): la forma `if name == "newagent"` su una variabile generica non è
  catturata; è dichiarato nel `reason` del budget, non nascosto.
- Gli esperimenti sono stati eseguiti mentre l'albero si muoveva: i primi (F, G, I, J, K, L, M, M2, N,
  O, A, B, C, D2, E, S, V, W4, X, Z, Q, R, P) su `96cd03a` con il blocco dello stop reason ancora
  in-flight; gli ultimi (H2, AA, AB, AC) su `44e72b8`. Ogni verdetto vale per lo stato di HEAD
  dichiarato. Nessun esperimento ha lasciato tracce: ogni ciclo si chiude con `git diff --stat` vuoto
  sul file toccato e hash del restore verificato; i revert sono stati scritti in modo da **compilare**,
  perché un errore di build non è un rosso comportamentale (eccezione dichiarata: la prima stesura di
  W, scartata e rifatta come W4).

## 12. Verdetto finale — verifier, task-7

**Tutti e dieci i cluster di fix hanno almeno una prova con dente, e ogni prova è un rosso
comportamentale ottenuto revertendo la parte essenziale della correzione.** Le uniche eccezioni sono
dichiarate qui sopra e non sono silenziose: due punti di copertura mancante (cwd run→`deps.Cwd`, metà
"path" della guardia di affinità), un limite residuo del budget di agnosticismo, e il punto 4 della
descrizione di `6a4c745` che è falso e va tolto o riscritto.

Sull'agnosticismo: nessun fix introduce un ramo o una tabella per nome di agente o provider, la prova
negativa è vuota in produzione, e la regola è ora **eseguita** dal gate invece che affidata alla
disciplina (`GOVERNANCE_CHECK_OK`, 10 pattern budget, 0 failures).

**Nessuna azione bloccante residua.** La condizione che avevo posto — committare i due file di test dei
decoratori, allora untracked — è stata soddisfatta dal Lead in `d23a22b`, senza modifiche ai file che
avevo verificato. Il mio verdetto è **positivo**: tutti i dieci cluster hanno prove con dente, il gate
è verde su `d23a22b` (gofmt pulito, 0 issues di lint, 16/16 pacchetti `ok`, `-race` `ok` su
`elicitation`/`runtrace`/`session`/`runapi`, `governance_check` e `code_governance` senza failure), e i
punti non coperti sono i tre dichiarati sopra — nessuno dei quali è una regressione introdotta dai fix.
