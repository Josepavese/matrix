# Archivio `usage-feedback-halfpocket` — note operative Matrix viste da HalfPocket

**Stato archivio**: append-only, solo Markdown, non versionato.
**Tipo**: note di un agente esterno (MiniMax M3, `MiniMax-M3`) incaricato da Jose del 28/09/2026.
**Regole**: nessuna patch Matrix, nessun `git add/commit/push`, nessuna scrittura nel repo `/home/jose/halfpocket`, nessuna azione mutante su provider/runs/servizi. Tutto via Matrix o sola lettura.
**Ambito**: problematiche Matrix riscontrate nell'orchestrazione AI del 2026-09-28 e proposte evolutive per orchestrare meglio il lavoro.

**Confini (autosufficiente)**: questo file è l'unica pagina indice dell'archivio. Qualsiasi file fuori da `issues/usage-feedback-halfpocket/` NON è stato toccato. WIP altrui (`issues/2026-09-25…26…27…md`) restano integri.

## Baseline di riferimento

- **Binario installato** (verificato `matrix version`): `matrix 0.1.46`, commit `f0b97ab35e761cad6721a1ab985d3b51005f3e8e`, built `2026-09-24T13:52:20Z`.
- **Sorgente letto** (`~/hpdev/Libraries/matrix`): HEAD `2864953dad43cb9e12892b0cc3bd95377f7e0f1f` (commit documentale `release: record v0.1.46 patch verification`).
- La versione installata NON è `0.1.45`. `f0b97ab` è il commit di codice della v0.1.45; `v0.1.46` è il rilascio che lo include come binario (commit `f0b97ab`, build 2026-09-24). Il commit `2864953` è solo documentale sopra `f0b97ab`.

## Regole di classificazione

Ogni nota registra esplicitamente:

- **fonte/versione** (commit Matrix letto, file HalfPocket di evidenza, run ID)
- **esito osservato** (comportamento concreto, non ipotesi)
- **impatto** (effetto pratico su PM AI / Codex / user)
- **workaround** (se applicato e come, solo se Linux/PM-locale)
- **suggestimenti** (proposte evolutive, non claim di fix)
- **stato** ∈ `{certificato, ipotesi, caller_error, provider_limit, suggestion}` — e distinzione "cosa è attestato vs cosa no" per ogni scheda.
- **priorità** P1/P2/P3 per ogni suggerimento.
- **criterio di accettazione misurabile** per ogni suggerimento (verificabile in un test futuro).

NON si archivia:

- transcript provider, ragionamento intermedio, output grezzo modello.
- secret, chiavi, token, header values, customer data.
- log lunghi: solo riferimenti per path/linea e sintesi.
- claim di "bug Matrix provato" quando l'evidenza è solo compatibile/interoperabilità.
- pattern/provider behavior NON osservati come prova (solo proposta con precondizione esplicita).

## Processo futuro

Ad ogni nuova anomalia reale:

1. Aprire immediatamente un episodio minimo in `issues/usage-feedback-halfpocket/` con nome `ep-NN-<slug>-YYYY-MM-DD.md`.
2. Registrare: data ISO, versione Matrix (`matrix version`), `commit` installato, `full run_id` se disponibile, prova minima, sintomo, rimedio (se applicato), riferimento a issue pertinente già esistente (NON duplicarla).
3. Correggere versioni successive via riquadro `CORREZIONE: <ISO> <motivo>` in coda al file coinvolto, NON riscrivendo le righe storiche. Il riquadro prevale.
4. NON inserire transcript provider, ragionamento intermedio, secret, token, header values, customer data.
5. Nessun `git add/commit/push` su `usage-feedback-halfpocket/` per design.

## Indice

