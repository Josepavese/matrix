# Issue: CLI read-only in timeout sul Vault mentre il daemon e' attivo

Data: 2026-07-15

Versione verificata: `v0.1.22`

Commit verificato: `8ea0a0a3ad96a97e2fc842115879242c060099c1`

Priorita suggerita: media

## Sintesi

I comandi CLI di ispezione possono fallire dopo circa un secondo quando
`matrix run` mantiene aperto il database bbolt del PAL:

```text
vault error: [ERR_VAULT_OPEN] Failed to open bbolt database: timeout
(op: bolt.NewProvider)
```

Questa issue conserva il problema Vault ancora attivo estratto da
`issues/closed/2026-06-24-codex-acp-preflight-and-vault-timeout.md`. La parte
Codex ACP di quella issue e' stata risolta e rilasciata in `v0.1.22`.

## Evidenza corrente

Con il daemon locale attivo, Matrix `0.1.21` continua a riprodurre il problema:

```text
matrix agent show codex  -> ERR_VAULT_OPEN
matrix logs tail -n 5    -> ERR_VAULT_OPEN
```

L'ispezione del codice `v0.1.22` conferma che il difetto non e' stato risolto
dal rilascio Codex ACP:

- `matrix agent show` usa `NewAgentContext`, che apre `bolt.NewProvider` in
  scrittura;
- `matrix logs tail` carica prima la configurazione tramite
  `NewReadOnlyProvider`, ma l'apertura bbolt cross-process puo' comunque
  scadere mentre il daemon possiede il lock;
- i comandi di sola lettura non hanno un broker daemon o un fallback che eviti
  l'apertura del Vault conteso.

## Impatto

- La diagnostica fallisce proprio mentre il runtime e' attivo.
- Un timeout del lock puo' essere confuso con corruzione o indisponibilita' del
  Vault.
- `logs tail` non riesce a leggere il log predefinito anche quando il file e'
  disponibile senza consultare il Vault.

## Comportamento atteso

- I comandi di ispezione completano mentre il daemon e' attivo senza tentare
  una seconda apertura in scrittura.
- Le letture SSOT usano un percorso concorrente sicuro: broker del daemon,
  snapshot/API locale o altra strategia coerente con bbolt.
- `matrix logs tail` puo' usare il percorso log PAL predefinito senza aprire il
  Vault; una configurazione custom deve produrre un fallback o un errore
  esplicito e azionabile.
- Un lock posseduto dal daemon viene distinto da Vault corrotto o mancante.

## Criteri di accettazione

- Test MATRIX-owned con daemon/Vault writer attivo e processo CLI separato.
- `matrix agent show codex` e `matrix agent args list codex` completano senza
  `ERR_VAULT_OPEN` durante `matrix run`.
- `matrix logs tail -n 5` legge il log predefinito durante `matrix run` senza
  attendere il timeout bbolt.
- Nessun secondo writer, copia ad hoc del Vault o bypass del PAL.
- `matrix doctor` descrive esplicitamente un eventuale lock runtime e non lo
  presenta come corruzione.
