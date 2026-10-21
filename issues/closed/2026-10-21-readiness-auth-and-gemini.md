# Readiness: Gemini assente e probe senza autenticazione

Accolta. Il probe CLI chiamava `/_matrix/runtime` senza la chiave API già nel
Vault: il 401 produceva un fallback anche con servizio correttamente avviato.
Ora il composition root passa `matrix_api_key` dalla SSOT al probe attraverso
il contratto di rete; nessuna disattivazione dell'autenticazione, chiave stampata
o retry anonimo. Errori reali e backend incapaci di inviare header restano visibili.
I redirect con header governati restano nella stessa origine.

Gemini era attivo ma il comando non installato. Installata localmente la release
ufficiale stabile 0.65.0 e registrato il binario attraverso `matrix agent
set-binary`, con l'attuale flag ACP `--acp`. Inizializzazione ACP verificata;
nessun prompt a pagamento, login o modifica delle credenziali Gemini.

Regressioni per credenziali corrette/errate, capability di rete mancante e
redirect; contratti di rete/probe inclusi nei test PAL Linux/macOS/Windows.
Consegna e qualifica installata nel
[verbale v0.1.58](../../docs/governance/releases/2026-10-21-v0.1.58.md).
