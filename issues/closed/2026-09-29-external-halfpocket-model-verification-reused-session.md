# Nota esterna: attestazione modello nei run su sessioni riusate

**Decisione Matrix (2026-10-01): chiusa con limiti dichiarati.**
La verifica del modello nasce da evidenza di protocollo e origine di sessione
(`internal/providers/agents/acp_model_selection.go`), esposta come `ModelSelection`
in `internal/middleware/agent.go` con verdetti e motivi strutturati. Sul retry per
sessione persa il modello richiesto veniva perso: `retryTurnWithFreshSession` ora
copia il turno del chiamante azzerando solo `RemoteSessionID`, cosi' il contratto
segue il retry per costruzione e la ricevuta arriva per ogni tentativo (commit
`3e5d7d0`; verificato campo per campo: 17 campi, 1 mutato). Lo stesso commit chiude
la latenza di `StrictSession`, che persa avrebbe reso permissivo un vincolo che deve
fallire chiuso.

NON fatto: la riattestazione su sessione riutilizzata "calda" non e' possibile -
`session/set_model` (v2) restituisce solo `_meta` e la sessione calda non espone un
metodo di stato in sola lettura. Il verdetto onesto resta `unverified` con motivo
strutturato (`provider_does_not_attest`), mai un attestato inventato. E nessuna
riproduzione end-to-end con agente reale.

29 settembre 2026. Nota di un agente esterno del progetto Half Pocket.
Solo questo file non tracciato viene aggiunto; nessun codice, configurazione,
installazione o commit Matrix e' stato modificato.

## Ambiente e prova

Matrix installato `0.1.46`, revisione dichiarata dal binario
`f0b97ab35e761cad6721a1ab985d3b51005f3e8e`, API loopback `:9091`.
Fonte: export nativo `GET /v1/runs/{id}/trace`, letto dopo evento terminale
Unix SSE, senza polling o transcript intermedi.

Run concluso `run-122040ba-d4e6-41f9-b56a-1c96d5c5e809`, actor OpenCode,
sessione remota riusata `ses_f168fc1a4ffeR1WPgrdgaDJQ7Q`:

- schema `matrix.agent_communication_run_trace.v0`;
- outcome `completed`;
- `requested_model = deepseek/deepseek-flash`;
- `effective_model` assente, `configured_model` assente;
- `model_verification = unverified`;
- `protocol`, `agent_id`, ID run e ID sessione presenti.

Anche lo snapshot iniziale del run MiMo
`run-fd6d669b-e2b7-4e55-a1eb-5e683764c337`, stessa sessione remota del filone
precedente, dichiara modello richiesto e `unverified`. Essendo ancora in
esecuzione, NON e' una prova del suo risultato finale.

## Impatto e distinzione

L'evento terminale e il recapito all'orchestratore funzionano. Questa nota
non li classifica come guasto, non deduce un fallback, non afferma quale
modello sia stato effettivamente eseguito e non attribuisce una causa certa.
Manca soltanto la prova nativa del modello per qualificare il run: il PM non
puo' elevare il selettore richiesto a `provider_confirmed`.

Half Pocket ha inoltre corretto un proprio consumer che confondeva `Run`
con `TraceRun`: nell'export il canale e' `surface.channel`, non
`run.channel_id`. Questo era un errore del consumer, NON di Matrix.

## Richiesta ai manutentori

Verificare se una sessione ACP gia' aperta e riusata dovrebbe conservare o
riattestare il modello effettivo nello specifico run. Se non e' dimostrabile,
conservare `unverified`, ma esporre un motivo distinguibile: provider non
attestante, modello non selezionabile, verifica non ripetuta, o dato perduto.
Non introdurre conferme ricavate soltanto da `requested_model`.

Proposta di test: nuova sessione -> secondo run contestuale con stesso
modello -> richiesta diversa -> provider che non attesta. Distinguere stato
richiesto, selezionato e confermato; verificare traccia e evento dopo replay.

## Decisioni gia' prese / punti aperti

- Sessioni contestuali riusate serialmente per preservare contesto e token;
  non si aprono sessioni nuove per ottenere artificialmente un verde.
- I negativi dell'Hub restano fail-closed finche' manca l'attestazione.
- Nessuna installazione o patch Matrix da questo progetto.
- Causa e comportamento atteso su riuso ACP restano da accertare a monte.
