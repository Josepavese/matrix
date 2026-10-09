# Segnalazioni Matrix

Tutte le schede aperte presenti all'avvio della revisione del 2026-10-08 sono
state gestite e conservate in [closed/](closed/).

- [Riepilogo completo, decisioni e motivi dei rifiuti](../docs/governance/issue_resolution_2026-10-08.md).
- [Inventario e hash degli originali](../docs/governance/issue_sources_2026-10-08.json).
- [Regole di gestione](../docs/governance/issue_governance.md).

Questo file è un indice, non una segnalazione aperta. Qualifiche esterne e limiti
accettati sono espliciti nel riepilogo; l'archiviazione non li certifica risolti.

## Nuovo sviluppo PAL autorizzato

Le quattro evolutive sono implementate e archiviate dopo gate locali e CI nativa Linux/Windows/macOS. La consegna release/installazione è tracciata nel [ledger PAL](../docs/governance/pal_implementation_2026-10-08.md):

La [release v0.1.53](../docs/governance/releases/2026-10-08-v0.1.53.md) è pubblicata, verificata e installata localmente. Backup, configurazioni e collaudo reale del servizio sono documentati nel verbale.

- [Sandbox e policy provider](closed/2026-10-08-pal-01-sandbox.md).
- [Capacità e ammissione](closed/2026-10-08-pal-02-capacity.md).
- [Filesystem semantico](closed/2026-10-08-pal-03-semantic-filesystem.md).
- [Telemetria e collector](closed/2026-10-08-pal-04-telemetry.md).

## Correttive successive alla revisione

- [CLI run submit: channel e invio asincrono](closed/run-submit-missing-channel.md):
  corretto per v0.1.54 con selezione esplicita di channel/workspace e test contro
  l'handler reale, inclusa la CLI compilata sulle tre piattaforme PAL.

- [matrix home su stdout](closed/home-stdout-for-shell.md): corretto l'uso nelle
  sostituzioni della shell, con verifica della CLI compilata nativa.

- [Collaudo locale senza apertura/migrazione del Vault](closed/local-installer-vault-smoke.md):
  verifica artefatto separata dai controlli del runtime avviato.
