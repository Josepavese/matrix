# Fase 7 — audit totale: agnosticismo, superfici nuove, qualità

**Auditor**: verifier (team lead) · **Data**: 2026-10-02
**Oggetto**: `76499bd..49495e2750f2f238dbab1b4c741f1ee370ed1b5d` — 66 file, +3643/−457, 4 commit
(`f28c19b` release gate, `6f1dac2` ambiente del figlio + la procedura smette di ramificare sui nomi,
`6b4981d` domanda di elicitation + finestra di idempotenza, `49495e2` rimedio allo stato runtime).
**Metodo**: ogni affermazione è coperta da un esperimento (revert, sonda, misura) oppure è etichettata
**ipotesi**. Una misura è la misura dell'albero su cui è presa: tutti i numeri qui sono presi su
`49495e2` con l'albero pulito.

---

## 0. Congelamento e push

- `HEAD = 49495e2750f2f238dbab1b4c741f1ee370ed1b5d`, albero pulito (restano due voci `issues/`
  non tracciate, di altri).
- La rivendicazione «pushato» **è vera sul ramo che conta**: `git ls-remote origin` →
  `refs/heads/main = 49495e27…`. Il ramo locale di lavoro dichiara `[ahead 38]` solo perché il suo
  upstream (`origin/codex/matrix-session-supervisor-20260924 = 2864953`) è fermo: è un artefatto di
  bookkeeping, non un commit non spinto.
- Gate di rilascio (`scripts/release_gate.sh --ref 49495e2`): **tutti i controlli di contenuto verdi**
  — build `go build ./...` per linux/amd64, windows/amd64, darwin/arm64; `go vet ./...`; race detector
  pulito su 9 pacchetti; manifest di governance soddisfatto; code governance; 13 soglie di copertura
  rispettate. L'unico FAIL è la **precondizione di albero pulito**, che scatta sulle due voci non
  tracciate di altri; con `--allow-dirty` il verdetto è `RELEASE_GATE_OK` (vedi §5).
- L'audit qui sotto non ha toccato codice di produzione: gli unici file scritti sono questo documento,
  i manifest di prova in `/tmp/p7/` e una sonda temporanea cancellata a fine corsa (§5).

---

## 1. Agnosticismo — ri-audit dei quattro ratchet

### 1.1 I numeri, misurati azzerando `max` su una copia del manifest

| Ratchet | Misurato | Atteso |
|---|---|---|
| `agent_name_literals_in_logic` | **13** | 13 |
| `adhoc_agent_name_literals` | **0** | 0 |
| `agent_identity_branch_shape` | **0** | 0 |
| `identity_comparison_shape` | **0** | 0 |

Le 13 occorrenze stanno in 11 file (alcune valgono 2): `cmd/matrix/constants.go`,
`internal/logic/agentidentity/codex.go`, `internal/logic/onboarding/auth_handler.go` (×2),
`internal/logic/onboarding/wizard_codex.go`, `internal/logic/session/manager.go` (×2),
`internal/providers/a2a/server.go`, `internal/providers/agents/router_observer_content.go`,
`internal/providers/matrixapi/server.go`, `internal/providers/runapi/types.go`.

La motivazione del ratchet afferma che «le tredici che restano sono dati, non dispatch». Verificato sui
tre siti che più somigliano a un dispatch: `runapi/types.go:127` e `matrixapi/server.go:38` sono
`defaultAgent: "opencode"` (un valore di default), `router_observer_content.go:148` legge
`meta["codex"]` da metadati prodotti dall'agente (una chiave di dato, non un ramo sull'identità).
Nessuno dei tre decide un percorso in base al nome di un agente.

### 1.2 Il check non è stato indebolito

`scripts/governance_check/` non è toccato dal diff: `git diff --stat 76499bd..HEAD -- scripts/governance_check/`
è vuoto, e nessun commit dell'intervallo lo nomina. La regola AST (`ast_identity.go`,
`ast_rule = "identity_comparison"`) è la stessa dei task precedenti.

