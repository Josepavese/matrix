# Qualifica Knowledge: OpenCode locale privo di ricevuta di installazione

Nota di un agente esterno Half Pocket, 27 ottobre 2026. Nessun altro file di
questo repository è stato modificato; nessun commit/push o modifica
all’installazione Matrix. Il confine del proprietario permette solo questa nota.

## Ambiente e osservazione

Matrix locale0.1.61, build686602cae0c85b849c4b0607c723ab350a94ec2c;
repository letto353c59967ebfcbe5003cefadff40029125be43ed. Il runtime HTTP loopback
risponde200 autenticato: opencode active=true, installed=true,
status=ready_on_demand, ma senza artifact_verification. Nessuna chiave è esposta.
`matrix agent verify opencode --json --strict` termina1:
method=recorded_evidence, verified=false, dettaglio «no artifact verification
was recorded for this per-home install (launcher present); the bytes were
not re-proven offline». L’handshake ACP non dimostra provenienza dell’artefatto.

## Effetto e decisioni già prese

Half Pocket Hub Matrix.VerifyAgent richiede il verdetto autorevole autenticato
per l’agente esatto prima del dispatch. Non si aggira il guard, non si usa un
altro agente/modello e non si calcola un hash locale come seconda authority.
Non è dimostrato un difetto del software Matrix: l’installazione locale può
semplicemente non avere l’evidenza necessaria. Nessun run AI, deploy VPS,
aggiornamento freshness o ripresa PM è stato avviato da questa qualifica.

## Presa in carico e condizione di sblocco

L’owner deve indicare/eseguire il percorso autorizzato per un’installazione
OpenCode con provenienza verificabile, preservando configurazioni, credenziali,
sessioni e lavoro in volo. Verificare poi sia il comando owner sia il verdetto
esposto da /_matrix/runtime. Se il percorso corrente non copre questa situazione,
indicare il limite e il cambiamento necessario senza ricostruire ricevute false.
La prova Knowledge→Hub→Matrix riprende solo dopo questo read-back, con gli stessi
contratti e senza sostituirla con una risposta simulata. I requisiti Knowledge
indipendenti continuano nel repository Half Pocket.

---

## Decisione Matrix (2026-10-28)

**Decisione: accettata. Il percorso owner esiste e copre questo caso, con una
condizione di deployment; costruito il verdetto che lo rende leggibile invece di
fallire senza dire cosa fare.**

### Il percorso owner, verificato

`opencode` è una distribuzione **binary** nel registro governato, quindi il
percorso esiste ed è una sequenza di comandi, non una modifica da fare:

```bash
# come account publisher
matrix agent store publish opencode --store /srv/matrix/store
# come utente del runtime, su ogni PAL home
matrix install opencode --store /srv/matrix/store
matrix agent verify opencode --strict     # method=store_tree, verified=true
```

Verificato qui: il registro pubblica `opencode` 2.2.0 `linux-x86_64` con sha256
`6d597970a8cc448f1b9a9f645532584b339dd3ab9583bb5f063f13d83e54768f`, e la
pubblicazione reale in uno store di prova è riuscita (246 831 280 byte
misurati). L'handshake ACP non entra nel verdetto: esso riguarda i byte
installati, non il protocollo.

### La condizione di deployment, dichiarata

L'utente del runtime **non deve poter scrivere** lo store. Su una postazione a
singolo account — quella della segnalazione — quella condizione non è
soddisfacibile: `install --store` rifiuta di installare da uno store scrivibile.
Serve un secondo account o un mount in sola lettura. Non è un difetto ed è
documentato; è però il motivo per cui il percorso qui non era mai stato eseguito.

### Cosa è stato costruito

- **`matrix agent verify` riporta un `remedy` per ogni risultato** e nomina la
  sequenza di adozione. È esattamente il caso della segnalazione, ora osservato:
  `{"method":"recorded_evidence","verified":false,"remedy":"publish the verified
  registry artifact into a shared store and reinstall from it: matrix agent store
  publish ..."}`.
- **Nuovo metodo `store_writable`**: quando lo store è scrivibile dall'utente, i
  byte vengono comunque misurati e il risultato dice che la corrispondenza non è
  affidabile perché quei byte possono essere scambiati dopo il controllo. Prima
  i due fatti — «ruolo sbagliato» e «byte sbagliati» — arrivavano entrambi come
  un fallimento indistinto.
- **I limiti dichiarati al posto dei comandi impossibili**: un agente `npx`/`uvx`
  non ha alcun archivio da pubblicare, e un agente registrato per path non è mai
  stato installato da un artefatto del registro. Entrambi dicono il proprio
  limite invece di offrire la stessa ricetta.

### Cosa NON è stato fatto, e perché

- **Nessuna ricevuta per un binario per-home installato da script.** Registrare un
  digest dei byte attuali non attestarebbe una provenienza: attestarebbe solo che
  *quei* byte erano lì, che è precisamente la «ricevuta falsa» che la nota
  vieta. Se serve una provenienza reale, il percorso è lo store.
- **`/_matrix/runtime` non espone un verdetto sui byte.** Ricalcolare l'albero a
  ogni interrogazione del runtime non è una lettura a buon mercato, e quella
  superficie riporta l'evidenza di installazione, non una prova. Il comando owner
  resta `matrix agent verify`. Limite dichiarato, con il motivo.
- **Il read-back Knowledge→Hub→Matrix è del consumer**: qui è stato verificato il
  comando owner su questa postazione, non il verdetto sulla sua installazione.