| File | Episodio | Stato principale |
| --- | --- | --- |
| [README.md](./README.md) | Questo indice, regole, processo futuro | sempre valido |
| [registro-episodi-2026-09-28.md](./registro-episodi-2026-09-28.md) | Registro giornaliero episodi 2026-09-28 (include riquadro `CORREZIONE 2026-09-28`) | append-only |
| [REVISIONE-PM-2026-09-28.md](./REVISIONE-PM-2026-09-28.md) | Verifica indipendente, capacita' native gia' presenti, fonti preesistenti ed EP-08 (porta sbagliata) | prove limitate + caller_error |
| [ep-01-agent-acp-native-registration.md](./ep-01-agent-acp-native-registration.md) | MiMo registrato via nativo `set-binary/enable` ma assente in runtime prima di idle+restart | suggestion + certificato limitato |
| [ep-02-mimo-cwd-via-env-wrapper.md](./ep-02-mimo-cwd-via-env-wrapper.md) | Cwd MiMo non propagata: workaround `env -C` Linux verificato dal PM | certificato + suggestion |
| [ep-03-workspace-id-inventati-vs-add.md](./ep-03-workspace-id-inventati-vs-add.md) | Workspace_id inventati falliscono; correzione path-only / `workspace add` esplicito | suggestion + caller_error + ipotesi |
| [ep-04-cli-vault-get-typed-parse.md](./ep-04-cli-vault-get-typed-parse.md) | `matrix vault get` errore d'uso getter stringa su record typed | certificato (superficie) + suggestion |
| [ep-05-sse-run-notifications-positivo.md](./ep-05-sse-run-notifications-positivo.md) | SSE Unix-socket recapito Codex senza intermediati | certificato (positivo) + suggestion |
| [ep-06-model-request-vs-actual.md](./ep-06-model-request-vs-actual.md) | `model_request != provider_actual`: tracciato multilivello proposto | certificato (limitato) + suggestion |
| [ep-07-quota-429-retry-after.md](./ep-07-quota-429-retry-after.md) | Quota 429/RetryAfter/one-shot resume: SOLO proposta (nessuna prova) | suggestion (precondizione P1) |

## Confini operativi

- Scrittura consentita: solo qui in `issues/usage-feedback-halfpocket/`.
- Nessun file Matrix sotto `cmd/`, `internal/`, `docs/`, `installer/`, `configs/`, `test/`, `AGENTS.md`, `AGENT_GOVERNANCE.md`, file di config o secrets viene editato.
- Nessun file HalfPocket sotto `/home/jose/halfpocket/` viene editato.
- `apply_patch` (tool opencode) usato solo per scrivere le note in questa cartella.

## Revisioni archivio

- 2026-09-28: creazione archivio, baseline installato `matrix 0.1.46` (`f0b97ab`) + HEAD sorgente `2864953`, 7 episodi registrati, 0 chiusure.
- 2026-09-28 (revisione PM): riquadro `CORREZIONE` in registro; EP-07 riclassificato a sola proposta; baseline installato verificata con `matrix version`; ogni scheda con priorità P1/P2/P3 + criterio di accettazione misurabile; processo futuro documentato.

## Aggiunta PM esterno — 29 settembre 2026

- [EP-09: live context MiMo non supportato](./ep-09-mimocode-live-context-unsupported-2026-09-29.md):
  prova API/eventi nativi, limite di interoperabilità non attribuito a bug
  Matrix, proposta P2 di discovery preventiva; nessun intervento sul runtime.
  Nota aggiunta da Codex PM, distinta dal mandato MiniMax storico sopra.

---

## NOTA MANUTENTORE — 2026-10-02 (Matrix v0.1.49, commit `bd4316be`)

Nota non committata, come da processo di questo archivio. Nessuna riga storica
sopra è stata riscritta. Stato di ogni episodio rispetto al codice rilasciato;
ogni voce dice cosa è stato fatto e cosa **no**.