### 1.3 Le due sonde restano rosse

Su una copia del manifest con le `reviewed_pairs` svuotate:

- `if name == "brandnewagent"` → **rosso**: `compares name against "brandnewagent"`.
- `agentID == probeNuovaCostante` (costante di pacchetto, valore `"agentedelTuttoNuovo"`) → **rosso**:
  `compares agentID against const probeNuovaCostante ("agentedelTuttoNuovo"), whose value names no
  agent in the vocabulary`.

La seconda sonda è la prova che l'estensione AST alle costanti non è una promessa: la regola legge il
valore dalla sorgente (stesso pacchetto, o qualificato via import del file + `go.mod`) e decide sulla
**forma**, usando il vocabolario solo per etichettare il reperto.

### 1.4 Le coppie revisionate

Con le coppie svuotate il check riporta **11 reperti** (4 costanti + 7 letterali); con le 9 coppie
dichiarate riporta 0. Ogni coppia dichiarata chiave una comparazione che esiste nell'albero: nessuna
coppia stantia. Le sette comparazioni della procedura guidata sono sparite insieme ai loro rami, e le
coppie sono state rimosse con loro.

### 1.5 La guarigione della procedura è proprietà, non rinomina

`internal/logic/onboarding/auth_flow_ownership.go` (nuovo, 87 righe) introduce
`selectionPreparer` / `methodStarter` / `providerAuthOwner` e smista sulla `AuthHandler` risolta dal
registro; un agente che il registro non conosce conserva il percorso generico. Il residuo di
`agentCodex` in `wizard_codex.go` (`:22` costante, `:133` `HasExecutable`, `:138` `Install`,
`:154` `agentcfg.Load`, `:169` `Runner`) è dato del flusso codex, non un ramo condiviso.

### 1.6 Il limite dichiarato, misurato

