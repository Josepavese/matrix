# Esporre evidenza privacy-safe del catalogo tool offerto per richiesta ACP

Segnalazione esterna di Josepavese, 14 novembre 2026, issue
[#20](https://github.com/Josepavese/matrix/issues/20). Conservata verbatim; la
decisione Matrix segue in coda.

## Originale

Please expose privacy-safe, provider-owned evidence of the tool catalog actually offered to a model for an ACP run. Configured capabilities, executed calls and a model's statement about availability are different observations.

Observed on Matrix 0.1.60 (`9fdf7e5ed839f968b8a1b825ae9c7beb40bab6d2`) with OpenCode 1.18.32:
- One existing logical session repeatedly completed with no tool calls while reporting search unavailable.
- Another logical session on the same agent/workspace/model executed native search successfully.
- Read-only metadata checks found the same OpenCode agent/mode/model/directory, no session permission override, and no per-message tools override.
- Endpoint configuration declared search enabled. A sampled child had the expected flags, but that sample was not correlated to each disputed request and is not evidence of its request catalog.
- Existing OpenCode logs and ACP session/run metadata did not contain the offered tool names or their digest. Therefore the observed difference cannot be conclusively assigned to missing tools, a provider/session filter, cached environment, or model behavior.

Relevant provider boundary:
- [OpenCode resolves session tools](https://github.com/anomalyco/opencode/blob/v1.18.32/packages/opencode/src/session/tools.ts).
- [Final request preparation filters them](https://github.com/anomalyco/opencode/blob/v1.18.32/packages/opencode/src/session/llm/request.ts#L208).
- [ACP session load/prompt](https://github.com/anomalyco/opencode/blob/v1.18.32/packages/opencode/src/acp/service.ts) does not expose that final model-request catalog in its normal session response.

Requested owner-supported observation:
1. For each actual model request, publish an optional adapter/provider attestation containing normalized offered tool names and a canonical digest, provider/runtime version and correlation to the native run/request.
2. Make clear whether the evidence is configured, observed-before-dispatch, executed, refused, or unavailable. Absence of instrumentation must remain `not_observed`, never an empty catalog or success.
3. Keep it separate from prompts, message bodies, raw thoughts, arguments, credentials and tool results. Preserve existing sessions/history and authorization; observing inventory must not grant or execute tools.
4. Provide a bounded public CLI/API/trace read path and qualify resumed sessions, role-denied profiles and a provider without this observation capability.

No claim is made that the model's refusal proves a missing catalog. No reset, forced cache change or external code patch was performed. Please diagnose and release the supported owner path.

Duplicate searches for catalog/inventory/offered/toolnames found no match. #18 is about summaries of executed results, and #19 is a separate cached-environment comparison gap; neither supplies this evidence.

---

## Decisione Matrix (2026-11-14)

**Decisione: accettata come richiesta di osservabilità. Risposta con il limite,
non con codice che narra una garanzia: il catalogo offerto al modello appartiene
al provider e ACP v1 non lo trasmette.**

### Diagnosi della frontiera

Matrix è il client ACP: l'agente (qui OpenCode) è il server che costruisce la
richiesta al modello. Il catalogo dei tool offerti a quella richiesta vive dentro
il provider; `initialize` e `session/new` trasmettono capability e fatti di
sessione, non i nomi dei tool inseriti nel corpo della richiesta. Matrix non può
quindi attestare quel catalogo né correlare un campione di processo alla singola
richiesta: un'attestazione con nomi e digest sarebbe narrazione, non osservazione,
e la sua assenza verrebbe letta come catalogo vuoto o come successo.

### Cosa è stato costruito

- **La frontiera è ora esplicita nel contratto del trace.** Nuova sezione [What
  the trace cannot observe](../../docs/matrix_agent_communication_run_trace.md):
  assenza = `not_observed`, mai catalogo vuoto o successo; capability
  configurata, chiamate eseguite e affermazioni del modello sono osservazioni
  diverse e nessuna è il catalogo della richiesta; l'evidenza va ottenuta dalla
  strumentazione del provider, correlata alla richiesta nativa; osservare
  l'inventario non autorizza e non esegue tool.
- **Nessun percorso verticale per un provider.** Il limite vale per ogni agente e
  ogni canale; non è stata aggiunta una mappatura OpenCode-specifica.

### Cosa NON è stato fatto, e perché

- **Nessuna attestazione con nomi/digest del catalogo offerto.** Non è osservabile
  al confine Matrix: richiederebbe strumentazione del provider (OpenCode) che
  esponga il catalogo finale preparato per la richiesta, correlato alla richiesta
  nativa. Costruirla dai flag dell'endpoint o dalle capability dichiarate sarebbe
  la garanzia falsa che il contratto vieta.
- **Nessuna nuova CLI/API/trace read path per quel dato.** Non c'è un dato owner
  da leggere; un campo che dice sempre `not_observed` non aggiunge osservazione.
- **Nessuna qualifica di sessioni riprese o profili role-denied per questa
  evidenza.** La qualifica richiesta presuppone il dato; senza strumentazione del
  provider non esiste un esito da qualificare.
- **Nessuna modifica ad autorizzazioni, sessioni o storico.** La richiesta chiede
  che l'osservazione non conceda né esegua tool: qui non si osserva affatto, e
  nulla viene concesso.

### Prove

- Contratto del trace aggiornato con la sezione dedicata; nessuna superficie
  dichiara ora un catalogo offerto che Matrix non possiede.
- `scripts/quality_gate.sh --static` e `scripts/release_gate.sh` passano sulla
  candidata; suite completa verde.
- Rilascio v0.1.64: [note di rilascio](../../docs/governance/releases/2026-11-14-v0.1.64.md).
