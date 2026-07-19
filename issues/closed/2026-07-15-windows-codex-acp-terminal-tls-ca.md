# Issue: terminale Codex ACP su Windows non usa correttamente la trust CA locale

Data: 2026-07-15

Stato: rejected e chiusa il 2026-07-19

Versione MATRIX: `0.1.22`

Commit MATRIX: `8ea0a0a3ad96a97e2fc842115879242c060099c1`

Provider: `@agentclientprotocol/codex-acp` `1.1.2`

Sistema verificato: Windows 10, utente non amministratore `rober`

Priorita suggerita: alta per Half Pocket Desk

## Sintesi

Una run creata da Half Pocket Desk tramite il Local Companion arriva a MATRIX,
viene instradata all'agente `codex` e puo leggere correttamente la workspace
locale. Tuttavia, quando Codex esegue nel proprio terminale il client Go
`halfdesk-agent doctor`, la connessione HTTPS fallisce:

```text
tls: failed to verify certificate: x509: certificate signed by unknown authority
```

Lo stesso eseguibile, con lo stesso utente Windows e sulla stessa macchina,
completa `halfdesk-agent doctor` con stato `ready` quando viene avviato
direttamente da PowerShell/SSH fuori dalla sessione MATRIX/Codex.

## Evidenza

Configurazione della run:

```text
agent_id: codex
trusted_terminal: true
sandbox_mode: danger-full-access
approval_policy: never
workspace_path: C:\Users\rober\Documents\Half Pocket
```

Il trace della run conferma che il terminale puo eseguire comandi locali:

```text
halfdesk-agent version -> 0.3.2
halfdesk-agent doctor  -> x509: certificate signed by unknown authority
```

Controlli eseguiti fuori dalla run:

- `halfdesk-agent doctor` restituisce `ready` con lo stesso binario e utente;
- il certificato di `software.halfpocket.net` e valido e concatenato ad
  Actalis Domain Validation Server CA G3 / Actalis Authentication Root CA;
- il bundle CA di Git for Windows contiene la root Actalis;
- MATRIX e il Local Companion girano come utente `rober`;
- non risultano variabili proxy o CA custom nella shell diretta.

E stato anche impostato per l'agente MATRIX:

```text
SSL_CERT_FILE=C:\Program Files\Git\mingw64\etc\ssl\certs\ca-bundle.crt
```

La variabile risulta visibile nella run Codex, ma l'errore TLS resta identico.
Il workaround e stato quindi rimosso dalla configurazione persistente.

## Impatto

Il dialogo Desk -> Local Companion -> MATRIX -> Codex funziona e Codex puo
usare la workspace locale, ma i comandi `halfdesk-agent` che interrogano l'API
HTTPS di produzione non sono utilizzabili dentro il terminale agentico. Fuori
da MATRIX funzionano normalmente.

## Comportamento atteso

Un comando eseguito dal terminale trusted di Codex con
`danger-full-access` deve poter usare la stessa trust chain TLS disponibile
all'utente Windows che esegue MATRIX. Se il provider applica un isolamento del
trust store o dell'ambiente, MATRIX deve renderlo esplicito nella diagnostica.

## Criteri di accettazione

- Test Windows MATRIX-owned con provider Codex ACP canonico.
- Un client HTTPS nativo avviato nel terminale Codex valida una CA presente nel
  trust store dell'utente/macchina come fa nella shell diretta.
- `halfdesk-agent doctor` completa con stato `ready` sia fuori sia dentro la
  run MATRIX.
- Nessuna disabilitazione della verifica TLS e nessuna installazione di CA non
  necessarie.
- Eventuali differenze di ambiente, profilo utente o trust store sono esposte
  da `matrix agent doctor codex` o dal trace della run.

## Nota di classificazione

La riproduzione dimostra una differenza tra il terminale creato tramite
MATRIX/Codex ACP e la shell diretta. Non identifica ancora se la causa prima sia
nel launcher MATRIX, nel provider Codex ACP o nel runtime terminale Codex; la
issue e intenzionalmente formulata come contratto di integrazione e non come
attribuzione definitiva.

## Risoluzione maintainer

- Decisione: rejected e chiusa il 2026-07-19 come non pertinente a Matrix.
- Motivo: Matrix non termina la connessione HTTPS, non costruisce il pool CA e
  non applica un trust store proprio ai processi richiesti via ACP. Su Windows
  `terminal/create` avvia il comando con `os/exec`, nello stesso account del
  daemon; l'ambiente del processo Matrix viene ereditato e gli eventuali valori
  ACP sono aggiunti come override. L'endpoint Codex corrente dichiara inoltre
  `env_isolation=false`.
- Evidenza codice: `internal/providers/agents/terminal_request.go` conserva gli
  override `env` del protocollo; `internal/providers/exec/exec_windows.go`
  lascia a `os/exec` l'eredita dell'ambiente quando non ci sono override e usa
  `append(os.Environ(), spec.Env...)` quando sono presenti.
- Evidenza test: i test MATRIX-owned di `terminal/create` passano per esecuzione,
  working directory e lifecycle; il provider Windows compila per
  `GOOS=windows GOARCH=amd64`.
- Evidenza di classificazione: la segnalazione riguarda Matrix `v0.1.22` e non
  contiene una riproduzione con `v0.1.25`; l'impostazione di `SSL_CERT_FILE`
  risultava gia visibile al processo figlio, quindi non prova una perdita o una
  riscrittura dell'ambiente da parte di Matrix.
- Scope corretto: diagnosi del client Go `halfdesk-agent`, del certificato
  presentato al momento del fallimento e del contesto Windows del Local
  Companion. Una diagnostica CA specifica per un provider o consumer dentro
  Matrix introdurrebbe un percorso verticale vietato dalla governance.
- Condizione di riapertura: riproduzione su una versione Matrix corrente che
  confronti account/SID, ambiente effettivo e trust store dello stesso comando
  lanciato direttamente e tramite `terminal/create`, dimostrando una differenza
  introdotta da Matrix.
