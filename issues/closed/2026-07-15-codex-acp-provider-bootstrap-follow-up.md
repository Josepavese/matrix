# Issue: bootstrap provider Codex ACP obsoleto e causa reale nascosta

Data: 2026-07-15  
Versione MATRIX: `0.1.21`  
Commit MATRIX: `cbc43d57ca7e4a82e45d516b9d59a52a220ec570`  
Priorita suggerita: alta

## Sintesi

Una run asincrona indirizzata all'agente MATRIX `codex` arriva correttamente al
router, crea la sessione e poi fallisce durante `initialize`:

```text
[agent_preflight_failed] agent provider preflight failed agent=codex
protocol=acp phase=initialize: ACP initialize failed: client context cancelled
```

Il messaggio pubblico non contiene la causa. Eseguendo direttamente il provider
configurato da MATRIX emerge invece l'errore utile:

```text
Error: error loading config: /home/jose/.codex/config.toml:7:16:
unknown variant `default`, expected `fast` or `flex`
```

La configurazione valida per Codex CLI corrente contiene:

```toml
service_tier = "default"
```

Questa issue e' un follow-up di
`issues/2026-06-24-codex-acp-preflight-and-vault-timeout.md`: conferma il
problema il 2026-07-15 e aggiunge evidenza sul bootstrap/installazione del
provider.

## Evidenza provider

L'endpoint risolto dal registry MATRIX era:

```text
/home/jose/.nvm/versions/node/v22.12.0/bin/codex-acp
```

Il comando appartiene a:

```text
@zed-industries/codex-acp 0.13.0
```

Il pacchetto e' deprecato e sostituito da
`@agentclientprotocol/codex-acp`; al 2026-07-15 la versione pubblicata rilevata
e' `1.1.2`:

- <https://www.npmjs.com/package/@zed-industries/codex-acp>
- <https://www.npmjs.com/package/@agentclientprotocol/codex-acp>

Il servizio systemd MATRIX non eredita il path NVM e trova `/usr/bin/node`
18.19.1, mentre il wrapper npm vive sotto Node 22.12.0. Puntando MATRIX
direttamente al binario ELF incluso nel vecchio pacchetto si elimina questa
ambiguita di PATH, ma la run continua a fallire: il binario nativo stampa
l'errore `service_tier` riportato sopra. Quindi il problema osservato non e'
soltanto il PATH di systemd.

## Riproduzione Matrix-local

Prerequisiti osservati:

```text
matrix readiness: ready
agent_id: codex
protocol: acp
status: ready_on_demand
POST /v1/runs: disponibile
```

Inviare una run minima:

```bash
curl -sS http://127.0.0.1:9091/v1/runs \
  -H 'Content-Type: application/json' \
  -d '{
    "channel_id": "matrix.repro.codex-acp-initialize",
    "execution_mode": "async",
    "agent_id": "codex",
    "workspace_id": "matrix-repro",
    "workspace_path": "/tmp/matrix-repro",
    "input": {"text": "Rispondi soltanto: ok"}
  }'
```

Il trace osservato contiene:

```text
run.started
routing.decision
agent.prompt.sent
session.created
provider.preflight.failed
run.failed
```

La run termina in meno di un secondo con `client context cancelled`.

Il provider puo essere verificato senza eseguire una run:

```bash
codex-acp </dev/null
```

Con la configurazione descritta stampa immediatamente l'errore di parsing.

## Invariante MATRIX coinvolta

MATRIX non deve dichiarare un agente `ready_on_demand` quando il provider non
riesce a completare il proprio handshake minimo. Se un processo stdio termina
durante `initialize`, l'errore restituito deve conservare uno stderr breve e
sanitizzato quando disponibile.

Il bootstrap dell'agente deve inoltre usare un provider corrente e avviabile
nello stesso ambiente del daemon, senza dipendere implicitamente dal PATH della
shell interattiva.

## Comportamento atteso

Una delle seguenti superfici deve indicare la causa prima della run, oppure la
run deve restituirla in modo strutturato:

```json
{
  "code": "agent_provider_initialize_failed",
  "agent_id": "codex",
  "phase": "initialize",
  "provider_exit_code": 1,
  "provider_stderr": "error loading config: ~/.codex/config.toml: unknown variant default"
}
```

Nessun token, valore ambiente o contenuto completo della configurazione deve
essere incluso.

## Criteri di accettazione

- `matrix install`/registry usa il pacchetto Codex ACP canonico corrente oppure
  segnala esplicitamente che il provider installato e' deprecato.
- Il comando provider viene provato con lo stesso ambiente del servizio
  MATRIX, non soltanto con quello della shell interattiva.
- `matrix readiness` o `matrix agent doctor codex` esegue un handshake bounded
  e non riporta `ready_on_demand` se `initialize` fallisce.
- Un provider stdio che esce durante `initialize` con stderr produce un errore
  strutturato e sanitizzato, non soltanto `client context cancelled`.
- Un test MATRIX-owned copre un fake ACP che termina durante initialize e
  verifica propagazione di exit code e stderr.
- Un probe reale con il provider Codex ACP supportato completa una run minima
  con evento terminale `run.completed`.

## Stato della verifica esterna

Il bridge chiamante e' stato verificato separatamente: `POST /v1/runs`
restituisce `202`, il `run_id` viene creato e gli eventi vengono letti fino al
terminale. Il blocco residuo e' interamente successivo al routing MATRIX, nella
fase di inizializzazione del provider ACP.

## Risoluzione maintainer

- Decisione: accepted e implementata il 2026-07-15.
- Motivo: lo stato `ready_on_demand` senza handshake e la perdita dello stderr
  violavano la readiness operativa e rendevano opaca una causa provider
  azionabile.
- Scope: trasporto stdio ACP, classificazione errori provider, supervisor
  on-demand, `matrix agent doctor`, risoluzione installer Codex e documentazione
  runtime/compliance.
- Evidenza: test fake ACP verifica exit code `7`, stderr sanitizzato e
  `failure_reason=provider_process_exit`; test runtime impedisce
  `ready_on_demand` prima del probe; test registry risolve `matrix install codex`
  verso `codex-acp` e rifiuta `@zed-industries/codex-acp`.
- Evidenza reale: `@agentclientprotocol/codex-acp` `1.1.2`, eseguito con PATH
  equivalente al servizio e `/usr/bin/node` `18.19.1`, ha completato initialize
  ACP v1, `session/new`, prompt, permission callback, token file/terminale e
  stop reason `end_turn`.
- Evidenza runtime MATRIX: `matrix install codex` ha materializzato il pacchetto
  canonico nel PAL `MATRIX_HOME/agents/codex`, registrando percorsi assoluti per
  Node e `dist/index.js`. Con un PATH privo di NVM il daemon ha completato il
  probe come `ready_on_demand`; la run API
  `run-ba65b752-f597-4027-a848-6fa8dbf579ec` e' terminata con
  `run.completed` dopo 11 eventi.
