# Esecuzione, capacità e osservabilità PAL

Le quattro superfici sono opzionali e usano gli stessi contratti su Linux,
Windows e macOS. Una richiesta esplicita non viene degradata quando manca un
driver, un'immagine o un'osservazione. Le configurazioni precedenti continuano
a funzionare quando questi contratti non sono richiesti.

## Sandbox

`MATRIX_SANDBOX` è una dichiarazione JSON nell'ambiente dell'agente, configurabile
con `matrix agent env set <id> MATRIX_SANDBOX '<json>'`. Campo sconosciuto,
chiave duplicata, profilo o contratto sconosciuto producono un rifiuto.

La policy nativa e il confine OS si possono richiedere separatamente o insieme.
La policy nativa configura i tool del provider: il suo stato è
`configured_not_attested`. Le impostazioni del progetto, degli agenti e quelle
gestite dal provider possono influire sulla policy finale. Il confine OS usa
un motore Docker locale con container Linux; Windows/macOS richiedono un motore
come Docker Desktop già configurato. Matrix non installa il motore e non
scarica né costruisce immagini durante l'avvio.

### Permessi nativi

```json
{"native_contract":"opencode-permission-v1","native_profile":"read-only"}
```

Per MiMo usare `mimocode-permission-v1`. Sono contratti dichiarati dall'operatore,
indipendenti dal nome assegnato all'agente. Traducono il profilo in
`OPENCODE_PERMISSION` / `MIMOCODE_PERMISSION`, conservando ordine e regole
preesistenti e aggiungendo solo divieti:

| Profilo | Divieti aggiunti |
|---|---|
| `workspace` | `external_directory`, `task`, `actor` |
| `read-only` | precedenti più `edit`, `write`, `patch`, `multiedit`, `bash` |

Un bypass dei permessi in argv/env viene rifiutato. Matrix disabilita i propri
tool ACP su host, l'autenticazione tramite terminale e la scelta automatica
della modalità più permissiva. MCP, directory aggiuntive e tool di estensione
del caller richiederebbero un confine separato e vengono rifiutati.

