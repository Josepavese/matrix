---
description: Politica di governance del codice per contenere la crescita del progetto e aumentare qualità, manutenibilità e leggibilità.
---

# Code Governance Workflow

Questo workflow definisce la politica di crescita del codice di Matrix V2. L'obiettivo non è essere rigidamente punitivi, ma impedire che il progetto cresca senza controllo e che la qualità degradi nel tempo.

La piattaforma ha due livelli distinti:

- `code_governance.go` governa dimensione, complessità e budget architetturali.
- `golangci-lint` governa correttezza, robustezza, stile e manutenibilità operativa.

## Obiettivi

- Limitare la crescita incontrollata del codice.
- Favorire funzioni, file e package leggibili.
- Rendere evidente quando una modifica aumenta il debito strutturale.
- Spingere verso qualità e manutenibilità, non verso il rispetto cieco di numeri arbitrari.

## Policy

La policy è codificata in:

- `matrix-v2/code-governance.toml`
- `matrix-v2/scripts/code_governance.go`
- `matrix-v2/.golangci.yml`

Il principio è semplice: ogni strumento deve avere un ruolo preciso. I budget strutturali non devono essere duplicati nel linter, altrimenti si creano segnali ridondanti e si abbassa la qualità del feedback.

### Budget hard

I budget hard bloccano la modifica quando superati:

- LOC package di produzione
- LOC file di produzione
- LOC per funzione
- numero di parametri per funzione

I test sono trattati separatamente per evitare di penalizzare l'aggiunta di copertura.

Gli override sono consentiti solo per debito legacy esplicito e tracciato. Non sono una scorciatoia: devono essere pochi, motivati, stretti e temporanei.

La policy attuale mira a questo profilo alto:

- package di produzione: `800` LOC
- file di produzione: `280` LOC
- funzione: `55` LOC
- parametri per funzione: `4`

L'unica eccezione strutturale ammessa è il package `cmd/matrix`, che funge da aggregatore CLI e ha quindi un budget di package separato. Non ci sono override puntuali su file o funzioni.

### Metriche qualità

Le metriche qualità non sono pensate solo per bloccare, ma per guidare refactor e design:

- media LOC per funzione
- p95 LOC per funzione
- branch points per funzione
- rapporto di funzioni troppo lunghe

Soglie attuali:

- media funzione: `<= 24`
- p95 funzione: `<= 50`
- branch points: `<= 10`
- long function ratio: `<= 0.08`

Se queste metriche peggiorano sensibilmente, la modifica va rifattorizzata anche se i budget hard non sono ancora formalmente superati.

### Linter

`golangci-lint` è configurato in modo severo per:

- error handling
- coerenza stilistica
- bug-proneness
- leggibilità

Il linter non sostituisce il giudizio tecnico, ma alza la baseline minima di qualità. La complessità strutturale resta nel layer governance, non nel linter.

## Come rispettare la policy

1. Tenere piccole le funzioni.
   Se una funzione supera il budget, estrarre responsabilità e semplificare il flusso.

2. Tenere piccoli i file.
   Se un file cresce troppo, separare helper, adapter, builder o provider.

3. Tenere coesi i package.
   Se un package cresce troppo, introdurre sottodomini più chiari.

4. Evitare firme troppo larghe.
   Se una funzione ha troppi parametri, introdurre struct di input o separare i casi d'uso.

5. Usare i warning di qualità come segnale progettuale.
   Un warning non va ignorato automaticamente; va giustificato o corretto.

6. Non allargare i budget per comodità.
   Se una modifica richiede più spazio, prima si dimostra perché il design corrente non basta.

7. Trattare gli override come debito da rimuovere.
   Ogni override in `code-governance.toml` deve avere un piano di rientro, non diventare default permanente.

## Come testare

Eseguire sempre questi comandi da `matrix-v2/`:

```bash
go run ./scripts/code_governance.go --config code-governance.toml
go run github.com/golangci/golangci-lint/cmd/golangci-lint@v1.64.8 run
go test ./...
```

## Criterio decisionale

Una modifica è accettabile quando:

- non rompe i test
- non viola i budget hard
- non introduce warning qualitativi gravi senza motivo
- non peggiora chiaramente leggibilità e manutenibilità

Se il requisito di prodotto impone più codice, prima si aggiorna la policy in modo esplicito e motivato, poi si allarga il budget. Non si aggira il sistema.
