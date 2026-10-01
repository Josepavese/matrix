# MiMo ACP: cwd fissa impedisce workspace Git isolati su Data

> Nota di un agente esterno Half Pocket, 1 ottobre 2026. Questo file è l'unica scrittura nel repository Matrix; nessun codice, configurazione, servizio, commit o push Matrix è stato modificato.

## Osservazione e ambiente

Matrix 0.1.46 (`f0b97ab35e761cad6721a1ab985d3b51005f3e8e`) su Linux. Due run asincroni MiMo 2.6 Pro, `run-e081f3b0-0c54-4873-bf38-5033a12ed152` e `run-e1404089-cc38-408b-8c86-d1f7f99bd0cc`, sono terminati in preflight `phase=session/new` con `RPC error -32603: Internal error (map[])`, senza sessione e senza codice prodotto. Il secondo chiedeva `workspace_path=/media/jose/Data/halfpocket-mvp-mail-provider-coverage-20261001`, worktree Git valido e pulito.

`matrix agent show mimo` risolve attualmente il child come `/usr/bin/env -C /home/jose/halfpocket /home/jose/.mimocode/bin/mimo acp --cwd /home/jose/halfpocket`. La cwd del child e il parametro `--cwd` sono quindi fissati alla checkout principale, mentre il workspace del run è fuori da quel sottoalbero. Questa configurazione è il workaround documentato in `issues/usage-feedback-halfpocket/ep-02-mimo-cwd-via-env-wrapper.md`; non si assume che sia l'unica causa senza log MiMo più dettagliati, ma è una spiegazione concreta compatibile con il rifiuto `directory_not_allowed` già osservato in EP-02.

## Impatto

L'orchestrazione Matrix di MiMo non può usare worktree isolati sulla partizione Data: il modello fallisce prima di iniziare. Il lavoro Mail è stato riassegnato **esplicitamente** a MiniMax, senza fallback silenzioso. Nessun dato o servizio di produzione è stato toccato da questi run.

## Richiesta ai manutentori

Separare e risolvere la cwd di processo per *run/workspace*, senza override globale che inchiodi MiMo a un unico checkout. Il workspace richiesto deve essere validato come reale/autorizzato prima del fork; child cwd, `--cwd` MiMo e workspace ACP devono concordare. Se l'endpoint non lo supporta, restituire un errore diagnostico esplicito con i tre path, non `Internal error (map[])`. Testare almeno due worktree contemporanei su mount diversi e la sessione successiva riusata.

Decisione già presa lato Half Pocket: non cambiare da questa task la configurazione o il codice Matrix, non collocare i worktree su root SSD solo per aggirare il problema. Serve una soluzione strutturale dei manutentori; la riassegnazione MiniMax è temporanea e dichiarata.
