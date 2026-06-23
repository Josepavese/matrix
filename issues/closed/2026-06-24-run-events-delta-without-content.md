# Issue: `/v1/runs/{id}/events` emette delta agente senza contenuto testuale

Data: 2026-06-24  
Stato: closed  
Origine: integrazione Half Pocket Desk

## Contesto

Half Pocket Desk usa il composer web in `execution_mode=async`:

1. `POST /v1/runs`
2. polling su `events_url`
3. render dei delta/finale agente in UI

La UI deve mostrare risposte in corso e non restare ferma su uno stato generico
come "Sto leggendo Halfdesk e preparando la risposta".

## Problema osservato

Su MATRIX `0.1.20-snapshot`, build `2026-06-23T19:03:42Z`, gli eventi
`agent.message.delta` restituiti da `/v1/runs/{id}/events` possono contenere
solo metadati come `content_length`, ma nessun campo testuale (`message`,
`text`, `output`, `summary`).

Esempio reale:

```json
{
  "id": "evt-472100a6-3423-4cce-979b-9fbe2b542bb0",
  "kind": "agent.message.delta",
  "status": "streaming",
  "summary": null,
  "message": null,
  "text": null,
  "output": null,
  "metadata": {
    "content_length": 2,
    "source_update_type": "agent_message_chunk"
  }
}
```

Nel log runtime, nello stesso periodo, MATRIX vede invece i chunk testuali:

```json
{
  "msg": "session update received",
  "update_type": "agent_message_chunk",
  "text_len": 2,
  "text_preview": "OK"
}
```

Quindi il runtime osserva testo, ma l'API eventi non lo espone al client.

## Effetto su Halfdesk

Halfdesk puo' mostrare la risposta solo quando arriva `agent.message.final`.
Durante run piu' lunghi, Roberto vede solo uno stato generico e puo' pensare che
il sistema sia bloccato.

Questo e' peggiorato quando l'agente produce prima testo operativo e poi tool
call: il frontend non riceve i delta reali e non puo' aggiornare lo stato con
contenuto utile.

## Criteri di accettazione proposti

- Gli eventi `agent.message.delta` espongono il testo del chunk in un campo
  stabile, per esempio `message` o `text`.
- Il campo non deve contenere segreti o trace interni, solo il contenuto
  front-end safe gia' prodotto dall'agente.
- `agent.message.final` continua a contenere la risposta finale completa.
- La documentazione `/v1/runs/{id}/events` specifica il contratto:
  - `agent.message.delta.message`: chunk incrementale;
  - `agent.message.final.message`: risposta finale completa;
  - `run.completed`: terminale.
- Se MATRIX decide di non esporre i delta per policy, deve indicarlo in modo
  esplicito con un evento/stato, non con delta vuoti.

## Evidenza locale

Run diagnostico Halfdesk:

```text
run_id: run-36e45503-f605-4a56-adf1-d6efc52f4750
agent_id: codex
workspace_id: halfpocket
```

Il run ha poi emesso:

```json
{
  "kind": "agent.message.final",
  "status": "completed",
  "message": "OK_UI_REAL"
}
```

Il final funziona; il gap riguarda lo streaming delta.

## Risoluzione maintainer

Implementato in Matrix:

- gli eventi `agent.message.delta` generati da update
  `agent_message_chunk`/`user_message_chunk` valorizzano ora il campo stabile
  `message` con il chunk testuale;
- `agent.message.final.message` resta il canale della risposta finale completa;
- `/v1/runs/{id}/events` continua a esporre eventi ordinati e cursorizzati con
  `next_cursor`;
- la documentazione pubblica specifica il contratto:
  `agent.message.delta.message`, `agent.message.final.message`, e terminali
  `run.completed` / `run.failed` / `run.cancelled`.

Documentazione aggiornata:

- `docs/wiki/API-Reference.md`
- `docs/matrix_agent_communication_run_trace.md`

Verifica locale:

- `go test ./internal/logic/runnotifier ./internal/logic/agentlaunch ./internal/providers/runapi ./internal/providers/agents ./internal/logic/session ./internal/middleware`
- `go test ./...`
