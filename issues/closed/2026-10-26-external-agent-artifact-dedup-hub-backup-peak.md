# Copie degli agenti per worker e picco del backup Hub

Data: 26 ottobre 2026. Stato: segnalazione esterna, da prendere in carico.

Questa e' una nota di un agente esterno al repository Matrix, impegnato nel
goal di indicizzazione amministrativa di Half Pocket. Questo file e' l'unica
scrittura effettuata qui dall'agente: nessun altro file Matrix e' stato
toccato e non sono stati eseguiti commit o push.

## Ambiente e revisione

- VPS Linux Half Pocket, disco40GiB non ampliabile per decisione del titolare.
- Hub0.4.32-readonly-plan.1, inventario Core290, Mail0.7.53-ai-reading.1.
- Matrix nel pacchetto Hub qualificato: binario SHA256
  `13ce7b63cf532e67912f329e4f3f676be4b5745c9559adee99f8bc0f3350d8a7`,
  provenienza `2f10dd98c217ad14344172514213b572661ba69b` nel BUILD.json.
- Repository Matrix consultato in sola lettura alla revisione
  `7a345269802cb20ddeeb3eff5b473363fea0c356`; non si presume che coincida
  con il binario installato.

## Osservazione e prova

Il budget nativo Hub, calcolato con `require_full_backup_capacity`, riporta:

| Voce | Byte |
| --- | ---: |
| Immagine dati prevista | 3.983.351.808 |
| Vista privata DB/WAL/SHM | 54.722.560 |
| Limite superiore export compresso | 5.640.325.764 |
| Riserva canonica | 2.147.483.648 |
| Picco massimo richiesto | 11.825.883.780 |
| Spazio libero osservato | 9.184.960.512 |

La misura metadati della voce `workers` nel backup e'3.716.993.024B.
`du -x -B1 --max-depth=3 /var/lib/hub-preview/workers` mostra otto alberi
`<worker>/matrix/agents`, ciascuno di circa185.180.160B allocati. Insieme
occupano circa1.48GB. La misura non dimostra ancora che ogni contenuto sia
identico: occorre qualificare i digest e distinguere le versioni effettive.
La directory `matrix/data` contiene invece stato utile e resta protetta.

Nel codice consultato `cmd/matrix/app.go` costruisce gli installer con
`BaseDir: ""` (circa righe194 e245); `agentmgr.NewInstaller` risolve allora
`matrixhome.AgentsDir(home)` (`internal/logic/agentmgr/installer.go`).
`installBinary` estrae in `agentinstall.AgentDir(inst.baseDir, manifest.ID)`.
La verifica SHA256 del download appartiene gia' a Matrix, in
`internal/logic/agentmgr/artifact_integrity.go`.

Il risultato e' software distribuito in ogni home e ricopiato nel backup
Hub insieme allo stato. Il costo si ripresenta aggiungendo worker. Copie
uguali di software ricostruibile non devono richiedere un backup completo
aggiuntivo per ogni worker.

## Intervento richiesto nel proprietario

1. Qualificare un deposito immutabile degli artefatti agente, indirizzato dal
   digest verificato da Matrix, riusabile da worker distinti. Riusare PAL,
   registro e verifica esistenti. Configurazione, credenziali, sessioni,
   eventi e workspace mantengono ownership e isolamento per worker.
2. Rendere espliciti writer, permessi e pubblicazione atomica. Un worker non
   deve poter alterare il software eseguito da un altro. Il solo `BaseDir`
   condiviso e scrivibile da tutti non soddisfa questo requisito.
3. Prevedere una conversione delimitata delle copie installate e verificate,
   con preview, confronto contenuti/permessi, riferimenti attivi e recupero.
   La scelta fra link e riferimenti nel registro dipende dai contratti
   correnti; non applicare hardlink o symlink alla cieca dall'Hub.
4. Integrare nel lifecycle il riuso e il ritiro delle generazioni software
   senza piu' consumer, anche dopo interruzione. Le prove di digest,
   licenze/provenienza e il codice necessario al recupero restano validi.
5. Esporre al consumer Hub il contratto necessario a ripristinare software
   rigenerabile senza includerne una copia per worker nel backup dei dati.
   Una proiezione o un riferimento non deve essere dichiarato backup completo
   di dati che non contiene.

## Decisioni gia' prese e limiti

- Il titolare autorizza pulizia aggressiva delle copie rigenerabili e una
  soluzione sistematica; dati utili, sessioni, WIP e ultimo backup realmente
  recuperabile restano protetti. Il disco mantiene la capacita' corrente.
- I byte software condivisi devono essere verificati e immutabili; la
  condivisione non riguarda stato, identita', credenziali o conversazioni.
- Le riserve di disco non si abbassano per far passare l'operazione.
- La policy delle patch resta di `workspace.service-freshness.weekly`.
  Questa conversione non riapre PM/VICKY e non avvia aggiornamenti impliciti.