Il limite noto della regola (risultati di chiamata, dereferenze, `var` di pacchetto, shadowing locale)
non è vuoto: uno scanner AST indipendente (`/tmp/p7/limit_scan.go`) enumera **26 comparazioni
invisibili** all'albero. Tre stanno in file toccati da questo diff
(`cmd/matrix/run_notifications_client.go`, `internal/providers/runapi/notifications.go`,
`run_request.go`) e sono classificatori di protocollo/dedup. La più vicina a un nome di agente è in
`internal/logic/agentlaunch/`: `codex_policy.go:39` e `:163` (`!= CodexPolicyContractV1`) e
`policy.go:39` (`==`), tutte su `envValue(endpoint.Env, CodexPolicyContractEnv)`, cioè il **valore di
un contratto dichiarato** (`MATRIX_CODEX_LAUNCH_POLICY_CONTRACT`, `codex_policy.go:14`), non
l'identità di un agente. Giudizio: rumore accettabile, e il motivo per cui il ratchet conta 13 e non
16 è che il vocabolario è confrontato a **parola intera**: un nome di agente dentro un identificatore
più lungo (la chiave d'ambiente) non è un letterale `"codex"`. È un limite, non un difetto: va scritto
qui perché nessuno lo scopra leggendo un numero.

---

## 2. Modello di minaccia sulle superfici nuove

### 2.1 L'ambiente del processo figlio — allowlist, e `spec.Env` per scelta dichiarata

`internal/logic/childenv/env.go`: `Names = {PATH, HOME, LANG, LC_ALL, TZ, TMPDIR, SystemRoot, COMSPEC}`,
`Environment()` restituisce solo i nomi che il daemon possiede davvero (`os.LookupEnv`).
Consumatori, tutti con la sola allowlist: `pkg/zedacpstdio/transport.go:67`
(`cmd.Env = append(childenv.Environment(), spec.Env...)` — il figlio agente), `process_windows.go:24`
(l'helper di kill), `internal/logic/workspacegrant/git_env.go:16` (la sonda git),
`internal/logic/vaultsec/permissions_windows.go:21` (icacls),
`internal/logic/deliverycontract/validator.go:104` (il validatore chiamato dal chiamante).
L'ambiente del daemon non raggiunge nessuno di questi figli.

**Esperimento (revert)**: riportato `transport.go:67` a `append(os.Environ(), childenv.Environment()..., spec.Env...)`
(con l'import di `os` aggiunto, perché il revert deve compilare) il test nominato diventa rosso:

```
--- FAIL: TestTheAgentChildDoesNotInheritTheDaemonsEnvironment
    transport_env_test.go:43: the agent read MATRIX_DAEMON_KEY_SENTINEL out of the daemon's environment.
```

Ripristinato con hash verificato nella stessa tornata. La allowlist ha un dente.

`spec.Env` resta il canale per cui un endpoint dichiara le proprie variabili (le credenziali
dell'agente, le chiavi del contratto di lancio): è **configurazione dell'operatore** nella volta, non
un campo di richiesta — nessun percorso di `POST /v1/runs` scrive in `spec.Env` (ricerca sui campi di
`run_request.go`: nessun `Command`/`Env` dal corpo). **Ipotesi**: un endpoint la cui dichiarazione
finisse sotto il controllo di un agente sarebbe iniezione d'ambiente nel figlio (p.es. `LD_PRELOAD`);
oggi la dichiarazione passa dalla CLI dell'operatore e dalla volta.

### 2.2 La finestra dell'ack — opt-in, e quando scade lo dice

`internal/providers/runapi/notification_ack.go`: la finestra è **opt-in** (`notificationAckMaxAge`
legge la chiave; zero = nessuna scadenza, che è il default e l'unico valore che Matrix sceglie per un
operatore). Dentro la finestra: stessa chiave + digest diverso → conflitto; stessa chiave + stesso
digest → replay. Fuori finestra: il record «non è più un record», viene trattato come assente — la
chiave torna a «prima claim» — e **la risposta lo dichiara** (`expired` + `idempotency_window_seconds`),
invece di far sembrare un replay una chiave nuova. `sweepExpiredNotificationAcks` è ciò che rende la
finestra un limite invece di una promessa (best effort, marca l'ultimo passaggio).
Un valore di finestra illeggibile è un errore, non un fallback silenzioso.

**Esperimento (revert)**: `expired := false` → rosso,
`notifications_test.go:678: un record fuori finestra non è un replay: replayed="true"`.
**Limite dichiarato**: con la finestra al default (zero) lo store delle conferme non ha scadenza e
cresce; la sua crescita in esercizio resta **ipotesi** (non misurata), mentre con una finestra
configurata il limite è reale e misurato dal test.

### 2.3 Il testo della domanda di elicitation

`internal/middleware/elicitation_question.go`: `ElicitationQuestionMaxRunes = 200`,
`BoundElicitationQuestion` collassa gli spazi (`strings.Join(strings.Fields(m), " ")`), taglia per
**rune** e restituisce `(testo, tagliato)`. Il chiamante è
`internal/providers/runapi/notifications.go:122`, che mette il risultato nella busta della notifica.
Il testo è contenuto che l'agente (e quindi, indirettamente, l'utente) sceglie: il taglio è un
**limite di spazio**, non una redazione — un segreto scritto nella domanda viaggia nella notifica con
la stessa larghezza di qualunque altro contenuto. La superficie è autenticata (lo stream del daemon),
quindi il giudizio è: accettabile, ma dichiarato come «limitato, non filtrato».

Sul valore del limite: i test di `internal/middleware/` derivano l'attesa dalla costante stessa
(`elicitation_question_test.go:38,48,59,66,80`), quindi **da soli non pinnano il numero** — misurato:
con la costante a `1 << 20` il pacchetto resta `ok`. Il numero è però pinnato dal test di
integrazione: con la stessa mutazione `./internal/providers/runapi/` diventa rosso su
`TestTheElicitationQuestionTravelsBoundedAndMarked` (che pretende che la sua domanda lunga risulti
tagliata). Il dente c'è, ma sta a valle: chi tocca la costante deve saperlo.

### 2.4 La vista viva del registro

`internal/logic/agentmgr/registry.go`: la vista è riletta quando lo snapshot ha più di `registryTTL`
(1s) **e** a ogni miss (`Get` ricarica una volta prima di rispondere «non esiste»); un ricaricamento
che fallisce serve la vista precedente con un warning e restituisce l'errore al chiamante.
**Esperimento (revert)**: tolto il ricaricamento sul miss →
`registry_freshness_test.go:78: an agent enabled after the daemon started was refused as unknown: agent 'late-agent' not found in registry`.
Limiti: (i) la vista è viva, quindi una definizione che cambia a metà run viene raccolta al lookup
successivo — nessuna garanzia di coerenza per tutta la durata di un run; (ii) il ricaricamento sul miss
è **lavoro non autenticato amplificabile**: un chiamante autenticato che chiede molti id inesistenti
provoca un rilettura della volta per ciascuno (serializzata dal mutex). **Ipotesi** non misurata sotto
carico.

### 2.5 Hot-enable

`internal/providers/runapi/hot_enable_test.go` misura il criterio come l'operatore lo incontra: un
agente abilitato dopo l'avvio non deve ricevere il 409 che lo mandava a riavviare; in `agentmgr` lo
stesso percorso è asserito con il supervisore vero (`TestTheRunPathResolvesAnAgentEnabledAfterStartup`).
Tutti verdi. Il meccanismo è la vista viva di §2.4.
**Limite**: la vista viva risolve la *risoluzione*, non la *supervisione*. Il giro di supervisione è
`Supervisor.StartAll`, chiamato una volta sola all'avvio (`cmd/matrix/run.go:61`): un agente abilitato
dopo non viene supervisionato in background fino al riavvio. Per un ACP su stdio è il percorso normale
(parte il run); per un ACP su `ws`/`http` è esattamente la rinuncia dichiarata in §4(e).

### 2.6 I file Windows-only: tre, non due

Il diff tocca **tre** file con build tag Windows — `internal/logic/vaultsec/permissions_windows.go`,
`internal/providers/exec/exec_windows.go`, `pkg/zedacpstdio/process_windows.go` — non due.

- `permissions_windows.go:21` (icacls) e `process_windows.go:24` (taskkill /T /F, con fallback
  `Process.Kill()`) usano la **allowlist** del figlio: indurire un file o uccidere un albero non ha
  bisogno delle chiavi dell'operatore.
- `exec_windows.go` **eredita** `os.Environ()` per scelta dichiarata (§4(d)).

**Esperimento**: `GOOS=windows GOARCH=amd64 go build` sui tre pacchetti → `BUILD_WINDOWS_OK`; e il
gate di rilascio costruisce l'intero modulo per windows e darwin sull'archivio del commit.

---

## 3. Qualità: tetti e budget

- **Tetti di pacchetto = misura esatta** (verificato su `code-governance.toml`): `agentmgr` 1212,
  `onboarding` 1493, `runapi` 2640, `runtrace` 1703, `middleware` 1190.
- Il diff del file tocca **solo** questi numeri: `onboarding` 1435→1493, `runtrace` 1676→1703,
  `agentmgr` 1165→1212, `runapi` 2538→2640 (`middleware` era già 1190). Nessun override di **unità**
  (file o funzione) è stato alzato: la regola «i budget di unità non si alzano mai» regge in questo
  intervallo.
- **Conseguenza da mettere per iscritto**: un tetto uguale al misurato non può fallire da solo — ogni
  riga aggiunta obbliga a toccare il numero, quindi è un **innesco di revisione**, non un vincolo. Il
  dente è di processo: il file è dell'operatore/Lead (verificato: i tre commit dell'intervallo che
  toccano `code-governance.toml` sono tutti suoi). Se lo si volesse automatico servirebbe margine
  sotto la misura — scelta che il Lead ha esplicitamente rifiutato («un ratchet con margine non
  stringe»).
- Lo slack che avevo segnalato nella fase precedente su `runapi` (tetto 2580 su 2538 misurate) è
  chiuso: ora 2640 = 2640.
- Test dei pacchetti toccati (10 pacchetti, `-count=1`): **tutti ok**.

---

## 4. Le decisioni del Lead, messe per iscritto

### (a) I valori di `agent.config.*` non vengono letti: **contraddetto su due superfici di visualizzazione**

La decisione, come formulata, è: i valori sono credenziali, quindi non si leggono; al loro posto una
classificazione che non stampi. Nell'albero:

- `cmd/matrix/agent_show.go:60` mette `"effective": cfg`, `:71` `"override": override`, `:73`
  `"env_effect": cfg.Env` nel payload, che viene marshallato (`:104`) e stampato su stdout (`:111`).
- `agentcfg.Config` (`internal/logic/agentcfg/store.go:26-40`) ha `Env []string \`json:"env,omitempty"\``
  e `Headers map[string]string \`json:"headers,omitempty"\``: i **valori** finiscono nel JSON.
- `cmd/matrix/agent_override.go:55` stampa a sua volta l'`override` intero.

Quindi le due superfici leggono e stampano i valori. **Non è una regressione di questo diff**: le tre
righe sono presenti identiche in `76499bd` (verificato con `git show 76499bd:cmd/matrix/agent_show.go`),
e il diff lì cambia solo un commento. Il modello della «classificazione che non stampa» esiste già
nello stesso comando: `agent_endpoint.go:83` stampa `header_names` (via `agentcfg.HeaderNames`, che
dichiara «non-secret header names for display») e `agent_show.go:101` stampa `env_count`.
Nota di contesto: la politica di `internal/logic/logredact/logredact.go:11-15` esenta esplicitamente
«una superficie che l'operatore interroga di proposito» — una risposta non è un log. Quindi la
decisione va scritta **nominando la superficie**: se vale «nessuna superficie legge i valori», vanno
cambiate `agent show` e `agent override` (nomi/conteggi al posto dei valori); se vale «nessun segreto
entra nel contesto di un agente», l'albero non la contraddice e le due superfici restano l'eccezione
già documentata. **Questa è l'unica affermazione del brief che non regge come scritta.**

### (b) Nessun sandbox del validatore questo ciclo — con il progetto di cosa servirebbe

Assenza verificata: nessun `SysProcAttr`, `Unshare`, namespace, `seccomp`, `Setrlimit`, cgroup o
`Chroot` in `internal/logic/deliverycontract/` né in `internal/logic/childenv/` (ricerca vuota).
**Misurato sull'albero congelato** (sonda temporanea, cancellata): il validatore ha letto un file
**fuori** dal workspace e ne ha scritto uno **fuori** dal workspace (`MEASURED: the validator read a
file outside the workspace and wrote one outside it`). La allowlist d'ambiente chiude il canale
ambiente, non il filesystem: la contenimento di oggi è l'utente OS del daemon.
Cosa servirebbe, in ordine di costo: un utente/gruppo separato per il figlio; `rlimit`s (CPU, memoria,
file, processi) via `SysProcAttr`/`prlimit`; un cgroup (solo Linux) per memoria e PIDs; una vista di
mount con il workspace scrivibile e il resto in sola lettura (Linux: namespace di mount; macOS: nessun
equivalente senza `sandbox-exec`, deprecato; Windows: job object + ACL); un filtro `seccomp`/`seatbelt`.
Ognuno è un compromesso di portabilità su tre sistemi, ed è la ragione per cui «nessun sandbox questo
ciclo» è una decisione, non una dimenticanza.

### (c) Il recapito resta at-least-once + dedup

Dichiarato in `notification_ack.go:17` e verificato: una claim per chiave dentro la finestra; digest
diverso sulla stessa chiave → conflitto; record fuori finestra → assente **e dichiarato**; il verdetto
di consegna è valutato una volta per run sotto `deliveryMu` (`delivery.go:72`). Il dedup è largo
esattamente quanto la finestra: senza finestra configurata non c'è scadenza (e lo store cresce).
Coerente con (c).

### (d) Due politiche d'ambiente, una per fiducia

Verificato. **Allowlist** dove il figlio non è scelto dall'operatore: figlio agente, validatore
chiamato dal chiamante, sonda git, helper di kill, icacls (§2.1). **Eredità per scelta** per
`internal/providers/exec`: `exec/doc.go` documenta le due politiche e `exec_windows.go` (righe 31, 58,
103, 178, 252) usa `cmd.Env = append(os.Environ(), spec.Env...)`. I chiamanti di
`execprovider.NewProvider()` sono percorsi con comandi **dichiarati dall'operatore**: CLI
(`cmd/matrix/app.go:199,226`, `logs_helpers.go:59`, `agent_show.go:83`), sonda del dottore
(`internal/logic/agentdoctor/probe.go:23`), sonda di identità (`internal/logic/childidentity/probe.go:45`).
Nessun `argv` proveniente da un modello o dal corpo di una richiesta. **Ipotesi**: la politica
ereditante resta offerta dalla libreria, quindi un chiamante futuro potrebbe usarla per un comando non
scelto dall'operatore; il documento di `exec` esiste per rendere quella scelta visibile.

### (e) Rinuncia dichiarata: ACP `ws`/`http` supervisionato senza figlio dopo l'enable

Misurato e confermato nella sua forma precisa:

- `supervisor.go:84-85` (preesistente): il ramo supervisionato è **solo** `acp` + (`ws`|`http`);
  tutto il resto è on-demand. Il ramo supervisionato pretende un eseguibile
  (`HasExecutable(cfg.Command)`) e il `watchdog` avvia lui il figlio (`:181-200`: alloca una porta,
  inietta gli argomenti, avvia).
- Il marcatore esiste ed è onesto: `runtime_status.go:219-224` risponde `ready_on_demand` con
  «served on demand: the runtime starts this agent when a run arrives; nothing has been observed
  running yet», e ogni altro protocollo on-demand resta `pending_apply`.
- **Nessun cancel per-agente** in quel percorso: l'unico `cancel` del supervisore è il timeout della
  sonda (`supervisor.go:127,129`). E la supervisione è decisa una volta sola in `StartAll` (§2.5).
- Un endpoint remoto senza campo `Command` viene riportato `missing_executable`: misurato,
  `exec.LookPath("")` → `err=exec: "": executable file not found in $PATH` → `HasExecutable=false`.
  Cioè: il ramo continua a parlare in termini di processo figlio, e per una configurazione senza
  figlio la sua parola è fuorviante.

La rinuncia è reale e dichiarata; qui sta la sua misura, non un'obiezione.

---

## 5. Revert e verifiche eseguite in questo audit

| Verifica | Mutazione | Esito osservato |
|---|---|---|
| `TestTheAgentChildDoesNotInheritTheDaemonsEnvironment` | `transport.go:67` torna a ereditare `os.Environ()` | **rosso**: `the agent read MATRIX_DAEMON_KEY_SENTINEL out of the daemon's environment` |
| `TestAConfiguredWindowDeclaresItselfAndTurnsAnExpiredReplayIntoAFirstClaim` | `expired := false` | **rosso**: `un record fuori finestra non è un replay: replayed="true"` |
| `TestAnAgentEnabledAfterTheDaemonStartedIsServedNotRefusedAsUnknown` | tolto il ricaricamento sul miss | **rosso**: `an agent enabled after the daemon started was refused as unknown: agent 'late-agent' not found in registry` |
| `TestBoundElicitationQuestionMarksTheCutAtTheDeclaredThreshold` | costante a `1 << 20` | **verde** (attesa derivata dalla costante): il numero è pinnato a valle da `TestTheElicitationQuestionTravelsBoundedAndMarked`, **rosso** con la stessa mutazione |
| Sonda del contenimento del validatore | nessuna (esecuzione reale di `sh -c` come validatore) | **misurato**: legge e scrive fuori dal workspace (§4(b)) |
| Build Windows dei tre pacchetti | nessuna | `BUILD_WINDOWS_OK` |
| Gate di rilascio su `49495e2` | nessuna | contenuto tutto verde; `RELEASE_GATE_OK` con `--allow-dirty` (l'unico FAIL è la precondizione di albero pulito, per due voci non tracciate di altri) |

Tutti i file mutati sono stati ripristinati con hash verificato nella stessa tornata; la sonda
temporanea è stata cancellata (`git status --porcelain` non la nomina).

---

## 6. Ipotesi e limiti, in un posto solo

1. Con la finestra di idempotenza al default (zero) lo store delle conferme non ha scadenza: crescita
   in esercizio **non misurata**.
2. Il ricaricamento del registro sul miss è amplificabile da un chiamante autenticato con molti id
   inesistenti: **non misurato sotto carico**.
3. La vista del registro è viva: nessuna coerenza garantita per tutta la durata di un run.
4. La supervisione è decisa all'avvio: un agente abilitato dopo non è supervisionato fino al riavvio
   (per `acp`+`ws`/`http` è la rinuncia (e)).
5. `spec.Env` è configurazione dell'operatore; un giorno in cui quella dichiarazione finisse sotto
   controllo di un agente sarebbe iniezione d'ambiente nel figlio.
6. `exec` eredita per scelta: la politica resta offerta dalla libreria a chiamanti futuri.
7. Il testo della domanda di elicitation è limitato a 200 rune ma non filtrato.
8. Nessun esercizio end-to-end dei trasporti `ws`/`http` in questo audit: le loro superfici sono
   giudicate dal codice e dai test dei pacchetti, non da un socket reale.

---

## 7. Gli artefatti di governance

Le tabelle di reversione nel repo contengono **126 righe** complessive (conteggio meccanico):
`docs/governance/delivery-contract-reverts.md` (51), `internal/logic/deliverycontract/REVERTS.md` (39),
`internal/logic/onboarding/REVERTS.md` (13), `pkg/zedacpstdio/REVERTS.md` (10),
`internal/logic/agentmgr/REVERTS.md` (8), `internal/logic/childidentity/REVERTS.md` (5).
Controllo meccanico: ogni nome di test citato nella prima colonna esiste nell'albero **tranne uno**.

- `docs/governance/delivery-contract-reverts.md:57` cita `TestAnIncompleteDeliverySaysWhy`, che **non
  esiste** in nessuna forma (`rg` su `*_test.go`: nessun riscontro; `git log -S` non trova né
  aggiunte né rimozioni). La riga però non è inventata: la mutazione (`verdict.explain()` rimossa,
  `internal/logic/deliverycontract/contract.go:214`) e il messaggio osservato
  (`an incomplete delivery must say why it is incomplete`) sono verbatim quelli di
  `TestCompletedRunWithoutTheDeclaredArtifactIsIncompleteDelivery`
  (`internal/logic/deliverycontract/contract_test.go:51`, messaggio a `:64`). È un **nome sbagliato**,
  non un revert falso: si corregge in una riga.

Un artefatto di governance che dice il falso è peggio di uno mancante: questa è l'unica riga, su 126,
che va sistemata prima del taglio.