| Episodio | Stato | Cosa c'è / cosa manca |
| --- | --- | --- |
| **EP-01.A** | **fatto** | `agent show` distingue `runtime.status=pending_apply` da `error`, con la remedìa accanto. |
| **EP-01.B** (hot-reload) | **non fatto** | Nessun riavvio scoped né ricarica a caldo: richiede un cambio del ciclo di vita del runtime. Dichiarato. |
| **EP-01.C** | **fatto** | Il 409 nomina l'agente e l'azione risolutiva. |
| **EP-01.D** (DSH nel catalogo) | **non fatto** | Richiede una query al catalogo ACP esterno, che Matrix non possiede. |
| **EP-02.A** (`process_cwd`) | **fatto** | `MATRIX_AGENT_PROCESS_CWD` → `CommandSpec.Dir`, con rifiuto del valore inutilizzabile e nessun fallback globale. Il percorso run ACP-stdio continua a usare il workspace per-run: due meccanismi sovrapposti sarebbero peggio di un limite scritto. |
| **EP-02.B** | **fatto** | Validazione pre-fork del workspace, fail-closed (`matrix_workspace_not_found`, `ResolveIdentity`). |
| **EP-02.C** | **fatto** | Il doctor legge `child.{cwd,argv}` da `/proc/<pid>/{cwd,cmdline}` del child reale, non dal wrapper. |
| **EP-02.D** | **fatto** | Nessuna escalation a directory globale: il valore inutilizzabile è rifiutato. |
| **EP-03.A** | **fatto parziale** | `workspace_requested`/`workspace_resolved`/`workspace_not_derived` nell'artefatto, mismatch → fail-closed. **`git_common_dir` e `branch` NON sono derivati**: leggerli significa eseguire git sul checkout del chiamante, e Matrix non fa il mestiere del VCS. L'artefatto lo dichiara invece di inventare un valore. |
| **EP-03.B** | **fatto** | Workspace id non risolvibile → la run non è eseguita, mai degradata a root. |
| **EP-03.C** (`54f552c5`) | **non fatto** | Richiede i log del provider: Matrix non li possiede, e l'attribuzione Matrix/MiMoCode non è stata inventata. |
| **EP-04.A-E** | **fatto** | `vault summary` bounded (cap 32 KiB, rifiuto esplicito), `vault result` senza contenuto, `--help` con campi e tipi prima del parse, getter documentati, cap per campo. |
| **EP-05.A** | **fatto** | `run submit`, `run wait` con cursore e `ack` idempotente (esattamente una volta lato Matrix). L'elicitation è nella sintesi (`id/state/session/since`). |
| **EP-05.B** | **fatto** | Prova end-to-end di reconnect/restart con dedup: crash del destinatario, riapertura dal cursore, un solo esito logico. |
| **EP-05.C** | **fatto parziale** | Le domande arrivano come eventi distinti e sono rispondibili via SDK. **Il testo della domanda non è esposto**: mettere contenuto dell'utente in un payload di notifica è una decisione di redazione (`applyTracePolicy`), non un campo da aggiungere per strada. |
| **EP-05.D / E** | **invariati** | Unix socket + SSE confermati; il bridge resta adattatore. |
| **EP-06.A** | **fatto** | `requested_model` / `effective_model` / `model_verification` con `verification_reason` ed `evidence_source`; sessione calda non ri-attestabile → `unverified` onesto. **Attenzione**: `model_fallback_used` è `omitempty`, quindi la sua assenza **non** prova `false`; il campo che risponde sempre è `fallback_used`. |
| **EP-06.B** | **fatto** | `provider.host` redatto nei log di default (`MATRIX_LOG_REVEAL_ENDPOINTS=1` per vederlo). Il report del doctor **non** è un log e continua a rispondere. |
| **EP-06.C** | **fatto** | `fallback_model_id` diverso è l'unica autorizzazione al fallback. |
| **EP-06.D** | **soddisfatto in sostanza, non come formulato** | Nessun segreto provider in chiaro (699.436 chiavi, plaintext 0, AES-256-GCM, `-rw-------`), nessun namespace di segreto vagante. Ma **le credenziali degli agenti esistono per design** dentro `agent.config.*` (cifrate): senza, Matrix non potrebbe lanciare agenti che si autenticano. I **valori** di quelle 8 voci non sono stati letti. |
| **EP-07** | **non fatto, per la vostra precondizione** | Nessun 429 reale è stato raccolto, quindi nessuna proposta P3 è stata implementata. È il vostro criterio P1, e lo rispetto. |
| **EP-08** | **fatto** | `matrix runtime endpoints [--json]` sul descriptor già pubblicato dal broker, mai sul log. |
| **EP-09a** (`trace_policy`) | **fatto** | Sintattico contro schema, con field path e tipo atteso. Un corpo troppo grande è un **terzo** caso (413) e non viene ricondotto a "invalid json". |
| **EP-09b** (`attach_context`) | **fatto** | `matrix agent doctor` espone la capability a tre livelli separati (trasporto / provider / consegna), letti dai **record reali** che la run action scrive, non dedotti. `Promised` è vero **solo** con consegna provata: accettazione non è consegna. Prima di ogni tentativo resta `unobserved`, e un record `pending` o `failed` **non** è una risposta sul provider. L'attribuzione provider/adapter/runtime **resta aperta**. |
| **EP-10** | **fatto** | `protocol_status` ≠ `acceptance_status`, contratto dichiarato prima del run, verdetto stabilito una volta sola come evento. Il validatore del chiamante gira come argv, senza shell implicita, con ambiente proprio e **solo l'exit code** letto. **Limite**: il contenimento del validatore è l'utente OS del daemon, non l'allowlist — un validatore legge e scrive fuori dal workspace. |