- Il 26 ottobre 2026 il titolare ha confermato che le attivita' di un altro
  repository si affidano esclusivamente tramite `issues/`. La presa in carico
  e' del proprietario Matrix: l'agente Half Pocket non modifica codice o
  installazione Matrix e non esegue commit o push in questo repository.

## Criteri di verifica

Provare deduplicazione su digest uguali, isolamento di versioni differenti,
diniego di scrittura da altri worker, interruzione/ripresa, ritiro con lettore
vivo e dopo il suo ultimo uso. Verificare i backend supportati con contratti
di piattaforma espliciti. Sul percorso Hub confrontare lo stato utile prima
e dopo, verificare runtime/agent attivi e digest autentici, misurare lo spazio
realmente liberato e ripetere il budget nativo del backup. Nessun risparmio
stimato viene dichiarato ottenuto prima della misura reale.

## Esito Matrix (26 ottobre 2026)

**Accolta in parte e chiusa con limiti dichiarati**, secondo
[issue_governance.md](../../docs/governance/issue_governance.md): l'accoglienza
non è un impegno a costruire l'intera richiesta, e le parti che chiedono
garanzie che Matrix non può osservare vengono risposte con un limite, non
simulate. Rilasciata in v0.1.61.

### Verifica delle affermazioni della segnalazione

- Confermato: `cmd/matrix/app.go` (circa righe 194 e 245) costruisce gli
  installer con `BaseDir: ""`, e `agentmgr.NewInstaller` risolve
  `matrixhome.AgentsDir(home)`; un'installazione binaria viene quindi estratta
  sotto il PAL home corrente e ricopiata nel backup che include quel home.
- Plausibile ma non verificabile da questo repository: la misura di ~185 MB per
  ciascuno degli otto alberi `<worker>/matrix/agents`. Matrix non ha accesso al
  VPS Hub e non può confrontare quei digest. Lo store rende la domanda
  verificabile: una generazione è identificata dal digest che Matrix verifica già.
- Confermato nel perimetro Matrix: nessuna correzione del repository poteva
  ridurre il picco del backup Hub. Il backup, la riserva di disco e la lista
  `workers` sono del consumer e non sono stati modificati.

### Cosa è stato costruito (v0.1.61)

