# Issue: Codex reasoning effort non selezionabile da MATRIX/Halfdesk

Data: 2026-06-24  
Stato: closed  
Origine: integrazione Half Pocket Desk

## Contesto

Half Pocket Desk usa MATRIX locale come ponte verso Codex per il composer
operativo PM.

Payload Halfdesk attuale verso `POST /v1/runs`:

- `agent_id`: `codex`
- `workspace_id`: `halfpocket`
- `workspace_path`: path locale del repo Half Pocket
- `execution_mode`: `async`
- `trace_policy`: inline senza metadati protocollo inutili

Sul PC Jose, `codex doctor` mostra:

```text
model: gpt-5.5
```

e `~/.codex/config.toml` contiene:

```toml
model_reasoning_effort = "xhigh"
```

Pero' Halfdesk non ha modo di sapere, selezionare o verificare quale reasoning
effort venga effettivamente usato da MATRIX quando lancia l'agente `codex`.

## Problema

Oggi il portale Halfdesk puo' scegliere l'agente e il workspace, ma non puo'
selezionare in modo esplicito il reasoning effort per Codex.

Esiste una possibile soluzione manuale tramite override argomenti agente:

```bash
matrix agent args append codex -- -c 'model_reasoning_effort="xhigh"'
```

ma non e' una superficie applicativa sicura per un client come Halfdesk:

- il valore non e' parte del contratto `/v1/runs`;
- il chiamante non puo' leggere dall'envelope se l'effort e' stato applicato;
- il trace non espone in modo standard il reasoning effort effettivo;
- non e' chiaro se ogni postazione erediti sempre lo stesso `$CODEX_HOME`;
- non si puo' differenziare effort per portale, utente, workflow o richiesta.

## Comportamento atteso

MATRIX dovrebbe esporre una configurazione governata per il reasoning effort
Codex, senza far dipendere i client da prompt o override manuali non osservabili.

Possibile contratto:

```json
{
  "agent_id": "codex",
  "agent_config": {
    "model_reasoning_effort": "xhigh"
  }
}
```

oppure:

```json
{
  "agent_id": "codex",
  "codex_config": {
    "model_reasoning_effort": "xhigh"
  }
}
```

Valori da valutare in base a Codex CLI:

- `xhigh`
- `high`
- `medium`
- `low`
- eventuali altri valori supportati dalla versione installata

## Criteri di accettazione proposti

- `/v1/runs` accetta un campo esplicito e validato per Codex reasoning effort.
- MATRIX traduce il valore in override Codex equivalente:
  `-c model_reasoning_effort="<valore>"`.
- Se il valore non e' supportato, MATRIX restituisce errore chiaro prima di
  avviare il run.
- Il trace del run espone evidenza non segreta del valore applicato, per esempio:

```json
{
  "agent_launch_policy": {
    "model_reasoning_effort": "xhigh"
  }
}
```

- In assenza di valore esplicito, MATRIX documenta se usa:
  - default Codex;
  - `$CODEX_HOME/config.toml`;
  - override `matrix agent args`;
  - altra policy.
- Il comportamento resta cross-platform su Linux, macOS e Windows.

## Nota operativa

Halfdesk puo' documentare il valore atteso, ma non deve inventare un campo
`reasoning_effort` e considerarlo efficace senza supporto MATRIX verificabile.

## Risoluzione maintainer

Implementato in Matrix:

- `POST /v1/runs` accetta `agent_config.model_reasoning_effort` e l'alias
  `codex_config.model_reasoning_effort`.
- I valori ammessi sono `low`, `medium`, `high`, `xhigh`.
- Il valore viene validato prima dell'avvio e tradotto in override Codex
  `-c model_reasoning_effort="<valore>"`.
- L'override e' consentito solo quando `agent_id` risolve a `codex`; valori
  non supportati, agent non Codex o config discordanti producono HTTP `400`.
- `routing.decision` espone l'evidenza non segreta in
  `protocol_meta.agent_launch_policy.model_reasoning_effort`.
- La cache dei client provider e' partizionata anche dagli argomenti per-run,
  cosi' client Codex con effort diversi non vengono riusati tra loro.

Documentazione aggiornata:

- `docs/wiki/API-Reference.md`
- `docs/wiki/Using-Agents.md`
- `docs/matrix_agent_communication_run_trace.md`
- `docs/matrix_protocol_neutral_runtime.md`

Verifica locale:

- `go test ./internal/logic/runnotifier ./internal/logic/agentlaunch ./internal/providers/runapi ./internal/providers/agents ./internal/logic/session ./internal/middleware`
- `go test ./...`