**Tre difetti trovati eseguendo i comandi**, non leggendo il codice, e ora chiusi:
il client locale leggeva `daemon_api_key` dove la superficie impone `matrix_api_key`
(ogni comando locale rispondeva 401, incluso il preesistente `matrix run wait`);
`agent show`/`agent doctor` scrivevano su **stderr**, quindi la ridirezione produceva
file vuoti; e la sonda git che decideva un grant lasciava `GIT_DIR` spostare il
common dir sul quale il grant è chiavato — **autorizzazione cross-repository**, non
un rifiuto come si era creduto prima di misurare.

**Cosa resta, elencato per non perderlo**: crescita illimitata dei record di ack
(ipotesi non misurata); finestra di riuso pid in `/proc/<pid>` (ipotesi);
`zedacpstdio` passa al figlio agente tutto l'ambiente del daemon — questione di
perimetro, non difetto, ma la forma della soluzione è un'allowlist minima più
`spec.Env`; i due rami device-auth del flusso di onboarding restano **guardie**
(uno di sicurezza verificato: senza, un agente estraneo ottiene un URL OAuth reale
con PKCE), non violazioni di agnoscità; e la distinzione deadlock/retry/quota/LLM
**non è derivabile** da ciò che Matrix registra oggi.

---

## CORREZIONE: 2026-10-02 — due difetti di questo indice (prevale sulle righe sopra)

Nota da manutentore, non committata, in coda come da processo. Le righe storiche
sopra non sono state toccate: questo riquadro **prevale**.

**1. La numerazione è in collisione.** `EP-09` indica **due episodi diversi**:
il *trace_policy con tipo sbagliato* (scheda dentro
`REVISIONE-PM-2026-09-28.md`) e il *live context MiMo non supportato*
(`ep-09-mimocode-live-context-unsupported-2026-09-29.md`). Sono due fatti
distinti, con cause e remedie diverse. Per riferirsi senza ambiguità:

| riferimento univoco | episodio | dove sta |
| --- | --- | --- |
| **EP-09a** | `trace_policy` con tipo sbagliato → 400 "invalid json" su JSON valido | `REVISIONE-PM-2026-09-28.md` |
| **EP-09b** | live context MiMo (`attach_context`) non supportato | `ep-09-mimocode-live-context-unsupported-2026-09-29.md` |

**2. L'indice è incompleto: `EP-10` non compare da nessuna parte**, ed era il
**P1 più grosso dell'intero archivio**. Sta in `REVISIONE-PM-2026-09-28.md`, la
cui riga d'indice sopra nomina solo EP-08. Riga mancante:

| File | Episodio | Stato principale |
| --- | --- | --- |
| [REVISIONE-PM-2026-09-28.md](./REVISIONE-PM-2026-09-28.md) | **EP-10** — `completed` non certifica accettazione né correttezza del report | `suggestion` **P1** (contratto di consegna) |

Conseguenza pratica di entrambi: **11 episodi, 10 etichette**, e il più
importante era il non indicizzato. Chi cerchi per numero trova due cose
diverse; chi cerchi per indice non trova il P1.