1. **Deposito immutabile content-addressed** (`internal/logic/agentstore`):
   generazione indirizzata dallo sha256 dell'archivio che Matrix verifica già
   (`320959bb…` per l'artefatto qualificato), pubblicazione atomica con staging e
   rename, metadati scritti per ultimi, albero reso read-only (i bit di scrittura
   sono rimossi, gli eseguibili restano eseguibili), manifest dell'albero
   (path/mode/size/sha256) e suo digest.
2. **Ruoli espliciti publisher/consumer**: `matrix agent store publish` scarica,
   verifica, estrae e pubblica; rifiuta un indice senza sha256 valido e un digest
   che non corrisponde, e rifiuta di ripubblicare byte diversi sotto lo stesso
   digest. `matrix install <id> --store <root> [--digest <sha256>]` registra
   l'agente senza copiare software nel home.
3. **Pubblicazione con writer/permessi espliciti e fail-closed**: il consumer
   rifiuta uno store che l'utente runtime può scrivere, controllando la radice e
   le directory di layout che decidono il contenuto di una generazione
   (`blobs`, `blobs/sha256`, `generations`); il messaggio nomina la rimedio
   (pubblicare da un altro account o montare lo store read-only). Rifiuta anche
   generazione mancante, piattaforma diversa, digest appartenente a un altro
   agente e albero manomesso (verify completo prima di scrivere il record).
4. **Riferimenti per home**: `<home>/configs/agent_references.json` e
   `agentcfg.ArtifactReference` nel Vault (store, digest, digest del manifest,
   launcher, versione, piattaforma); l'uninstall rimuove il riferimento, così il
   GC può ritirare la generazione davvero inutilizzata.
5. **Conversione delimitata senza link ciechi**: `matrix agent prune` verifica
   prima la generazione registrata, poi riporta i byte reali della copia
   superseded; `--apply` rimuove solo la directory software, mantenendo Vault,
   config e riferimento. Nessun hardlink o symlink viene applicato: la
   conversione è la sequenza esplicita publish → install --store → prune.
6. **Lifecycle e ritiro**: `matrix agent store gc` è dry run di default, non
   tocca le generazioni referenziate, rifiuta `--apply` quando la lista `--home`
   non contiene alcun riferimento (guardia contro una lista errata) e rimuove
   anche lo staging di pubblicazione più vecchio di un'ora.
7. **Contratto consumer per il backup**: `matrix agent inventory` deterministico e
   senza segreti (ordinato per id, env ridotto ai nomi, nessun timestamp né
   valore), `matrix agent verify` con distinzione esplicita fra `store_tree`
   (verifica offline completa dell'albero) e `recorded_evidence` (prova
   registrata, non riprova dei byte), `matrix agent restore --inventory` che
   reinstalla da generazione verificata o dal registro governato e rifiuta
   versioni diverse senza `--allow-version-drift`.
8. **Documentazione**: [contratto dello store](../../docs/matrix_agent_artifact_store.md)
   e aggiornamenti a `docs/wiki/Using-Agents.md` e `docs/wiki/CLI-Reference.md`;
   nota di rilascio `docs/governance/releases/2026-10-26-v0.1.61.md`.

### Prove eseguite

- Gate: `scripts/quality_gate.sh` (gofmt, vet, build, 93 pacchetti sotto race,
  coverage ratchet, governance manifest) e `scripts/release_gate.sh` sulla
  revisione rilasciata (export del commit, build Linux/Windows/macOS, race,
  governance, coverage) verdi; `code_governance` senza failure e senza warning.
  Lint CI (golangci-lint v2.14.0) pulito sul commit.
- Rilascio: CI `40334205776` e PAL `40334205890` verdi sul commit
  `686602cae0c85b849c4b0607c723ab350a94ec2c`; workflow Release
  `40334994356` verde (release-gate, goreleaser, SBOM SPDX e attestazione di
  provenienza); `scripts/verify_release.sh v0.1.61` = 88 controlli ok, 0 falliti,
  15 asset, non draft/prerelease.
- Artefatto installato: `matrix 0.1.61`, commit `686602c`, build `go1.27.3`,
  sha256 del binario `d038a92e…`; deploy locale con checksum verificato, servizio
  riavviato, `matrix readiness --strict` = `ready` (`matrix_http_up: true`,
  nessun warning).
- Smoke funzionale sull'artefatto installato, due home isolate: publish
  `amp-acp 0.10.0` (`linux-x86_64`, generazione `320959bb…`, tree manifest
  `17c7d8d2…`); install da store con store scrivibile **rifiutata**; install da
  store sigillato; `prune --apply` ha liberato `81.737.184` byte; `verify
  --strict` = `store_tree` ok; restore nella seconda home solo da inventario +
  store, senza nuovo download; `verify --strict` ok in entrambe; GC dry run con
  le due home = 0 byte reclamabili. Misura `du -sb`: store `81.737.951` byte,
  `agents/` di entrambe le home `0` byte.

### Risposta con limite (non costruito per scelta)

- **Isolamento di scrittura fra worker**: Matrix non può garantire che un worker
  non possa alterare il software di un altro. Può solo rifiutare uno store che
  l'utente runtime può scrivere, e in ambiente a singolo account il proprietario
  può sempre riabilitare la scrittura: l'immutabilità cross-account resta una
  proprietà del deployment (account diverso o mount read-only). Questa è la
  risposta al punto 2, non una sua implementazione completa.
- **Picco backup Hub, riserva di disco e lista `workers`**: non toccati. Matrix
  non modifica la configurazione di backup, non abbassa `require_full_backup_capacity`,
  non riduce riserve e non tocca il VPS Hub (l'agente esterno non è autorizzato).
  Fornito il contratto che rende l'esclusione del software rigenerabile una
  decisione misurata del consumer.
- **Migrazione automatica, hardlink/symlink, cancellazione autonoma**: non
  costruiti. La conversione è una sequenza esplicita con `--apply`; Matrix non
  riscrive né cancella il software di una home da solo.
- **Confronto dei digest degli otto worker**: non costruito. Lo store rende
  possibile pubblicare una generazione per piattaforma e verificarla offline;
  l'inventario e le misure restano del deployment.
- **Nessun risparmio dichiarato prima della misura**: la nota di rilascio riporta
  solo i numeri misurati nello smoke locale e i comandi per misurare un
  deployment reale; non estrapola il risparmio Hub.
- **Limiti tecnici dichiarati**: estrazione delegata al provider esistente, che
  scarta voci ostili non-launcher (un launcher ostile fallisce chiuso); store
  specifico per piattaforma; su Windows l'ownership non è dimostrabile e uno
  store scrivibile dal proprietario conta come scrivibile; GC coordinato col
  deployment; store non montato = avvio dell'agente fallito in modo visibile, non
  fallback silenzioso; Matrix verifica dichiarazioni e byte, non il
  comportamento runtime dell'agente.

### Cosa resta al consumer

Pubblicare le generazioni con l'account publisher per ogni piattaforma,
reinstallare dai riferimenti, eseguire `matrix agent prune --apply` per
recuperare spazio, escludere `agents/` dal backup dati mantenendo `configs/` e il
Vault, ripristinare con `matrix agent restore --inventory` e verificare con
`matrix agent verify --strict`. La riduzione effettiva del picco del backup va
misurata sul deployment con `du -sb` e con il budget nativo del backup, non
dedotta da questa nota.
