# MiMo ACP: cwd fissa impedisce workspace Git isolati su Data

**Decisione Matrix (2026-10-01): chiusa nel perimetro di test.**
La cwd di processo e' risolta per run/workspace, senza override globale: la catena
workspace del run arriva fino al processo figlio e la copre
`TestRunWorkspaceReachesTheAgentChildCwd` (commit `39ae9cb`), che parte dalla
richiesta del run, passa dal session manager e router reali e legge la cwd che il
figlio dichiara di avere. Il workspace e' validato e autorizzato prima del fork
(`ResolveIdentity`, grant su path canonico). Child cwd, `--cwd` e workspace ACP
devono concordare: `declaredWorkspaceDir`, `verifyChildWorkspace` e
`verifyWorkspaceAgreement` verificano l'accordo, e il rifiuto e' esplicito invece di
`Internal error (map[])` (che nasceva dal rendering di `pkg/zedacp/jsonrpc.go`, ora
per contenuto). Nessun nome di agente nel percorso: l'allowlist del launcher shell e'
rimossa e la decisione viene dall'esistenza osservabile dello script di init.

NON fatto: la prova richiesta con due worktree contemporanei su mount diversi
(`/media/jose/Data`) e sessione successiva riusata, con agente reale. I test partono
dal workspace del run e arrivano alla cwd del figlio via peer di prova, non con MiMo
sul binario. La configurazione documentata in `issues/usage-feedback-halfpocket/`
(e l'`-C` sul wrapper `env`) non e' stata toccata: e' roba dell'operatore.

> Nota di un agente esterno Half Pocket, 1 ottobre 2026. Questo file è l'unica scrittura nel repository Matrix; nessun codice, configurazione, servizio, commit o push Matrix è stato modificato.

## Osservazione e ambiente

Matrix 0.1.46 (`f0b97ab35e761cad6721a1ab985d3b51005f3e8e`) su Linux. Due run asincroni MiMo 2.6 Pro, `run-e081f3b0-0c54-4873-bf38-5033a12ed152` e `run-e1404089-cc38-408b-8c86-d1f7f99bd0cc`, sono terminati in preflight `phase=session/new` con `RPC error -32603: Internal error (map[])`, senza sessione e senza codice prodotto. Il secondo chiedeva `workspace_path=/media/jose/Data/halfpocket-mvp-mail-provider-coverage-20261001`, worktree Git valido e pulito.

`matrix agent show mimo` risolve attualmente il child come `/usr/bin/env -C /home/jose/halfpocket /home/jose/.mimocode/bin/mimo acp --cwd /home/jose/halfpocket`. La cwd del child e il parametro `--cwd` sono quindi fissati alla checkout principale, mentre il workspace del run è fuori da quel sottoalbero. Questa configurazione è il workaround documentato in `issues/usage-feedback-halfpocket/ep-02-mimo-cwd-via-env-wrapper.md`; non si assume che sia l'unica causa senza log MiMo più dettagliati, ma è una spiegazione concreta compatibile con il rifiuto `directory_not_allowed` già osservato in EP-02.

## Impatto

L'orchestrazione Matrix di MiMo non può usare worktree isolati sulla partizione Data: il modello fallisce prima di iniziare. Il lavoro Mail è stato riassegnato **esplicitamente** a MiniMax, senza fallback silenzioso. Nessun dato o servizio di produzione è stato toccato da questi run.

## Richiesta ai manutentori

Separare e risolvere la cwd di processo per *run/workspace*, senza override globale che inchiodi MiMo a un unico checkout. Il workspace richiesto deve essere validato come reale/autorizzato prima del fork; child cwd, `--cwd` MiMo e workspace ACP devono concordare. Se l'endpoint non lo supporta, restituire un errore diagnostico esplicito con i tre path, non `Internal error (map[])`. Testare almeno due worktree contemporanei su mount diversi e la sessione successiva riusata.

Decisione già presa lato Half Pocket: non cambiare da questa task la configurazione o il codice Matrix, non collocare i worktree su root SSD solo per aggirare il problema. Serve una soluzione strutturale dei manutentori; la riassegnazione MiniMax è temporanea e dichiarata.

---

**Aggiornamento Matrix (2026-10-02): la prova dichiarata non fatta è stata eseguita ed è riuscita.**
Il punto lasciato aperto sopra — *"la prova richiesta con due worktree contemporanei
su mount diversi (`/media/jose/Data`) e sessione successiva riusata, con agente
reale"* — è stato chiuso lato Half Pocket il 2 ottobre, su Matrix **0.1.46** con
agente reale `mimo`. Quattro run `completed`: due worktree attivi
**contemporaneamente** su mount diversi, più un secondo run per canale con
**sessione riusata** (`logical_session` identico fra primo e secondo run per
ogni canale). Nessun `Internal error (map[])`, nessun `directory_not_allowed`.
Collaterale: con un canale condiviso fra due workspace il routing ha rifiutato
esplicitamente con `workspace_identity_mismatch`, quindi l'isolamento per
sessione/workspace si comporta come dichiarato.

L'operatore ha inoltre **ritirato l'intero pin** `env -C` + `--cwd`, non solo la
parte `env -C`: con la risoluzione strutturale della cwd i tre termini
dell'accordo concordano senza override. Evidenza in
`issues/2026-10-02-external-halfpocket-mimo-worktree-cwd-acceptance.md`
(non committata, canale di segnalazione) e in
`~/halfpocket/docs/cloud-transition/evidence/2026-10-02-mimo-wrapper-cwd-rimosso.md`.

Limite della prova, dichiarato da chi l'ha eseguita: prompi minimali — copre
avvio sessione, prompt e completamento, **non** un turno di lavoro con strumenti.


## Stato aggiornato — 2026-10-08

L'accettazione reale del 2 ottobre è ora archiviata in
[questa scheda](2026-10-02-external-halfpocket-mimo-worktree-cwd-acceptance.md).
Il precedente «NON fatto» è storico. Prova reale aggiuntiva su MiMo: cwd temporanea,
resume e lettura di token casuale; dettagli nel riepilogo del 2026-10-08.
