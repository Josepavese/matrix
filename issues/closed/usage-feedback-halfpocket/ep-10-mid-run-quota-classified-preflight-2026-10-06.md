# EP-10 — Quota durante prompt classificata come preflight

Nota di un agente esterno, Codex PM, 6 ottobre 2026. Nel repository Matrix
e' stato aggiunto soltanto questo file in issues/, senza commit. Nessun
codice, installazione, configurazione o servizio Matrix modificato.

## Ambiente e osservazione reale

- Binario installato: Matrix 0.1.51, commit
  `7854f5b15c61938c1d597368a5704ccc1527488d`, build 2 ottobre 2026.
- Run: `run-ac68fefd-78d6-400b-81e9-93b73c6aaa6f`, OpenCode/ACP stdio,
  modello richiesto ed effettivo `zai-coding-plan/glm-5.3`, provider_confirmed.
- Sessione: `ses_ef3461048ffejOnTevrhiGi6Mc`.
- Avvio: 6 ottobre 13:31:14 UTC. Ultimo tool.result/write_file:
  15:40:41 UTC. Terminale failed: 15:42:06 UTC, notifica nativa **3710**.

Il trace contiene lavoro effettivo prima del fallimento. L'evento finale
espone `failure_code=agent_preflight_failed`; un successivo evento e'
`provider.preflight.failed`, phase `session/prompt`. Dati RPC preservati:

```text
failure_reason: provider_rpc_error
rpc_error_code: -32603
rpc_error_data: {"errorName":"APIError","service":"session"}
rpc_error_message: Internal error: Usage limit reached for 5 hour.
Your limit will reset at 2026-10-07 02:32:19
```

Questa e' prova di quota dichiarata dal provider durante il prompt, NON di
HTTP 429, header Retry-After o timezone del reset. Non sono stati letti
transcript o segreti. Notifica nativa recuperata correttamente dal cursore
3709 dopo timeout dell'osservatore: non si segnala perdita del terminale.

## Problema e proposta

La classificazione pubblica preflight e' fuorviante dopo oltre due ore e
tool effettivi: un consumer puo' interpretarla come lavoro mai avviato,
ignorare WIP o avviare replay inappropriati. Distinguere errore prima
dell'esecuzione da errore del provider durante prompt, preservando fase e
RPC originali. Se ACP non espone quota/reset strutturati, classificare
provider_api_error/unknown e conservare la diagnostica; non inventare
RetryAfter o un orario UTC tramite euristiche sul testo.

Criterio di accettazione: test causale con tool.result seguito da APIError
quota produce terminale runtime failure, non false preflight; un vero
errore pre-avvio resta preflight. Evento unico minimale, trace mirato;
WIP/sessione e nessun auto-retry preservati. La prova live sopra resta
distinta dal test sintetico futuro.

## Decisioni gia' prese e aperte

PM: nessun retry GLM in quota; WIP di sei file preservato, riassegnazione
esplicita temporanea a MiMo Token Plan in nuova conversazione, stesso
worktree dopo terminale. Non cambiata installazione Matrix.

Aperte per i manutentori: tassonomia compatibile dei codici runtime,
eventuale supporto a disponibilita'/reset strutturati dai provider;
nessuna richiesta di scheduler autonomo o fallback modello silenzioso.

Collegamento: EP-07 e' una proposta storica priva di prove 429; questo
episodio aggiunge un caso RPC reale, senza riscriverne il verbale.


## Decisione del manutentore — 2026-10-08

Bug corretto: errore durante session/prompt è runtime. ID EP10-20261006 distinto dall’EP10 storico di delivery.

Scheda gestita e archiviata. [Decisione completa, motivi ed evidenze](../../../docs/governance/issue_resolution_2026-10-08.md).
Lo stato di pubblicazione e installazione è nel verbale della nuova release;
le evidenze del reporter sopra restano lo snapshot originale.
