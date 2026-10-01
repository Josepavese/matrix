# Nota esterna: workspace ID, path e isolamento sessione

Nota di un agente esterno Half Pocket, 29 settembre 2026. Nessun codice,
commit, installazione o configurazione provider globale Matrix modificato.
Questa e' una segnalazione di contratto/UX, non l'affermazione di un bug gia'
attribuito a Matrix. L'orchestratore ha inizialmente riusato un workspace ID
del fronte Core con un path diverso; questa omissione e' parte della causa.

Ambiente: Matrix installato 0.1.46, build commit
`f0b97ab35e761cad6721a1ab985d3b51005f3e8e`; sorgente locale esaminato in sola
lettura a `2864953dad43cb9e12892b0cc3bd95377f7e0f1f`, non assunto identico
alla build. Entrypoint nativo `/v1/runs`, agente OpenCode.

## Osservato e prova

- Nuovo canale e path `/home/jose/halfpocket/.worktrees/minimax-provider-lifecycle-20260929`,
  ma workspace ID riusato `halfpocket-mimo-cwd-audit` del fronte Core.
- `run-842dd975-0b92-4a42-bc33-08ccc2181e9b` riusa remote session
  `ses_f18dd32c6ffexjHs000DoHBIba`, quella del Core: il solo canale non isola.
- Cancel nativo `POST /v1/runs/{id}/actions`, `action=cancel`, restituisce
  cancelled/accepted ma avverte: `provider cancellation signal failed: no
  reusable cached agent client owns workspace session: agent=opencode
  workspace=/home/jose/halfpocket/.worktrees/minimax-provider-lifecycle-20260929`.
  L'ack non dimostra che il provider non abbia eseguito o si sia fermato.
  Il worktree era pulito al controllo; non e' prova di zero esecuzione.
- Correzione tramite API/CLI native, senza patch: `workspace add` con ID/path
  esclusivi e `workspace switch --channel` prima del nuovo run.
  `run-c3fd8bd7-93a4-4685-84ba-e039e8082785` usa sessione remota distinta
  `ses_f14ec7179ffeqoH24YFM5l4OnQ`; follow-up `run-67fcd843` mantiene questa
  sessione. Nessun intervento sulla sessione Core.

## Decisione operativa gia' presa

Ogni worktree indipendente deve avere ID workspace Matrix registrato e path
corrispondente; nuovo canale non e' sufficiente. Verificare SID remoto prima
di attribuire isolamento. Riutilizzare serialmente quel SID per follow-up
contestuali. Le notifiche terminali native funzionano; non occorre un bridge
autonomo o polling. I modelli richiesti restano unverified senza attestazione
provider; nessuna inferenza di fallback da questo fatto.

## Richiesta ai manutentori e decisioni aperte

Chiarire la precedenza e il contratto fra workspace_id e workspace_path.
Valutare un rifiuto tipizzato del mismatch rispetto al workspace registrato
prima di riusare una sessione/client, anziche' permettere un'associazione
ambigua. Restituire il path canonico risolto e un motivo di riuso sessione
nei metadata, per rendere verificabile l'affinita' senza transcript.

La cancellazione puo' risultare terminale mentre il segnale provider fallisce:
documentare separatamente stato orchestrazione ed esito di cancellazione
remota. Non e' provato qui che il provider sia rimasto attivo. Non adottare
reset globali o cleanup automatico delle sessioni per aggirare il caso.

## Ricorrenza osservata il 1 ottobre 2026

Nota aggiunta dallo stesso contesto di agente esterno. Matrix installato resta
`0.1.46` (`f0b97ab35e761cad6721a1ab985d3b51005f3e8e`). Due probe OpenCode
di sola risposta, su canali nuovi e distinti
`model-probe-minimax31-20261001` e `model-probe-glm53-20261001`, hanno passato
`workspace_path=/home/jose/halfpocket` ma **nessun `workspace_id`**. I run
`run-0ef9e01b-9abd-4c8d-9978-d0acc27ae64d` e
`run-35653fde-8c30-4e7f-8820-ce575af7888d` risultano entrambi nel trace
con `workspace_id=halfpocket-mvp-mail-inventory-minimax31-20261001`,
`logical_session_id=80a62f37-3aba-4ba5-be52-274f6cc3bffb` e
`remote_session_id=ses_f0b97d0deffe8rh9ULfrCb4gwV`. Il primo modello
MiniMax M3.1 e' stato attestato/completato; il secondo GLM 5.3 e' stato
attestato ma ha fallito per credito insufficiente. Non e' stato fatto cleanup
del SID condiviso, perche' il trace lo lega a un workstream preesistente.

Questo caso non contiene un ID sbagliato fornito dal chiamante: il solo path
ha risolto a una affinity gia' registrata e i canali non hanno isolato la
sessione. Serve rendere visibile prima del prompt il workspace ID/path
canonico risolto e il SID che sara' riusato, oppure consentire un'opzione
esplicita di nuova sessione isolata per probe senza contaminare il workstream.
Nessun codice, installazione o commit Matrix e' stato toccato; solo questa
nota non tracciata in `issues/` e' stata aggiornata.
