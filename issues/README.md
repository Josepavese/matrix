# Segnalazioni Matrix

Tutte le schede aperte presenti all'avvio della revisione del 2026-10-08 sono
state gestite e conservate in [closed/](closed/).

- [Riepilogo completo, decisioni e motivi dei rifiuti](../docs/governance/issue_resolution_2026-10-08.md).
- [Inventario e hash degli originali](../docs/governance/issue_sources_2026-10-08.json).
- [Regole di gestione](../docs/governance/issue_governance.md).

Questo file è un indice, non una segnalazione aperta. Qualifiche esterne e limiti
accettati sono espliciti nel riepilogo; l'archiviazione non li certifica risolti.

## Revisione del 22 ottobre 2026

- [Input multimodale dei run e validazione rigorosa](closed/2026-10-22-external-vicky-rich-run-input.md):
  **implementata e chiusa in v0.1.59**. Riprodotti scarto silenzioso delle immagini, identità del
  payload normalizzato per immagini diverse, accettazione di campi sconosciuti,
  duplicati e documenti JSON aggiuntivi. Priorità alla correttiva d'ingresso;
  evolutiva generica tramite i contratti rich esistenti, senza bypass del run.
  Prove native e sull’artefatto installato Matrix→OpenCode→DeepSeek positive;
  release verificata, installazione locale e readiness strict completate.

## Revisione del 21 ottobre 2026

Verificati `issues/`, `improvements/issues/`, gli audit/debiti in
`docs/governance/`, le analisi `.agent/local/project-analysis/` e le issue GitHub
(nessuna aperta). Nessun nuovo TODO/FIXME applicativo. Le decisioni storiche
restano nel registro dell'8 ottobre e nel successivo ledger PAL.

- [Consegna ACP sotto carico e durata del collegamento](closed/2026-10-21-acp-delivery-barrier.md):
  difetti Matrix riprodotti e corretti; regressioni native Linux/macOS/Windows.
- [Finale OpenCode/VICKY non consegnato](closed/2026-10-21-external-vicky-opencode-final-not-delivered.md):
  gestita nel perimetro Matrix e archiviata; prova locale positiva sulla v0.1.56.
  Adozione e collaudo VPS affidati dall'utente al team VICKY. La causa storica
  senza wire trace resta non attribuita; nessuna prova locale certifica la VPS.

- [Allineamento ACP/A2A](closed/2026-10-21-protocol-alignment.md): contratti correnti,
  verifiche di interoperabilità e rilascio nel verbale dedicato.

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
