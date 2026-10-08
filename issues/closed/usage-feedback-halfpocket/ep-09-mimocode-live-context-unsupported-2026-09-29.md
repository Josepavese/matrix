# EP-09 — Aggiornamento live del contesto MiMo non supportato

> Nota di un agente esterno (Codex PM Half Pocket), 29 settembre 2026.
> Solo questa segnalazione e il suo riferimento nell'indice, dentro `issues/`,
> sono aggiunti al repository Matrix. Nessun codice, installazione,
> configurazione o servizio Matrix modificato; nessun commit o push qui.

**Stato**: `certificato` per l'esito API/evento osservato;
`provider_limit` / interoperabilità da diagnosticare, non bug Matrix provato;
`suggestion` per la discovery preventiva.

## Ambiente e prova minima

- CLI verificata: Matrix 0.1.46, commit
  `f0b97ab35e761cad6721a1ab985d3b51005f3e8e`, build 2026-09-24T13:52:20Z.
- Sorgente consultato in sola lettura: HEAD
  `2864953dad43cb9e12892b0cc3bd95377f7e0f1f`.
- Run attivo: `run-8dac4869-05ba-4322-a5be-94ef44e2c591`, agente `mimo`,
  modello effettivo `xiaomi/mimo-v2.6-pro`, `provider_confirmed`.
- API nativa `POST /v1/runs/<id>/actions`, `action=attach_context`, capsule
  visibile al modello: risposta `accepted=true`, delivery
  `ctx-404c71b5-e644-4272-ab54-bb94d87cd96d`.
- Nel trace nativo, solo metadata di consegna letti: primo
  `run.context.attached` accepted/pending, poi stesso delivery ID
  `unsupported`, `delivery_class=unsupported`.

Non sono inclusi transcript, capsule, output provider, dati aziendali,
segreti o valori degli header. Il run rimane attivo: l'esito della consegna
non è un errore terminale del lavoro originario.

## Impatto e decisione già presa

Il PM non può presumere che MiMo abbia ricevuto la correzione in corso.
Non viene avviato un secondo turno concorrente né riavviato il servizio:
il punto va verificato alla consegna e, se manca, inviato con un seguito
nella stessa conversazione dopo il terminale. Non si costruisce un bridge
alternativo. Questa limitazione non blocca le notifiche terminali native.

## Evolutiva proposta — P2

Esporre nella discovery/doctor dell'agente la capacità effettiva di
`attach_context`, distinguendo supporto del trasporto, del provider e
consegna provata. `accepted` deve restare chiaramente distinto da delivered.

Accettazione futura: provider privo della capacità rifiutato esplicitamente
senza promessa di recapito; provider compatibile con prova positiva di
ricezione durante il run e negativo typed. Il terminale del run originale
deve continuare a essere recapitato indipendentemente dal live attach.

Resta aperta l'attribuzione precisa del limite a provider/adapter/runtime;
nessuna modifica a monte richiesta come prerequisito del MVP Half Pocket.


## Decisione del manutentore — 2026-10-08

Doctor e delivery separati; limite live-attach MiMo conservato come limite del provider, senza falsi riusi remoti.

Scheda gestita e archiviata. [Decisione completa, motivi ed evidenze](../../../docs/governance/issue_resolution_2026-10-08.md).
Lo stato di pubblicazione e installazione è nel verbale della nuova release;
le evidenze del reporter sopra restano lo snapshot originale.
