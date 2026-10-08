# Sviluppi PAL del 2026-10-08

Mandato: tutti i quattro sviluppi precedentemente rinviati sono accolti, con
PAL Linux/Windows/macOS. Nessun nuovo rifiuto di uno dei quattro punti.
Base: `a2bae6867c369d087816c36f195b767763a197dd`, release locale/pubblica v0.1.52.
Release pubblicata: v0.1.53. Verifiche della pubblicazione e stato osservato
dell’installazione locale: [verbale release](releases/2026-10-08-v0.1.53.md).

## Implementazione

| Punto | Superficie consegnata | Confine esplicito |
|---|---|---|
| Sandbox | contratti OpenCode/MiMo e Codex esistente; driver container comune; agenti ACP locali e validatori; guest cwd; stato persistente; cleanup; doctor | policy tool configurata distinta dalle impostazioni OS verificate; driver/immagine espliciti; remoto non isolabile dal client |
| Capacità | disco/volume/CPU/RAM nativi; workspace/CLI/API; lease di concorrenza e prenotazione disco prima della route; codici persistenti | runtime Matrix corrente; prenotazione contabile; crediti provider non dedotti |
| FS semantico | `fs.FS`, CLI, API autenticata, mount rclone nativo; dati selezionati; sommari opt-in; ID portabili | no namespace config/credenziali; mount richiede prerequisiti reali; lettura singola non transazione tra file |
| Collector | OTLP/HTTP JSON logs; primario locale conservato; coda/batch/timeout/shutdown limitati; stats autenticate | solo eventi/etichette/contatori ammessi; no transcript; exporter log, non billing/span/metriche OTLP |

Configurazione e comandi: [guida PAL](../wiki/PAL-Execution-and-Observability.md).

## Prove realmente osservate su questa macchina Linux

| Prova | Esito |
|---|---|
| Osservatore e CLI su workspace reale con spazi | PASS: volume/disco/CPU osservati, nessun file creato nel workspace |
| Native policy OpenCode 1.18.31 | PASS: binario installato carica i divieti v1 in home/config temporanei; nessuna richiesta al modello |
| Native policy MiMoCode 0.1.15 | PASS: stesso controllo sul binario installato; nessun accesso alla configurazione privata del provider |
| Docker 29.8.2, Ubuntu 24.04 già presente | PASS: lettura del workspace, scrittura negata sul bind read-only e root, nessun socket Docker esposto, rimozione alla cancellazione |
| Validatore nello stesso driver | PASS: scrittura negata, codice reale 7 conservato, stato ritrovato alla seconda esecuzione, container rimossi |
| ACP dentro container reale con peer di collaudo | PASS: bridge FS/terminal host non pubblicati; segreto host non ereditato; sessione realmente salvata e caricata con stesso ID dopo reap/riavvio; nessuna seconda session/new |
| Mount rclone 1.75.1 con FUSE Linux | PASS: contenuto letto, scrittura negata, smontaggio osservato, run aggiornato dopo completamento |
| OpenTelemetry Collector ufficiale 0.162.0 | PASS: evento operativo ricevuto dal collector; marker privato assente |
| API semantica/telemetria | PASS: 401 senza chiave, 405 su mutazioni, snapshot aggiornato con richiesta condizionale, campi privati esclusi |
| Refusal capacità | PASS: richiesta raggiunge router comune, HTTP 429 e failure_code nel record/trace; lease rilasciata alla cancellazione; richiesta senza capacity conserva il payload idempotente |
| Collector guasto e privacy | PASS: primario file conservato; failure/partial-rejection/drop osservati; redirect/URL insicure rifiutati; coda finita e produttore non bloccato |

Le prove di policy native verificano il caricamento dello schema. Il peer ACP è
una fixture dichiarata e non un collaudo commerciale di un modello. La prova
OS è un processo reale nel container, non una simulazione del filesystem.

Artefatti di collaudo ottenuti dalle release ufficiali e verificati:

- rclone v1.75.1 Linux amd64, SHA256 dell'archivio
  `982b5aa772841168f8e380f139e9e787b2a105403e32b94da8676a0e1c0a13ab`.
- otelcol v0.162.0 Linux amd64, SHA256 dell'archivio
  `f99929987a915d3c6b2c9b15bc4938c5cea903a37a3f49e478024a0fa0772339`.

Esecutori, download e log sono scratch dell'agente; non sono installazioni
globali né dati da aggiungere al repository. Le immagini Docker preesistenti
sono riusate e preservate. Nessuna policy globale del servizio utente è attivata
durante questi test.

## PAL e qualifiche

