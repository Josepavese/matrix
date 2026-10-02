# Reversioni — identità del figlio (pid riusato)

Metodo del repo: si copia il file, si applica la reversione, si compila, si esegue
il test, si ripristina **dalla copia** (mai con `git`). P1–P3 stanno in
`identity.go`, P4 nel ciclo di osservazione in `probe.go` (`observeChild`).

| nome | cosa reverte | test che cade |
|---|---|---|
| P1 | confronto dello start time rimosso dalla guardia | `TestARecycledPIDIsRefusedByTheStartTimeItShows` |
| P2 | verifica del parent rimossa dalla guardia | `TestAnObservationOfAnotherProcessIsRefused` |
| P3 | start time letto dal campo 18 invece che dal 22 | `TestStatFieldsReadsPositionsNotNames`, `TestReadIdentityReadsTheStartTimeTheKernelReports` |
| P4 | il ciclo di osservazione non consulta più la guardia (la chiamata a `guard.accept` sparisce) | `TestTheObservationLoopRefusesAChildTheGuardDoesNotRecognise` |

P4 esiste perché P1–P3 provano la **regola** e nessuna di esse prova che il probe
la interroghi: senza P4 si può togliere la chiamata e lasciare il pacchetto tutto
verde. Con la chiamata rimossa cadono solo il test di P4 (osserva un figlio vivo
invece di riportarlo come rifiutato) e nessun altro, che è la prova che il test
misura il collegamento e non la regola.

## Che cosa chiude, misurato

Il pid è un numero che il kernel ricicla: da solo non dice a quale processo
appartiene. La finestra concreta nel probe è questa: si legge
`/proc/<pid>/{cwd,cmdline,stat}` in un ciclo fino a 500 ms, e il figlio può essere
già morto.

Misurato su questo host (`/bin/sleep` ucciso e non reaped): il figlio resta in
stato `Z` e `/proc/<pid>` **esiste ancora**; sparisce solo dopo il reap. Il probe
reapa il figlio nel `Wait()` differito, cioè **dopo** il ciclo, quindi durante
l'osservazione il pid non è riassegnabile — e un figlio morto risulta `unreadable`,
non un processo altrui.

La guardia rende quella proprietà indipendente dal ciclo di vita del provider, che
vive in un altro pacchetto: un'osservazione è evidenza solo se il kernel dice che
il processo è ancora figlio di chi l'ha avviato (`/proc/<pid>/stat` campo 4) **e**
mostra lo start time osservato la prima volta (campo 22). Un rifiuto è terminale:
aspettare non fa tornare il figlio, quindi il probe chiude subito invece di
riportare l'identità di chi ha ereditato il numero.

## Limiti dichiarati

- Lo start time è in tick di clock dal boot: due processi diversi hanno lo stesso
  valore solo se avviati nello stesso tick, e il pid deve anche essere stato
  riciclato nello stesso istante. La prova non costruisce quel caso con processi
  veri (non è riproducibile a comando): la regola è provata sulla guardia, il
  parsing è provato contro il kernel su un processo reale.
- Questi test girano su Linux; su Windows e Darwin `readIdentity` non esiste e il
  probe riporta l'errore dichiarato. La compilazione incrociata è verificata,
  l'esecuzione no.
