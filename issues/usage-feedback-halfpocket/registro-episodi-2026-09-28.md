# Registro episodi 2026-09-28

Registro append-only. Una riga per osservazione concreta. Vecchi record non si cancellano; correzioni esplicite solo con nota `CORREZIONE: <ISO> <motivo>`. Mantainers possono aggiungere note non committate a piè di pagina senza riaprire la riga.

## Baseline tecnica

**Binario installato** (verificato `matrix version` su workstation):

- Versione: `matrix 0.1.46`
- Commit: `f0b97ab35e761cad6721a1ab985d3b51005f3e8e`
- Built: `2026-09-24T13:52:20Z`

**Sorgente letto** (`~/hpdev/Libraries/matrix`, sola lettura):

- HEAD: `2864953dad43cb9e12892b0cc3bd95377f7e0f1f` (docs `release: record v0.1.46 patch verification`).
- Nota distinzione: l'installato `f0b97ab` (commit di codice v0.1.45) precede di un commit l'HEAD sorgente `2864953`, il cui unico cambiamento è documentale. La baseline di fatto eseguita resta il commit `f0b97ab`; non va riportato `0.1.45` come "versione installata".

**Altro contesto repo**:

- Branch di lavoro: `codex/matrix-session-supervisor-20260924`. Working tree pulito a parte 3 issue file untracked pre-esistenti (letti, NON toccati) e la nuova cartella `issues/usage-feedback-halfpocket/` (untracked per design, NON da aggiungere).
- WIP altrui (untracked, NON miei, NON da pulire):
  - `issues/2026-09-25-opencode-minimax-end-turn-with-unfinished-output.md`
  - `issues/2026-09-26-external-halfpocket-mimo-completed-without-deliverable.md`
  - `issues/2026-09-27-external-workspace-grant-id-does-not-resolve-path.md`
- Repo HalfPocket: `/home/jose/halfpocket/` letto in sola lettura per evidenze (`docs/cloud-transition/evidence/2026-09-28-*.md`), nessuna scrittura.

## Episodi del 28/09/2026

### EP-01 — Registrazione MiMo via nativo funziona, registry standard no

- **Fonte/versione**: Matrix HEAD `2864953` (su base `f0b97ab`). Report HalfPocket `2026-09-28-mimo-matrix-cwd.md` righe 26–30 e 104–129.
- **Run ID osservati**: `52b0a4ef-933a-4d2b-bd34-79a8ec667edb`, `325ed3ce-ca00-4a6e-ba7a-25bfabd70eb2`, `40ca971c-79eb-4890-b979-ea3f249c06cd` (proxy MiMo, NON terza run), `b8b2257b-394a-4714-b75a-cc886b291e93` (in corso G11 al momento della nota).
- **Esito**: `matrix agent set-binary/enable` registra l'agent nel Vault SSOT (override `active: true`). `matrix agent doctor` handshake PASS; `matrix agent show` vede override. Daemon runtime invece non espone ancora `mimo` come agente ACP eseguibile, finché non idle+restart; POST `model req` riceve `HTTP 409 "model_id is supported only for ACP agents"`.
- **Impatto**: friction onboarding agent di terze parti non presenti nel catalogo upstream. ACP standard registry non offre MiMo/DSH: l'utente deve usare `set-binary` manuale.
- **Workaround**: `matrix agent set-binary` + `enable`, poi restart daemon se il child agent non appare.
- **Suggerimenti**: hot-reload agent senza restart daemon; attivazione per-agent su idle; `status=pending_apply` distinto da `status=error`; errore più preciso ("agent non ancora registrato in runtime, post-restart"). DSH assenza dal catalogo upstream NON è bug Matrix: l'utente sceglie ora OpenCode/API diretta DeepSeek (cfr. ADR 0013 in HalfPocket).
- **Stato**: `suggestion` per l'evoluzione Matrix; `certificato` per la sequenza osservata.
- **Follow-up**: nessuno in questo task. Vedi scheda `ep-01-agent-acp-native-registration.md`.

### EP-02 — Cwd del child ACP non propagata al processo `mimo`

