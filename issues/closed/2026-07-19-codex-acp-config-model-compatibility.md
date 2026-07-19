# Issue: il runtime `codex` accetta la run ma fallisce nel preflight ACP

Data: 2026-07-19

Stato: risolta e chiusa il 2026-07-19

Runtime MATRIX verificato: `0.1.21`

Commit runtime MATRIX: `cbc43d57ca7e4a82e45d516b9d59a52a220ec570`

Repository MATRIX osservato: `main` a
`1dd061504099cd62d52dc5ff78e552aa762b985f`

Provider ripristinato dopo la diagnosi: `@zed-industries/codex-acp` `0.13.0`

Codex CLI: `0.144.5`

Sistema verificato: Linux x86_64, servizio utente `matrix.service`

Priorita suggerita: alta per Half Pocket Desk e Funnels

## Sintesi

Una run inviata a `POST /v1/runs` con `agent_id=codex` viene risolta da MATRIX
verso il provider ACP corretto, ma fallisce prima dell'esecuzione del prompt.
La configurazione Codex corrente contiene `service_tier="default"`; il provider
ACP `0.13.0` accetta invece soltanto `fast` o `flex` e termina durante
`initialize`.

La diagnosi controllata ha inoltre mostrato che:

- `service_tier="flex"` supera il parsing locale, ma il provider remoto risponde
  `Unsupported service_tier: flex`;
- `service_tier="fast"` supera inizializzazione e parsing, ma la richiesta viene
  rifiutata perche il modello configurato `gpt-5.6-sol` richiede una versione
  piu recente di Codex/ACP;
- `matrix agent doctor codex` non espone preventivamente questa incompatibilita
  completa tra config, adapter e modello.

La configurazione runtime e il provider sono stati ripristinati allo stato
precedente. Questa issue non include modifiche al codice o alla configurazione
MATRIX.

## Evidenza

Risoluzione iniziale errata del consumer Funnels, gia corretta lato consumer:

```text
requested_agent: codex-acp
AGENT_NOT_FOUND: agent codex-acp not found in registry
```

Con l'identificatore corretto:

```text
requested_agent: codex
protocol_kind: acp
transport: stdio
phase: initialize
Error loading config: unknown variant `default`, expected `fast` or `flex`
```

Probe diagnostici successivi:

```text
service_tier=flex
Unsupported service_tier: flex

service_tier=fast
The 'gpt-5.6-sol' model requires a newer version of Codex.
Please upgrade to the latest app or CLI and try again.
```

La run Funnels viene quindi chiusa come `retryable_failed`; nessun brief o
risultato strutturato viene proiettato nel database online.

## Impatto

- Half Pocket Desk puo accettare una richiesta destinata a `codex`, ma la run
  fallisce solo dopo il dispatch.
- Il worker Funnels reclama correttamente il work order e il lease, poi consuma
  un tentativo senza poter produrre la revisione richiesta.
- Il problema appare come errore generico `client context cancelled` al
  consumer, mentre la causa utile e presente soltanto nei log MATRIX.

## Comportamento atteso

MATRIX deve poter verificare prima del dispatch che l'agente `codex`, il
provider ACP, la configurazione Codex e il modello selezionato siano
compatibili. Se non lo sono, `matrix agent doctor codex` e la risposta della
run devono esporre una causa stabile e azionabile senza ridurla a
`client context cancelled`.

L'identificatore pubblico dell'agente resta `codex`; il nome del pacchetto o
del binario ACP e un dettaglio dell'adapter e non deve diventare l'agent ID dei
consumer.

## Criteri di accettazione

- Il team MATRIX sceglie e distribuisce una combinazione supportata di Codex
  CLI e provider ACP per il modello configurato.
- `matrix agent doctor codex` rileva almeno config non parsabile, adapter
  incompatibile e modello non supportato prima di una run reale.
- Una run minima `agent_id=codex` completa `initialize`, `session/new` e
  `session/prompt` sul workspace autorizzato.
- Gli errori ACP/provider conservano nella risposta un codice specifico e il
  messaggio utile, senza essere mascherati dal solo `client context cancelled`.
- Desk e Funnels possono usare esclusivamente `agent_id=codex` senza override
  locali divergenti per postazione.

## Nota diagnostica secondaria

Con `matrix.service` attivo, `matrix agent list` ha restituito:

```text
ERR_VAULT_OPEN: Failed to open bbolt database: timeout
```

Il team puo valutare separatamente se i comandi diagnostici debbano passare
dal daemon o usare un accesso read-only compatibile con il lock del Vault.

## Risoluzione maintainer

- Decisione: closed il 2026-07-19 come risolta e superseded.
- Motivo: la riproduzione dipendeva dal provider ritirato
  `@zed-industries/codex-acp@0.13.0` e da Matrix `v0.1.21`. La policy
  ZERO-LEGACY rifiuta oggi quel provider; l'identificatore pubblico resta
  esclusivamente `codex`.
- Scope implementato: installer canonico nel PAL, percorsi assoluti Node e
  provider, handshake bounded in `matrix agent doctor`, propagazione
  sanitizzata degli errori provider, broker Vault autenticato e ownership
  atomica durante l'avvio del daemon.
- Evidenza runtime corrente: Matrix `v0.1.25`, Codex CLI `0.144.5`, provider
  `@agentclientprotocol/codex-acp@1.1.4`, configurazione effettiva con
  `service_tier="default"` e modello `gpt-5.6-sol`.
- Evidenza reale: `matrix agent doctor codex` completa l'handshake e riporta
  `ready_on_demand`; la run `run-954fbbe5-46dd-45e3-b091-7421fba1b600`
  ha completato con output `ok` e 12 eventi ordinati fino a `run.completed`,
  includendo `session.created` e `agent.message.final`.
- Evidenza Vault: daemon attivo, PID stabile, `NRestarts=0`; i comandi CLI usano
  il broker e non riproducono `ERR_VAULT_OPEN`.
- Issue precedenti che contengono l'implementazione dettagliata:
  `issues/closed/2026-07-15-codex-acp-provider-bootstrap-follow-up.md` e
  `issues/closed/2026-07-15-vault-readonly-cli-timeout-with-running-daemon.md`.
