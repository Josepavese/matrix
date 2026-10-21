# Allineamento ACP e A2A — 21 ottobre 2026

Accolta e implementata nel perimetro delle capacità Matrix. Fonti ufficiali e
limiti nella [copertura canonica](../../docs/protocol_coverage.md).

- ACP stabile: wire 1/schema 1.26.0. Notice effimere, compattazione con snapshot
  separati dalla risposta, presenza/null/append/replacement e osservazione fra
  turni. Richieste operative validate prima di scritture, processi e permessi.
- ACP v2 draft alpha.9: acknowledgement con messageId, foreground esplicito,
  session-info con cancellazioni esplicite fino allo specchio neutrale,
  replay idle senza falsa conclusione, errori/budget senza falso successo,
  drenaggio delle notifiche ricevute, patch dei metadati per entità. I campi di
  identità/classificazione Matrix non sono sovrascrivibili da estensioni peer.
- A2A protocollo 1.0.1/SDK 2.7.0: controlli versione, negoziazione media type,
  input conformi alla card, JSON trailing rifiutato, timestamp UTC e scopes
  ProtoJSON. Rimossi traduttore A2A 0.3 e workaround dei task terminali ora
  gestiti dall'SDK; resta la policy Matrix di un turno attivo per task.
- PAL Linux/macOS/Windows, contratti neutrali e stato posseduto dalle rispettive
  connessioni; nessuna migrazione del Vault o logica applicativa VICKY.

Non aggiunti: listener gRPC, ACP Streamable HTTP, MCP-over-ACP, NES/editor e
servizio di firma/fiducia JWS. Sono capacità opzionali non pubblicizzate che
richiedono un contratto di prodotto dedicato, non lacune dei binding esposti.
ACP v2 resta draft: mock stdio conformi non dimostrano supporto commerciale.

Prove e consegna nella [release v0.1.57](../../docs/governance/releases/2026-10-21-v0.1.57.md).