- **Fonte/versione**: Matrix HEAD `2864953`. Log MiMo `/home/jose/.local/share/mimocode/log/2026-09-28T082215848Z-main-47605-abc0a033.active.log` + precedente `…081916385Z-main-45363-3e8f4845.log`. Report HalfPocket `2026-09-28-mimo-matrix-cwd.md` righe 32–92, 287–312.
- **Run ID**: `52b0a4ef…`, `325ed3ce…`, `40ca971c…` (proxy), `b8b2257b…` (post-fix).
- **Esito**: argv `mimo acp --cwd /home/jose/halfpocket` corretto; cwd del child MiMo resta `/home/jose/.local/share/matrix` (ereditata dal daemon Matrix). MiMo rifiuta con `directory_not_allowed` perché `halfpocket` non è discendente della propria cwd interna. `session/new` → RPC `-32603`.
- **Causa letta in Matrix** (sola lettura):
  - `internal/logic/matrixhome/home.go:28` — `os.Chdir(home)`.
  - `internal/logic/agentmgr/supervisor.go:201` — `CommandSpec` costruito senza `Dir`.
  - `internal/providers/exec/exec_unixlike.go:255` — `cmd.Dir = spec.Dir` solo se popolato.
  - `internal/providers/agents/router_clients.go:292` — `effectiveCwd(workspacePath)` esiste ma è propagato solo come `deps.Cwd`, non come `cmd.Dir` (`router.go:287-289`).
- **Workaround nativo applicato dal PM** (dopo idle, via configurazione runtime, NON wrapper su disco):
  `command=/usr/bin/env args="-C /home/jose/halfpocket /home/jose/.mimocode/bin/mimo acp --cwd /home/jose/halfpocket"` (`--args=-C` e `--args=--cwd` espliciti per pflag).
- **Verifica PM**: PID MiMo `62831`, `readlink /proc/62831/cwd` = `/home/jose/halfpocket`. Configurazione non contiene credenziali; Vault runtime non è Git.
- **Impatto**: bug di interoperabilità MiMo↔Matrix solo se l'agent impone server-cwd = process-cwd; non è prova universale Matrix. ACP workspace cwd può essere virtuale.
- **Suggerimenti**: knob `process cwd` separato da `session cwd`; validazione realpath/workspace binding; `agent doctor` probe actual child identity non `env --version`; nessuna escalation a directory globale.
- **Stato**: `certificato` (workaround applicato, cwd verificata); `suggestion` per fix strutturale; nessuna "issue upstream" creata.
- **Follow-up**: nessuno in questo task. Vedi scheda `ep-02-mimo-cwd-via-env-wrapper.md`.

### EP-03 — Workspace_id inventati → fail pre-provider; correzione path-only / `workspace add`

- **Fonte/versione**: trace Matrix osservati. Report HalfPocket non presente come singolo file; evidenza combinata da `2026-09-27-external-workspace-grant-id-does-not-resolve-path.md` (HalfPocket precedente) e pratica PM 28/09.
- **Run ID**: `41fc6d63…`, `eef05ea8e…` (primi GLM/M3 con workspace_id inventati), `54f552c5…` (MiniMax, report VPS committed ROOT main benché inteso worktree), `98447c88…`, `cab3788c…` (DeepSeek OK).
- **Esito**: workspace_id inventati → `workspace not found` pre-provider. `workspace add` + `workspace_id` esplicito + guard `pwd`/`head` hanno funzionato per DeepSeek/GLM/MM worktree.
- **Anomalia `54f552c5…`**: provider ha committato su ROOT main benché PM intendesse worktree. NON è bug Matrix provato (provider può eseguire `git -C root`). Da investigare lato provider/orchestrazione HalfPocket.
- **Impatto**: confusione tracciamento modifiche e sicurezza branch isolation quando l'orchestrazione passa solo `workspace_id` e non binding esplicito.
- **Suggerimenti**: confronto `requested workspace_id` vs `resolved workspace_id`+path+real child cwd+`git commonDir`+branch; fail-closed su mismatch; nessun fallthrough silenzioso a root.
- **Stato**: `suggestion` evolutiva; `caller_error` per i primi due (PM ha ammesso path inventato); `ipotesi` per `54f552c5…`.
- **Follow-up**: nessuno in questo task. Vedi scheda `ep-03-workspace-id-inventati-vs-add.md`.

### EP-04 — `matrix vault get` su field structured: ERR_VAULT_PARSE

