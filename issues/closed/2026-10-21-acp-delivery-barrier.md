# ACP: finale in coda perso alla terminazione del prompt

## Decisione

**Accepted — implementato.** Emerso dalla presa in carico di
[OpenCode/VICKY](2026-10-21-external-vicky-opencode-final-not-delivered.md).
La causa storica di quell'episodio resta distinta da questi difetti Matrix
riproducibili; la relativa qualificazione esterna rimane aperta.

## Difetti e correzioni

- Un callback bloccato oltre i due drain da 250 ms consentiva a `Prompt` di
  restituire successo e rimuovere l'osservatore con il finale ancora in coda.
  Ora la consegna attende il backlog reale, il contesto o la chiusura del client.
- Oltre 256 aggiornamenti pendenti venivano scartati silenziosamente. Ora il
  collegamento fallisce esplicitamente: niente output parziale promosso a
  successo, nuove richieste sul client fallito rifiutate prima dell'invio.
- Gli aggiornamenti accodati erano assegnati agli osservatori al momento della
  consegna. Ora conservano l'identità della registrazione originale: una nuova
  run non eredita il finale di quella cancellata.
- Il turno ACP v2 dopo l'ack attendeva solo il contesto della richiesta. Ora
  osserva anche la durata del collegamento e conserva la causa del guasto.
  Il suo stop reason è letto dopo lo stato terminale, non prima del prompt.

Tutto nel confine SDK/adattatore, con primitive Go portabili. Nessun branch
provider/progetto, recupero da cronologia privata o fallback di modello.

## Prove

- Baseline f00f326: regressione osservatore trattenuto, risultato erroneamente
  riuscito dopo 0,50 s, prima della consegna del finale.
- Regressioni: callback lento, overflow, cancellazione e osservatore successivo,
  client già fallito, guasto v2 dopo ack e stop reason terminale.
- Guard di completamento conservato: reasoning, selezione modello ed `end_turn`
  senza messaggio/tool/output non sono evidenza di lavoro.
- Provider reale OpenCode 1.18.31, ACP stdio `acp --pure`, API DeepSeek diretta,
  `deepseek/deepseek-flash`, selezione `provider_confirmed`, nessun fallback.
  Test adattatore con lo schema/testo sintetico originale: 14 campi, 1.577 byte,
  SHA256 `b2dff32bdc5aa83d9533c62ff822b3a10c74af05d453f4161488c6432ee25aef`,
  `end_turn`, sessione `ses_ea288e40efferTBQTjbYcplBF0`.
- Wire probe indipendente: 14 campi; finale ricevuto prima di `end_turn`.
- Preflight completo su Go 1.27.1: governance, budget invariati, lint, suite race,
  build temporanea e GoReleaser. Il lint compilato su 1.27.1 non legge gli export
  di 1.27.2: il gate usa il toolchain dichiarato dal repo.
- Stato CI, tag pubblico e installazione:
  [verbale v0.1.55](../../docs/governance/releases/2026-10-21-v0.1.55.md).

## Follow-up sul runtime installato

Il primo collaudo completo v0.1.55, run
`run-dd8a0538-323f-4f72-988d-e543b08642c0`, ha prodotto l'errore esplicito
`provider_runtime_failed` / `acp notification delivery overflow`, dopo 190
aggiornamenti osservati. Non è una prova positiva e non identifica la causa
storica VICKY. La protezione rende visibile la saturazione precedentemente
silenziosa, ma occorre anche gestire il flusso reale di token.

La correzione successiva accorpa solo chunk testuali adiacenti con identità e
metadati identici. Limiti indipendenti su byte e aggiornamenti distinti; RPC
sempre leggibile, testo e whitespace conservati, nessuna fusione fra reasoning,
risposta, fasi o turni. Prova a 2.048 chunk e prova OpenCode reale con callback
rallentato di 300 ms: finale di 14 campi consegnato in 108,38 s.
Consegna e collaudo installato nel [verbale v0.1.56](../../docs/governance/releases/2026-10-21-v0.1.56.md).
