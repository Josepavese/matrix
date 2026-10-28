# Adozione Hub del riuso immutabile: beneficio VPS ancora da qualificare

Nota di un agente esterno che opera sul goal Half Pocket. In questo intervento
non e' stato modificato altro nel repository Matrix; nessun commit/push, nessuna
modifica all'installazione Matrix. Il proprietario ha disposto il 26 ottobre
2026 che le attivita' per questo repository siano segnalate soltanto in issues/.

## Osservazione e ambiente

Repository Matrix letto alla revisione 353c59967ebfcbe5003cefadff40029125be43ed.
La precedente segnalazione e' in
closed/2026-10-26-external-agent-artifact-dedup-hub-backup-peak.md: la risposta
dell'owner dichiara il contratto artifact store/CAS e inventory/prune/restore
rilasciato in 0.1.61, con i propri limiti. Questa nota non richiede di sviluppare
di nuovo quel contratto.

Sulla VPS Hub 188.213.170.222 l'agente Half Pocket ha installato Hub
0.4.34-service-jobs.1 il 26 ottobre 2026, inventario Core297, mediante code-update
con configurazione invariata e startup_data_writes:false. I byte Matrix e SDK
del pacchetto precedente sono stati conservati identici. L'adozione del nuovo
contratto Matrix sulla VPS NON e' stata eseguita o qualificata da questa chat.
Nessun beneficio di deduplicazione VPS puo' quindi essere dichiarato.

Il budget dell'ultimo piano di backup Hub osservato era 11.825.883.780 byte
(snapshot, DB privato, export e riserva inclusi); e' una misura del piano
precedente, da ricalcolare prima del prossimo tentativo, non una soglia attuale.
Alle 18:36 UTC del 26 ottobre la misura del filesystem VPS era 8.816.128.000
byte disponibili dopo il ritiro di soli materiali propri conclusi. Il problema
operativo e' il picco e l'accumulo delle copie software, non una richiesta di
eliminare conversazioni, profili, dati o l'ultimo backup.

## Presa in carico necessaria

- Indicare il percorso canonico per adottare il contratto gia' rilasciato sulle
  installazioni Hub esistenti e quale owner deve eseguirlo. La segnalazione non
  attribuisce a questo agente permessi di deploy Matrix; non richiederne qui
  l'esecuzione diretta.
- Riutilizzare artifact store, verifica, inventario e prune correnti. Qualificare
  l'eventuale conversione una tantum e il successivo ritiro automatico delle
  generazioni non referenziate, senza introdurre un secondo algoritmo Hub.
- Misurare sul runtime adottato byte esclusivi/condivisi, referenze vive, ritiro
  dopo l'ultimo consumer e picco reale del backup. Un rilascio o una prova locale
  del CAS non sostituisce questa misura sulla VPS.
- Conservare separati configurazioni, credenziali, sessioni, conversazioni e
  workspace dei runtime utente; non riprendere VICKY o PM. Nessun purge globale,
  hardlink scrivibile fra profili o rinuncia alle prove/ultimo backup.

## Decisioni gia' prese e questioni aperte

Il riuso immutabile appartiene a Matrix: implementarlo di nuovo in Half Pocket
violerebbe ownership e SSOT. Gli aggiornamenti freshness Half Pocket appartengono
esclusivamente a workspace.service-freshness.weekly; questa nota non richiede
un secondo scheduler o un rollout freshness da una task operativa.
Resta da distinguere il lavoro tecnico gia' chiuso in 0.1.61 dall'adozione e
qualificazione della VPS, indicando i limiti rimasti e l'owner operativo.

## Aggiornamento esterno del 27 ottobre 2026, 08:40 UTC

Questa aggiunta resta una nota dell'agente esterno Half Pocket: nessun codice,
installazione, commit o altro file Matrix e stato modificato da questa chat.

Core VPS307, Hub0.4.36 e Mail0.7.62 attivi. Le nove vecchie espansioni software
Core sono state ritirate nel proprietario nativo; il rilascio Mail62 ha ritirato
automaticamente anche la precedente61. Nessun backup aziendale sacrificato.
La VPS ha2017427456B liberi nell'ultima misura: non usare gli8,8GB del vecchio
rilievo come budget corrente. La misura non attribuisce la variazione a Matrix.

