# Registro e artefatti agenti configurabili per installazioni offline da GitHub

**Decisione Matrix (2026-09-24): accolta e chiusa.** `MATRIX_AGENT_REGISTRY_URL`
seleziona un indice HTTPS completo per `matrix install`, ricerca e informazioni
del registro; senza override resta l'indice ACP pubblico. Gli URL degli
artefatti in un indice HTTPS devono essere HTTPS. I binari senza SHA-256
pubblicato sono rifiutati per impostazione predefinita, e il digest pubblicato
deve corrispondere ai byte scaricati. La provenienza e la verifica sono
registrate nel Vault. I test coprono URL non validi, downgrade e installazione
dal mirror. Una prova con la CLI reale ha installato un artefatto da un mirror
HTTPS locale isolato: una richiesta all'indice, una all'artefatto, nessuna a
GitHub, SHA-256 verificato. L'integrazione dell'installer Nido e il suo banco
di staging appartengono al consumer e richiedono un collaudo separato lì.

Nota di un agente esterno (Half Pocket), 24 settembre 2026. Nessun codice,
configurazione, installazione o altro file del repository Matrix è stato
modificato; questa issue non è committata.

## Ambiente e revisione

- Matrix v0.1.44; sorgente Matrix alla revisione `4ec504c`.
- Banco Nido Ubuntu 26.04 isolato, con GitHub non raggiungibile e un mirror
  HTTPS di staging per gli artefatti della piattaforma. Nessun servizio o dato
  business è stato migrato.
- Hub installato tramite il one-shot Half Pocket. I prerequisiti Matrix e hubd
  scaricati dal mirror, verificati con SHA-256 e riapplicati senza modifiche.

## Osservazione e prova

`matrix install opencode`, invocato durante l'attivazione Hub, ottiene l'indice
ACP pubblico e segue l'URL GitHub dell'artefatto. Con GitHub bloccato,
l'attivazione termina in `hub_agent_install_failed`; non si arriva al collaudo
di upgrade Hub né alle API autenticate. La disponibilità del binario Matrix
nel mirror non basta a installare l'agente.

Nel sorgente `internal/logic/agentmgr/registry_client.go:97` il default è
`https://cdn.agentclientprotocol.com/registry/v1/latest/registry.json`.
`cmd/matrix/app.go:180,220` passa sempre la stringa vuota al costruttore del
client: l'API interna accetta un URL alternativo, ma la CLI non lo espone.
`MATRIX_AGENTS_CONFIG` riguarda la mappa dei comandi locali, non l'indice di
distribuzione. Non è stato trovato un flag/env/config nativo per sostituire
l'indice o la base degli artefatti preservando `matrix install`.

## Richiesta al proprietario Matrix

Esporre una configurazione esplicita e amministrata della sorgente dell'indice
di installazione agenti, così che `matrix install` possa leggere un indice
affidabile con URL HTTPS interni/mirror e digest degli artefatti. Conservare i
controlli attuali: digest obbligatorio/fail-closed per i binari, contenimento
del comando, tracciamento della provenienza e verifica dell'artefatto. Il
default deve restare il registro ACP pubblico quando non configurato. Servono
test per default, override valido, URL/digest errato e installazione senza
accesso a GitHub ma con mirror disponibile.

## Decisioni già considerate e motivazione

- **Non modificare Matrix dal consumer.** La proprietà del runtime è del team
  Matrix; patch o fork applicati dall'installer Half Pocket creerebbero una
  dipendenza opaca e non aggiornabile.
- **Non usare `matrix agent set-binary` come surrogato automatico.** Registra
  un eseguibile già presente e cambia la semantica dell'installazione via
  registro/digest richiesta dal contratto Hub; si perderebbe la provenienza
  verificata gestita da `matrix install`.
- **Non rendere GitHub un prerequisito implicito.** La distribuzione
  applicativa è prevista su un dominio software controllato; il banco con
  GitHub bloccato è un requisito di accettazione intenzionale.
- **Non disattivare la verifica SHA.** Il mirror deve offrire un indice e
  artefatti verificabili, non un bypass del controllo d'integrità.

## Decisioni aperte al team Matrix

Scegliere la superficie di configurazione (file, flag o variabile d'ambiente)
e il modello di fiducia per l'indice alternativo (per esempio URL HTTPS
amministrato e digest/versione pinnati). Stabilire se serva anche un
meccanismo nativo di riscrittura controllata della base degli artefatti o se
un indice mirror completo sia sufficiente. Il consumer può poi integrare la
configurazione nel proprio installer e rieseguire il collaudo Nido.
