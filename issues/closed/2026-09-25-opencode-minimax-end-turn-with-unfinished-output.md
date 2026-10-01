# OpenCode/MiniMax: `run.completed` con output non concluso

Nota di un agente esterno al repository Matrix, 25 settembre 2026. Questo file
e' l'unica modifica qui effettuata: nessun codice, configurazione, installazione,
commit o push di Matrix e' stato toccato.

## Ambiente e revisione

- Matrix installato: 0.1.46, commit `f0b97ab35e761cad6721a1ab985d3b51005f3e8e`.
- OpenCode: 1.18.31, ACP, modello richiesto ed attestato
  `minimax-coding-plan/MiniMax-M3` (`provider_confirmed`).
- Ingresso: `POST /v1/runs`, `execution_mode=async`, nuova sessione effimera,
  output osservato via trace/eventi Matrix. Worktree isolato Half Pocket.

## Osservazione ripetuta

| Run ID | Stato/stop Matrix | Lunghezza `agent.message.final` | Fine del testo |
| --- | --- | ---: | --- |
| `run-3e5f292a-e17e-4fa8-8a51-8ee14de9ac6e` | `completed` / `end_turn` | 133051 caratteri | «Vediamo cosa restituisce:» |
| `run-7beca573-17c0-407c-8ef2-e4214fc92436` | `completed` / `end_turn` | 9796 caratteri | «Let me check if there's state leakage:» |

In entrambi i casi l'evento `agent.message.final` contiene il ragionamento
operativo accumulato, non una risposta finale al compito. Il secondo run era
stato chiesto esplicitamente di consegnare un finale entro 1200 caratteri con
commit/test/limiti. Il test indipendente
`TMPDIR=/dev/shm php apps/catalog/tests/maintenance.php` rimane rosso
(`check_20`), con modifica di debug nel worktree. Nessuno dei due run e' stato
accettato come consegna, malgrado `run.completed`.

## Cosa serve verificare

Determinare dal flusso ACP grezzo se OpenCode/MiniMax invia davvero
`end_turn` e chiude a meta' ragionamento, oppure se Matrix termina/accorpa
prematuramente i delta. Nel primo caso, distinguere nel trace una conclusione
del provider senza risposta finale utile, se il protocollo offre un segnale;
nel secondo, correggere la classificazione/finalizzazione senza euristiche
testuali. Una prova riproducibile dovrebbe registrare i messaggi/fasi ACP e
confrontarli con `agent.message.final`, `run.completed` e `outcome.summary`.

Decisioni gia' prese: non introdurre fallback di modello, non considerare
`run.completed` sufficiente per accettare il codice, non modificare Matrix dal
repository consumer. La causa (provider, adapter OpenCode o Matrix) resta
aperta; questa nota non la attribuisce senza prova.