`.github/workflows/pal.yml` esegue test, build e CLI native su Ubuntu, Windows e
macOS: capacità/memoria, admission, decoder/policy, guardie ACP, semantica FS,
autenticazione e collector. L'esecuzione riuscita va provata sull'esatto SHA.
Le compilazioni incrociate sono controlli complementari.

Un runner privo di Docker Desktop/macFUSE/WinFsp non certifica quei driver.
Mount e container fisici Windows/macOS richiedono una macchina con i rispettivi
prerequisiti. Il codice offre il contratto comune e rifiuta l'assenza del driver;
questo ledger non dichiara prove fisiche Windows/macOS avvenute su Linux.

Riproduzione delle prove opzionali:

```bash
MATRIX_REAL_CONTAINER_ENGINE=docker go test ./internal/providers/containersandbox ./internal/providers/agents -run '^TestSmokeReal' -count=1 -v
MATRIX_REAL_FUSE_DRIVER=/path/to/rclone go test ./tests/integration -run 'TestFUSE_MountAndRead|TestSmokeFUSELive' -count=1 -v
MATRIX_REAL_OTLP_ENDPOINT=http://127.0.0.1:19381 go test ./internal/providers/otlplog -run '^TestSmokeOfficialCollector' -count=1 -v
```

## Governance

Le soglie file/funzione/parametri/rami/warning non sono aumentate. I package
esistenti crescono per nuove superfici funzionali, con baseline LOC misurate:

| Package | Budget precedente | Nuova baseline |
|---|---:|---:|
| cmd/matrix | 4400 | 4744 |
| internal/logic/agentlaunch | 800 | 890 |
| internal/logic/agentmgr | 1305 | 1325 |
| internal/logic/session | 5826 | 5887 |
| internal/logic/workspace | 1099 | 1111 |
| internal/middleware | 1190 | 1269 |
| internal/providers/agents | 5140 | 5312 |
| internal/providers/runapi | 2649 | 2691 |

Il vecchio mount statico e la dipendenza go-fuse inutilizzata sono rimossi.
L'esenzione dai test di fusefs è rimossa: ora ci sono prove di rifiuto,
autenticazione e un collaudo di mount reale. Il preflight usa scratch privato
anche per il report delle capacità di orchestrazione.

Gate locali: deploy preflight, quality gate con tutte le soglie di copertura,
lint, governance, scansione dello stage e diff check completati sul codice.
Release v0.1.53 verificata con 88 controlli e firme di tutti i 12 archivi/SBOM,
vincolate a tag, SHA sorgente e workflow. Installer pubblico isolato collaudato.
Il servizio locale resta in attesa della conclusione dei task già attivi prima
del backup coerente e del riavvio; lo stato è esplicito nel verbale della release.

## Difetti emersi dalla prima CI nativa

La prima esecuzione PAL su Windows/macOS ha trovato difetti reali preesistenti
nei percorsi dei validatori e nei permessi dei log. I workspace vengono ora
canonicalizzati prima del confronto e gli artifact path radicati/drive-qualified
sono rifiutati su tutti gli OS. Alias di workspace restano validi, link esterni
restano rifiutati. I log Windows usano una DACL protetta dell'account corrente,
applicata alla creazione e ai file esistenti prima delle scritture; i test
verificano l'ACL Windows invece di richiedere mode bits Unix.
Le correzioni sono sottoposte nuovamente alla stessa CI nativa.

### Qualifica nativa osservata

PAL run [37794391626](https://github.com/Josepavese/matrix/actions/runs/37794391626)
su SHA `b9b369c4adc21d6e16f2da6367c1feb58a9b52d8`: Ubuntu, Windows e macOS
PASS. Capacità/ram, contratti, guardie, API e CLI sono prove native, non cross-build.
Il confronto ACL Windows usa SID binario e diritti effettivi: la stringa SDDL
può abbreviare un SID conosciuto e non è un confronto di identità affidabile.

La CI generale ha inoltre fatto emergere una race residua del live context:
il watcher cancellava il provider prima di pubblicare la prova terminale,
permettendo al recorder concorrente di registrare `failed` al posto di `late`.
La prova viene ora pubblicata prima della cancellazione, con regressione
che verifica deterministicamente l'ordine e ripetizioni sotto race detector.

### Interoperabilità del peer di collaudo cwd

La prima CI dell'esatto merge SHA ha bloccato un test perché il suo peer shell
emetteva `initialize` prima di ricevere la richiesta JSON-RPC. Su un avvio veloce
la risposta arrivava prima della registrazione del correlatore e veniva persa.
Il peer ora risponde soltanto dopo la richiesta; le route di collaudo hanno
anche un timeout esplicito. Il controllo reale del cwd e dell'isolamento dei
workspace resta invariato, con ripetizioni sotto race detector.