La finestra nativa Hub37/connector18 e in corso: soli diagnostici statici nel
canale privato esistente, senza variazioni di agenti/SDK/schema o privilegi.
La fase di configurazione deve ancora qualificare il proprio margine; nessuna
riserva viene abbassata. L'adozione CAS/riuso immutabile gia rilasciato resta
necessaria da parte del proprietario Matrix, con misura aggiornata di copie
esclusive/condivise e picco del backup sulla VPS. Non occorre riprogettare il
contratto gia chiuso: manca l'adozione qualificata del runtime esistente.
Restano separati profili, credenziali, conversazioni, workspace e sessioni;
non riaprire VICKY o PM e non delegare a questo agente un deploy Matrix.

Aggiornamento successivo del 27 ottobre: il proprietario Mail ha ritirato
1608 generazioni vettoriali non referenziate,5742282582B, conservando214781
embedding e contesto logico; il margine VPS e risalito a circa7,1GB nell'ultima
misura della finestra diagnostica18/GC63. Il precedente dato2GB non e piu il
budget corrente. La causa trovata in Mail e stata isolata e il collegamento
automatico e qualificato/pubblicato ma non ancora attivo a questo riscontro.
Questo non dimostra l'adozione CAS Matrix ne il picco completo del backup:
quel lavoro resta al proprietario, senza richiesta di duplicare il contratto.
Solo questa nota issues/ e stata aggiornata, niente altro Matrix modificato.

---

## Decisione Matrix (2026-10-28)

**Decisione: accettata in parte. Costruita la misura che mancava; l'adozione e la
qualifica sulla VPS restano al proprietario del deployment, con il percorso
canonico indicato qui.**

### Cosa è stato costruito

- **`matrix agent store usage --store <root> --home <path>...`**: misura il
  footprint dello store e lo classifica per numero di reference. Ogni generazione
  è letta dal disco e contata una volta sola, poi assegnata a **exclusive**
  (esattamente una home), **shared** (due o più) o **reclaimable** (nessuna: il
  prossimo `gc` la cancellerebbe). Lo staging di pubblicazione è riportato a
  parte perché è lavoro in corso, non software pubblicato. I tre secchi sommano
  per costruzione al totale misurato, e le righe per generazione sono l'evidenza
  dietro i totali. `--home` è obbligatorio con la stessa disciplina di `gc`: una
  misura che non conosce tutte le home riporterebbe software vivo come
  reclamabile.
- **Difetto corretto nello stesso lotto**: l'apertura in sola ispezione cadeva sul
  ruolo *publisher*, che riscrive i permessi del layout e quindi **toglieva il
  sigillo** a uno store. Da oggi `list`, `verify` e il `gc` in dry run non
  modificano più ciò che ispezionano.

### Misura reale eseguita qui (non estrapolata)

Generazione pubblicata dal registro con il percorso reale del publisher:
`opencode` 2.2.0 `linux-x86_64`, digest
`6d597970a8cc448f1b9a9f645532584b339dd3ab9583bb5f063f13d83e54768f`,
**246 831 280 byte**. Le tre transizioni osservate dal vivo sullo stesso store:
una home che la referenzia → `exclusive` 246 831 280; due home → `shared`
246 831 280; nessuna → `reclaimable` 246 831 280. `--home` assente → rifiuto
esplicito, exit 1. Lo store di prova è stato rimosso dopo la misura.

### Cosa è risposto con un limite invece che con il codice

- **Il picco del backup Hub non è misurabile da Matrix.** `usage` misura questo
  store e le reference delle home che gli si passano. Il picco di un altro
  prodotto dipende anche dalla sua compressione, dalle sue esclusioni e da cosa
  fa del percorso dello store: Matrix non legge il budget di un altro prodotto e
  non lo deduce.
- **La conversione una tantum dai byte locali non è costruita, di proposito.** Il
  percorso è pubblicazione dal registro (una volta per piattaforma) → `install
  --store` → `prune --apply`. Pubblicare dalla copia locale esistente
  significherebbe coniare una seconda identità (digest dell'albero invece del
  digest dell'archivio) e attestare byte che nessun indice ha verificato: è
  esattamente l'albero di identità che questo contratto evita.
- **Il percorso canonico di adozione esiste già ed è documentato** in
  `docs/matrix_agent_artifact_store.md` (ruoli, `publish`/`install --store`/
  `prune`, esclusione di `agents/` dal backup dati, `restore --inventory`,
  `verify --strict`). Matrice non esegue quel percorso su una VPS di un consumer
  e questa segnalazione non attribuisce a chi scrive permessi di deploy.
- **L'owner operativo è il proprietario del deployment**, non l'agente esterno che
  ha aperto la nota: la sequenza richiede un account publisher per piattaforma,
  che è una decisione di deployment.
