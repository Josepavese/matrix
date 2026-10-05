# MiMo ACP: prova di accettazione eseguita — due worktree su mount diversi e sessione riusata

> **Nota di un agente esterno Half Pocket, 2 ottobre 2026.** Questo file è
> l'unica scrittura nel repository Matrix; nessun codice, configurazione,
> servizio, commit o push Matrix è stato modificato. Il file resta **non
> committato**, come da canale di segnalazione concordato.

Chiusura del punto lasciato aperto in
`issues/closed/2026-10-01-external-halfpocket-mimo-worktree-cwd-preflight.md`
(«NON fatto: la prova richiesta con due worktree contemporanei su mount
diversi (`/media/jose/Data`) e sessione successiva riusata, con agente
reale»). **La prova è stata eseguita oggi ed è riuscita.**

## Osservazione e ambiente

- Matrix 0.1.46 (`f0b97ab35e761cad6721a1ab985d3b51005f3e8e`), daemon
  `matrix.service` (unità utente) su postazione Ubuntu.
- Agente reale `mimo` (`/home/jose/.mimocode/bin/mimo acp`), protocollo ACP,
  `remote_session_id` MiMo `ses_...` su ogni run.
- Worktree: `/home/jose/halfpocket/.worktrees/mimo-s3` (mount SSD) e
  `/media/jose/Data/halfpocket-mvp-mail-provider-coverage-20261001`
  (mount Data) — lo stesso che il 1 ottobre terminava in preflight
  `phase=session/new`.
- Sottomissioni su `POST /v1/runs` con `execution_mode=async`,
  `workspace_path` per run, canale distinto per workspace; esiti letti con
  `matrix run wait`.

## Prova (esiti terminali reali)

| Run | Workspace | Esito |
| --- | --- | --- |
| `run-56458090-6648-49a1-acdd-769c1d14ab5c` | `/media/jose/Data/halfpocket-mvp-mail-provider-coverage-20261001` | **completed** |
| `run-0d1416b3-75f3-494c-9456-d7fcacd43888` | `/home/jose/halfpocket/.worktrees/mimo-s3` | **completed** |
| `run-b1b5421c-a6b0-4392-ab5c-a14a1e8621e7` | Data, secondo run, **sessione riusata** | **completed** |
| `run-d5be44ab-a1f7-4671-8305-ebef11e9e9c8` | SSD, secondo run, **sessione riusata** | **completed** |

- I primi due run sono stati sottomessi **contemporaneamente** (stesso
  istante): due worktree attivi su mount diversi, agente reale, nessun
  `Internal error (map[])` ne' `directory_not_allowed`.
- Sessione successiva riusata: i log (`matrix logs tail`) mostrano lo stesso
  `logical_session` fra primo e secondo run per ogni canale —
  `367cc57e-aaf7-45e5-9aac-837b1f8f54d8` (SSD) ed
  `eae95e1f-c371-4baa-bc74-ed4fe38f21c4` (Data) — con sessioni distinte per
  workspace.
- Collaterale utile: con un **canale condiviso** fra i due workspace, il
  routing ha rifiutato in modo esplicito con
  `workspace_identity_mismatch: workspace halfpocket-mimo-s3 has root ...,
  request asked for ...` — l'isolamento per sessione/workspace funziona come
  dichiarato.

## Decisioni prese lato Half Pocket (già applicate, le riportiamo perché restino traccia)

1. **Rimosso l'intero pin, non solo `env -C`.** La registrazione dell'agente
   `mimo` è ora `command=/home/jose/.mimocode/bin/mimo`, `args=[acp]`.
   Abbiamo tolto anche `--cwd /home/jose/halfpocket`: `declaredWorkspaceDir`
   lo legge come rivendicazione di workspace (`workspaceDirFlags` include
   `--cwd`), quindi da solo avrebbe continuato a far rifiutare ogni run fuori
   dal checkout principale. Verificato che il default di `mimo acp --cwd` è
   la cwd di processo: con la vostra risoluzione strutturale i tre termini
   dell'accordo concordano senza override. Se preferite che gli operatori
   lascino `--cwd` come dichiarazione esplicita, ditelo: è una riga di
   configurazione.
2. **Un canale per workspace.** Il router di sessione lega canale → sessione
   → workspace: per run concorrenti su workspace diversi servono canali
   distinti (v. `workspace_identity_mismatch` sopra). Con il medesimo canale e
   workspace, la sessione viene riusata come atteso.
3. **Reload del daemon necessario dopo `matrix agent set-binary`**: il daemon
   in esecuzione continuava a servire la vecchia registrazione argv (i primi
   submit sono stati rifiutati con il vostro errore esplicito che nomina i tre
   path — bella sorpresa: il check fa esattamente il suo lavoro). Se il
   caricamento a caldo fosse nelle vostre intenzioni, non è ancora avvenuto.

## Impatto e limiti

- Prompi minimali («Rispondi solo: OK»): la prova copre avvio sessione,
  prompt e completamento dei run, **non** un turno di lavoro con strumenti né
  risultati del modello. Il vecchio difetto (`session/new` →
  `Internal error (map[])` / `directory_not_allowed`) non si riproduce più in
  nessuno dei quattro run.
- Lezione operativa per la vostra documentazione: con `execution_mode`
  sincrono predefinito, la POST resta aperta per l'intera run; un client con
  timeout breve (30 s) cancella il contesto e il turno muore in
  `phase=session/prompt` con `ACP prompt failed: context canceled` (osservato,
  run non conteggiato nella tabella). Per orchestrazioni esterne consigliate
  `execution_mode=async` + `matrix run wait`, come usato qui.

## Richiesta ai manutentori

Nessuna azione obbligatoria. Due spunte facoltative: (a) chiudere la voce
«NON fatto» della nota del 1 ottobre con questa prova; (b) se utile,
aggiornare `issues/usage-feedback-halfpocket/ep-02-mimo-cwd-via-env-wrapper.md`
registrando che il workaround `env -C` + `--cwd` è stato ritirato il 2 ottobre
2026 grazie alla risoluzione strutturale della cwd.

Riferimento interno Half Pocket (stessa prova, dettagli e verbali):
`/home/jose/halfpocket/docs/cloud-transition/evidence/2026-10-02-mimo-wrapper-cwd-rimosso.md`
e `.../2026-10-02-verifica-suite-completa.md`.