- **Fonte/versione**: trace Matrix osservati dopo completion. CLI: `matrix vault get runtrace.run.<id>` su campo typed.
- **Esito**: `ERR_VAULT_PARSE object cannot unmarshal Go string`. Errore di parsing tipato su campo `string` value-vs-typed.
- **Diagnosi**: CLI usa un getter stringa sul JSON typed store; il record è typed (object), la CLI string getter non lo converte. NON è corruzione SSOT: la `runtrace` contiene payload typed, e i getter del CLI sono limitati per discovery.
- **Cosa NON provare**: decrittazione, `vault read`/estrazione chiavi, dump del DB. Limitarsi a proporre miglioramenti CLI.
- **Suggerimenti**: endpoint/CLI bounded final-only content resolver per `summary_ref`; separare `result` diagnostico dal `summary`; schema multi-livello con `help` prima, accesso per-field, size cap.
- **Stato**: `ipotesi` sul getter specifico, `suggestion` sull'evoluzione CLI; `certificato` sul messaggio di errore come superficie.
- **Follow-up**: nessuno in questo task. Vedi scheda `ep-04-cli-vault-get-typed-parse.md`.

### EP-05 — SSE run-notifications recapita a Codex senza intermediati (POSITIVO)

- **Fonte/versione**: Matrix HEAD `2864953` (su base `f0b97ab`). Run ID terminali: GLM `cef15023…`, M3 `54f552c5…`, DeepSeek `98447c88…`, `cab3788c…`.
- **Esito**: Matrix espone Unix socket `run-notifications.sock` con SSE; recapito eventi terminali a Codex via `send_message_to_thread`. Solo `run_id`/esito, nessun transcript provider né intermediati. `matrix agent doctor` handshake PASS per agent ACP.
- **Verifica indipendente PM** (riportata in `2026-09-28-deepseek-opencode-policy.md` 141–148): run `cab3788c…` termina `completed` con `model_verification=provider_confirmed`, stessa sessione logica `ad6da3b1…`.
- **Protezione SSRF intenzionale**: webhooks localhost vietati. NON è bug, è policy design.
- **Gap di ingress attuale**: adattatore minimo `Codexsend_message_to_thread` + SSE inline Python, NON bridge autonomo.
- **Suggerimenti**: CLI/SDK `run submit + wait events/cursor/ack/idempotency delivery/elicitation summary short`; non polling; durabilità reconnect/restart con delivery dedup; native user questions strict scoping.
- **Stato**: `certificato` (POSITIVO) per il recapito nativo SSE+Unix-socket; `suggestion` per ingress adattatore e primitive CLI/SDK.
- **Follow-up**: nessuno in questo task. Vedi scheda `ep-05-sse-run-notifications-positivo.md`.

### EP-06 — Model request vs effective vs provider_actual

- **Fonte/versione**: trace Matrix dopo run DeepSeek `98447c88…`, `cab3788c…`. Cache provider OpenCode non-locale.
- **Esito**: `model_request=deepseek/deepseek-flash`, `model_effective=deepseek/deepseek-flash`, `provider_confirmed`. Verifica PM: SDK provider `api.deepseek.com`, `model_name=DeepSeek V4.1 Flash`.
- **Cosa NON è prova**: API alias da datasheet (`deepseek-flash`, `deepseek-v4-flash` legacy) non è prova di TLS interception o vendor modello fisico raw. NON è OpenRouter.
- **Impatto**: serve tracciato multilivello per governance e per debug "modello sbagliato".
- **Suggerimenti**: evidenza su tre livelli: advertised (datasheet), selected (Matrix), provider_actual (opzionale); provider host metadata redatto; policy fail-closed fallback; nessuna API key nello store Matrix (cloud Vault/refresh owner semantics distinti, ADR 0003).
- **Stato**: `certificato` per la sequenza osservata; `suggestion` per il tracciato multilivello.
- **Follow-up**: nessuno in questo task. Vedi scheda `ep-06-model-request-vs-actual.md`.

### EP-07 — Quota 429 / RetryAfter / one-shot resume (SOLO PROPOSTA)