I contratti v1 seguono lo [schema OpenCode v1](https://dev.opencode.ai/docs/permissions/)
e i [sorgenti MiMo](https://github.com/XiaomiMiMo/MiMo-Code/blob/main/packages/cli/src/config/permission.ts).
Lo [schema OpenCode v2](https://opencode.ai/v2/docs/permissions) è differente e
non è coperto dal contratto v1.

### Isolamento del processo

Esempio di dichiarazione; directory, UID/GID e immagine vanno scelti sul proprio host:

```json
{
  "native_contract": "opencode-permission-v1",
  "native_profile": "workspace",
  "container": {
    "engine": "docker",
    "image": "my-prepared-agent:version",
    "workspace_access": "workspace-write",
    "network": "bridge",
    "state_dir": "/explicit/provider-state-base",
    "user": "1000:1000",
    "memory_bytes": 536870912,
    "cpus": 2,
    "pids": 128
  }
}
```

`workspace_access`: `read-only` oppure `workspace-write`; `network`: `none` oppure
`bridge`. UID e GID sono numerici e diversi da zero. RAM, CPU e PID richiedono
limiti espliciti. Il workspace dichiarato è montato in `/workspace`, lo stato in
`/home/matrix`, la root è in sola lettura, `/tmp` è limitato a 64 MiB senza exec,
le capability sono rimosse e `no-new-privileges` è richiesto. I bind non includono
mount annidati. Non sono ammessi root/home dell'utente, stato sovrapposto al
workspace, immagini con volumi persistenti impliciti o daemon remoti.
Il driver verifica supporto ai limiti nel motore e impostazioni effettive del container prima dell'attach;
il log driver Docker è `none` per evitare copie dei transcript sul disco del motore.

Il comando deve esistere nell'immagine Linux, anche quando Matrix gira su Windows:

```bash
matrix sandbox command opencode /usr/local/bin/opencode --arg=acp --arg=--pure
matrix sandbox doctor opencode --workspace /existing/host/workspace
```

Gli argomenti sono quelli dell'immagine, senza traduzione implicita di percorsi host. ACP riceve il workspace guest `/workspace`. Il risultato del task include `sandbox_execution` con digest dell'immagine e `engine_configuration_verified`, distinto dall'attestazione dei tool nativi.

Lo stato è separato per identità, comando e workspace fisico, stabile attraverso
riavvii e cambi di profilo. Non viene cancellato alla chiusura. Le credenziali
necessarie vanno configurate esplicitamente nell'ambiente del provider o nella
sua directory di stato; la home host non viene montata. La chiusura/cancellazione
rimuove soltanto il container con nome casuale creato da quel driver.
Una policy cambiata non può riutilizzare un client precedente: prima serve
terminare le sue lease e reap del client, preservando gli altri task attivi.

Doctor/prerequisiti non dichiarano un handshake avvenuto: un container richiede
un workspace esplicito. Il runtime globale riporta `sandbox_requires_workspace_probe`
e il router applica il confine quando il task viene eseguito.

### Validatori

`delivery_contract.validator.sandbox` accetta lo stesso oggetto `container`.
Il comando è un argv dell'immagine. Il verdetto usa il codice di uscita del
container realmente terminato, senza conservare stdout/stderr. Il runner usa
gli stessi limiti, bind, cancellazione e rimozione degli agenti. L'assenza del
runner o un errore del motore rende il verdetto `unverifiable`, senza esecuzione
alternativa sull'host. Senza richiesta di sandbox resta il contratto precedente.

## Capacità

```bash
matrix capacity /existing/workspace
matrix workspace show <workspace-id>
```

La misura è del filesystem del workspace effettivo, con identificatore di volume,
byte disponibili all'utente e byte totali, CPU logiche e RAM nativa. Linux usa
statfs/sysinfo, Windows le API volume/memoria, macOS statfs/sysctl/vm_stat.
RAM libera esclude le cache recuperabili; macOS esclude anche pagine speculative.
Zero è una misura valida; un dato non osservato è assente con la sua motivazione.
L'osservatore non crea file e non pulisce il disco.

Configurazioni globali: `capacity.max_concurrent` e
`capacity.min_disk_free_bytes` (zero disabilita il relativo limite).
`POST /v1/runs` può aggiungere:

```json
{"capacity":{"min_disk_free_bytes":1073741824,"reserve_disk_bytes":268435456}}
```

Le lease coprono le route attive di questo runtime, su tutti i canali. Le
prenotazioni sono contabili per volume, non preallocazioni o quote del filesystem.
Si rilasciano anche in errore/cancellazione; non coprono altri runtime, processi
esterni o il credito di un provider. I limiti fisici di CPU/RAM/PID del processo
sono quelli del container. `GET /_matrix/capacity?workspace_id=<id>` richiede
l'autenticazione runtime; osservazioni mancanti e limiti esauriti hanno codici
distinti, registrati anche nel trace e nella notifica terminale.

## Filesystem semantico

```bash
matrix fs list
matrix fs path runs <run-id>
matrix fs read runs/<encoded-id>/status.json
matrix fuse mount /existing/empty/mountpoint --driver-path /path/to/rclone
```

Espone `agents`, `runs`, `workspaces`: stato dichiarativo degli agenti, stato dei
run, metadati e capacità dei workspace. Gli ID sono reversibili e distinti anche
su filesystem senza distinzione di maiuscole, evitando nomi riservati Windows.
Limiti: 125 byte UTF-8 per ID, 4096 entità per directory, 64 KiB per sommario.
Gli errori di limite sono espliciti e non troncano dati.

Config, credenziali, input, errori grezzi e metadati arbitrari del client sono
esclusi. `summary.txt` è terminale e disponibile soltanto con
`--include-summaries`; può contenere testo privato. `GET/HEAD /_matrix/fs/`
richiede autenticazione ed esclude sempre i sommari.
Getter e risposte HTTP restituiscono snapshot della singola lettura;
non promettono una transazione comune tra più file.

Il mount usa [rclone HTTP](https://rclone.org/http/) e il driver nativo:
Linux FUSE, macFUSE/FUSE-T su macOS, WinFsp su Windows. La proiezione HTTP privata
ha una chiave casuale propria, non la chiave admin Matrix. Permessi owner-only,
mount sola lettura, niente cache su disco. Per le viste vive non usa HEAD o cache
di directory: la dimensione prima della lettura può essere sconosciuta/zero.
La prova di mount richiede un driver realmente installato; un test di API o una
compilazione non certificano il mount. Dati esistenti nel mountpoint non vengono
nascosti e la directory non viene rimossa allo smontaggio.

## Collector

L'esportatore opzionale usa [OTLP/HTTP JSON logs](https://opentelemetry.io/docs/specs/otlp/).
Configurare `system.logging.format=json`,
`system.logging.collector.endpoint` e, se necessaria,
`system.logging.collector.authorization` nel vault. `queue_size` è opzionale,
tra 16 e 4096. Il log locale configurato resta la destinazione primaria.

Esporta soltanto nomi di evento, etichette selezionate e contatori/durate ammessi;
non esporta prompt, transcript, messaggi grezzi o attributi arbitrari. HTTPS
obbligatorio, salvo loopback esplicito; redirect, credenziali in URL e query
sono rifiutati. Coda, batch, timeout e shutdown sono limitati; una rete lenta
non blocca il produttore. Rifiuti parziali OTLP sono conteggiati come tali.

`GET /_matrix/telemetry` autenticato mostra acknowledgment, drop, filtri privacy,
batch falliti, coda e ultimo codice di errore, senza credenziali. Questo exporter
invia log operativi: non aggiunge span, metriche OTLP o integrazioni di billing.
Quando disabilitato non apre connessioni né crea il worker di export.
