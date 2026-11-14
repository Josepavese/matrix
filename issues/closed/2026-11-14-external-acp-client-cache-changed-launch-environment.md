# ACP client cache ignora un ambiente di lancio cambiato senza sandbox

Segnalazione esterna di Josepavese, 14 novembre 2026, issue
[#19](https://github.com/Josepavese/matrix/issues/19). Conservata verbatim; la
decisione Matrix segue in coda.

## Originale

An existing ACP client can remain reusable after its resolved launch environment changes when both endpoint declarations have no sandbox.

Confirmed by source inspection of Matrix 0.1.60, commit `9fdf7e5ed839f968b8a1b825ae9c7beb40bab6d2`, and the same branch on current main (observed 2026-11-14):
- [cachedLaunchMatches](https://github.com/Josepavese/matrix/blob/9fdf7e5ed839f968b8a1b825ae9c7beb40bab6d2/internal/providers/agents/router_sandbox.go#L25) returns true immediately when both `Sandbox` fields are nil, before comparing serialized endpoint declarations.
- [getOrCreateClientLocked](https://github.com/Josepavese/matrix/blob/9fdf7e5ed839f968b8a1b825ae9c7beb40bab6d2/internal/providers/agents/router_clients.go#L181) then reuses the cached client.
- The [cache key](https://github.com/Josepavese/matrix/blob/9fdf7e5ed839f968b8a1b825ae9c7beb40bab6d2/internal/providers/agents/router_client_keys.go#L34) includes agent, workspace and launch arguments, not endpoint environment.

Static reproduction (branch evaluation, not a claimed live reproduction):
1. Cache a nonsandboxed ACP client with `OPENCODE_ENABLE_EXA=0`.
2. Change the resolved endpoint override to `OPENCODE_ENABLE_EXA=1`, keeping agent/workspace/launch arguments unchanged.
3. Both sandbox fields remain nil, so `cachedLaunchMatches` returns true despite different environments. The retained child was started with the old environment.

Expected: a changed launch environment must be detected for nonsandboxed clients too. Preserve active leases and unrelated sessions; fail closed with the existing explicit reap/relaunch workflow instead of silently presenting the updated endpoint configuration as the running child's configuration. Please qualify partial/lost acknowledgements and unchanged-environment replay.

This is a source-confirmed cache comparison gap. It is **not established as the cause** of a separate observed model refusal to call search; do not infer that causal link from this report.

No external code or runtime was modified. Duplicate searches for cachedLaunchMatches/environment/override found no matching issue; #18 concerns trace result summaries and is separate.

---

## Decisione Matrix (2026-11-14)

**Decisione: accettata. Difetto reale, confermato nel codice della candidata.
Corretto e rilasciato in v0.1.64.**

### Diagnosi

`cachedLaunchMatches` usciva con `true` quando entrambe le `Sandbox` erano nil,
prima di confrontare le dichiarazioni serializzate. La chiave di cache copre
agente, workspace e launch arguments, non l'ambiente risolto: un endpoint senza
sandbox con `Env` diverso continuava quindi a riusare il figlio avviato con il
vecchio ambiente, e la configurazione aggiornata veniva presentata come quella
del figlio in esecuzione.

### Cosa è stato costruito

- **Il confronto è completo per ogni client ACP.** Rimosso il ritorno anticipato:
  la dichiarazione risolta del client cached e quella risolta ora vengono
  serializzate e confrontate, sandbox o no. Un cambiamento di ambiente,
  argomenti, comando, indirizzo o altro campo di lancio invalida il figlio.
- **Il mismatch fallisce chiuso e preserva ciò che è in volo.** Il dispatch viene
  rifiutato con `launch declaration changed: reap the previous workspace client
  before dispatch`; il client cached non viene espulso né chiuso, la lease attiva
  resta intatta, le altre sessioni non vengono toccate. Il reap/relaunch resta il
  flusso esplicito esistente; Matrix non presenta la nuova configurazione come
  quella del figlio vecchio.
- **La dichiarazione invariata rigioca il client cached.** Il riuso non diventa
  un ciclo di riavvii: con la stessa dichiarazione risolta il client viene
  riusato.
- **Regressioni**: ambiente cambiato senza sandbox → niente match; mismatch con
  lease attiva → errore e client preservato; dichiarazione invariata → replay
  dello stesso client. Il test sandbox esistente continua a passare. Due
  scaffolding di test sono stati corretti per modellare client reali invece del
  vecchio bypass.

### Cosa è stato risposto con un limite, non con codice

- **Nessun reap automatico del figlio in volo.** La richiesta è preservare le
  lease attive e fallire chiuso con il flusso esplicito: chiudere il processo
  sotto una lease viva sarebbe l'opposto. Il mismatch viene riportato e il client
  resta finché le sue lease lo consentono.
- **Un resolver che non risponde ora fa fallire il riuso** invece di riusare un
  client senza ricontrollarlo. È il prezzo del fail closed e coincide con il
  percorso sandbox già esistente.
- **Acknowledgement parziali o persi restano evidenza del ciclo di turno**, non
  dell'identità di lancio: la decisione di cache avviene prima del dispatch,
  quindi nessun esito del figlio vecchio può essere presentato come quello della
  configurazione aggiornata. I nuovi test qualificano quest'ordine; nessuna
  modifica al protocollo di acknowledgement.
- **Nesso causale con il rifiuto del modello di usare search**: non stabilito, e
  non inferito. Questa chiusura non lo afferma.

### Prove

- `go test ./internal/providers/agents` e suite completa verdi;
  `scripts/quality_gate.sh --static` e `scripts/release_gate.sh` passano sulla
  candidata.
- Rilascio v0.1.64: [note di rilascio](../../docs/governance/releases/2026-11-14-v0.1.64.md).
