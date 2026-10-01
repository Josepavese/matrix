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
