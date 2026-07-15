# Issue: errore ACP generico nasconde crash `codex-acp` e warning vault bbolt

Data: 2026-06-24

Versione MATRIX: `0.1.21`

Commit: `cbc43d57ca7e4a82e45d516b9d59a52a220ec570`

## Contesto

Half Pocket Desk invia richieste a MATRIX tramite `POST /v1/runs` con
`agent_id=codex`, `execution_mode=async`, `workspace_path=/home/jose/halfpocket`.

L'utente vedeva in dashboard:

```text
[agent_preflight_failed] agent provider preflight failed agent=codex protocol=acp phase=initialize: ACP initialize failed: client context cancelled
```

## Problema 1: errore utente troppo generico

Il run falliva in fase `initialize`, ma la causa reale era solo nel log runtime:

```text
agent stderr:
Error: error loading config: /home/jose/.codex/config.toml:7:16:
unknown variant `default`, expected `fast` or `flex`
```

La riga problematica era:

```toml
service_tier = "default"
```

`codex-acp` usciva subito, MATRIX riceveva EOF sul trasporto stdio e riportava
solo `client context cancelled`.

### Riproduzione

1. Impostare in `~/.codex/config.toml`:

   ```toml
   service_tier = "default"
   ```

2. Avviare MATRIX.
3. Lanciare una run Codex:

   ```bash
   python3 apps/half-pocket-desk/scripts/matrix-halfdesk-run.py \
     --input "ping diagnostico: rispondi solo ok" \
     --workspace-path /home/jose/halfpocket \
     --no-tb-agent
   ```

4. Il run risponde con `agent_preflight_failed`, mentre il dettaglio utile resta
   nel log `matrix-runtime.jsonl`.

### Comportamento atteso

Quando il provider ACP esce durante `initialize`, MATRIX dovrebbe includere nel
payload di errore almeno uno stderr sanitizzato e breve, per esempio:

```json
{
  "code": "agent_preflight_failed",
  "phase": "initialize",
  "provider_stderr": "error loading config: ~/.codex/config.toml:7:16: unknown variant `default`, expected `fast` or `flex`"
}
```

Così dashboard e operatori capiscono subito che non e' un timeout generico ma
un errore di configurazione Codex.

### Workaround verificato

Rimuovere `service_tier = "default"` da `~/.codex/config.toml`. Dopo la
rimozione, una run MATRIX minima con Codex ha completato correttamente con
output:

```text
ok
```

## Problema 2: timeout apertura vault bbolt durante diagnostica

Durante la diagnosi, alcuni comandi MATRIX CLI fallivano o producevano warning:

```text
vault error: [ERR_VAULT_OPEN] Failed to open bbolt database: timeout (op: bolt.NewProvider)
```

Esempi:

```bash
matrix agent show codex
matrix agent args list codex
matrix logs tail -n 220
```

`matrix doctor` indicava inoltre:

```json
{
  "storage": {
    "schema": {
      "status": "unavailable",
      "error": "[ERR_VAULT_OPEN] Failed to open bbolt database: timeout (op: bolt.NewProvider)"
    }
  }
}
```

`lsof` mostrava il daemon `matrix run` con il vault aperto in scrittura:

```text
matrix <pid> mem-W /home/jose/.local/share/matrix/data/matrix-vault.db
matrix <pid> 6uW  /home/jose/.local/share/matrix/data/matrix-vault.db
```

### Comportamento atteso

I comandi diagnostici read-only dovrebbero evitare timeout frequenti sul vault,
per esempio con una o piu' di queste strategie:

- usare il daemon gia' attivo come broker per letture SSOT;
- aumentare/retryare il timeout su aperture read-only;
- distinguere chiaramente "daemon tiene lock" da "vault corrotto/non
  disponibile";
- permettere a `matrix logs tail` di leggere il file log senza aprire il vault,
  almeno quando il path log di default esiste.

## Priorita suggerita

Media.

Il problema ACP blocca completamente le run Codex ma ha workaround locale.
Il problema vault rende piu' difficile la diagnostica proprio quando il run
fallisce.

## Risoluzione maintainer

- Decisione: chiusa il 2026-07-15 come `superseded/split`.
- Problema 1: risolto da Matrix `v0.1.22`; il provider canonico viene
  installato nel PAL, la readiness esegue un handshake ACP bounded e i crash
  stdio conservano exit code e stderr breve sanitizzato. Evidenza completa in
  `issues/closed/2026-07-15-codex-acp-provider-bootstrap-follow-up.md`.
- Problema 2: ancora attivo e trasferito senza perdita di contesto in
  `issues/2026-07-15-vault-readonly-cli-timeout-with-running-daemon.md`.
- Motivo della chiusura: questa issue combinava ormai un incidente provider
  legacy e un difetto Vault indipendente; mantenerli insieme rendeva ambiguo
  lo stato e i criteri di accettazione.