- **Fonte/versione**: nessuna evidenza di HTTP 429, `reset=5h`, `Retry-After`, né di resume automatico funzionante fornita in questo task. La nota nasce come proposta evolutiva, NON da prova osservata.
- **Esito**: nessuno attestato. Da non classificare come `certificato` né come "bug osservato".
- **Impatto atteso** (se i pattern provider esistessero davvero): PM potrebbe pianificare finestre; niente schedule-polling. Non verificato qui.
- **Suggerimenti** (proposta, vedi scheda con priorità/criteri):
  - Quota 429/RetryAfter strutturati senza schedule polling.
  - Task queue fairness/cancel per-run (no other channels).
  - Capacity metadata per workspace (es. SSD scarso).
- **Stato**: `suggestion` (solo proposta). NON `certificato`, NON bug.
- **Capacità disco**: il dato `/` 314G/292M liberi sul lavoro attuale HalfPocket riguarda la workstation del PM, NON capacità VPS (vedi VPS preflight `2026-09-28-vps-preflight.md` per la VPS, distinto).
- **Follow-up**: nessuno in questo task. Vedi scheda `ep-07-quota-429-retry-after.md`.

## CORREZIONE — 2026-09-28 (revisione PM, prevale sulla riga EP-07 sopra)

- **EP-07 corretto**: nessuna prova di HTTP 429, `reset=5h`, `Retry-After` né di resume automatico è stata fornita in questo task. Lo stato passa a sola `suggestion` (proposta), non `certificato`. NON è attestato bug.
- **Capacità disco**: il valore `/` 314G/292M liberi citato nel registro originale è riferito alla workstation del PM (HalfPocket host), NON alla VPS. La capacità VPS è attestata separatamente in `2026-09-28-vps-preflight.md` (`/` LVM 39G, 25G liberi). Non mischiare le due fonti.
- **Baseline installato**: la versione installata è `matrix 0.1.46`, commit `f0b97ab35e761cad6721a1ab985d3b51005f3e8e`, built `2026-09-24T13:52:20Z` (verificato `matrix version`). NON va riportato `0.1.45` come "versione installata". HEAD sorgente `2864953` è un commit documentale sopra `f0b97ab` e va tenuto distinto.
- **EP-06 cache OpenCode**: la cache è LETTA localmente (workstation PM); NON include nessuna promessa su alias legacy o fatturazione `DeepSeek-V4.1-Flash` non necessaria, né provata dal singolo run.
- **EP-01 DSH**: assenza osservata nel runtime HalfPocket PM, NON inferita dal catalogo globale ACP senza verifica dedicata. Il riavvio daemon per rendere operativo un agent registrato NON è classificato come problema intrinseco ACP.
- **EP-02 workaround**: confermato Linux-only via `env -C`. NON si propongono `brew install coreutils` o wrapper `bash -c` come fallback di questa nota.
- **EP-04**: il getter stringa su record typed è errore d'uso/limite del getter CLI osservato, NON corruzione del Vault SSOT.
- La riga EP-07 sopra resta come cronaca della versione iniziale della nota; il presente riquadro di correzione prevale.

## Process note (futuro)

Ad ogni nuova anomalia reale:

1. Aprire immediatamente un episodio minimo in `issues/usage-feedback-halfpocket/` con nome `ep-NN-<slug>-YYYY-MM-DD.md`.
2. Registrare: data ISO, versione Matrix (`matrix version`), `commit` installato, `full run_id` se disponibile, prova minima, sintomo, rimedio (se applicato), riferimento a issue pertinente già esistente (NON duplicarla).
3. Correggere versioni successive via riquadro `CORREZIONE: <ISO> <motivo>` in coda al file coinvolto, NON riscrivendo le righe storiche. Il riquadro prevale.
4. NON inserire transcript provider, ragionamento intermedio, secret, token, header values, customer data.
5. Nessun `git add/commit/push` su `usage-feedback-halfpocket/` per design.

## Note di chiusura giornaliera

- Nessuna patch Matrix scritta. Nessun commit. Nessun `git add`.
- Nessuna azione su provider, runs attivi, daemon Matrix, servizi HalfPocket o VPS.
- Source artifacts del 2026-09-28 NON cancellati; nessuna pulizia WIP altrui.
- Cartella `usage-feedback-halfpocket/` rimane `untracked` (per design: è archivio note esterne, NON versionato come richiesto da Jose).

## Spazio per note non committate dei maintainers

(Sezione vuota. Chi vuole può annotare qui sotto senza riaprire righe sopra.)
