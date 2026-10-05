# EP-06 — Model request vs effective vs provider_actual

> Nota agente esterno (MiniMax M3, `MiniMax-M3`), 28/09/2026. Nessun file Matrix al di fuori di `issues/usage-feedback-halfpocket/` è stato toccato. Nessuna patch, nessun commit, nessuna azione mutante su provider/runs/daemon/servizi.

**Stato**: `certificato` (sequenza osservata limitata a quanto sotto) + `suggestion` (tracciato multilivello).
**Severità**: bassa governance, media debug "modello sbagliato".

## Sintesi

Dopo run DeepSeek `98447c88…` e `cab3788c…`: `model_request=deepseek/deepseek-flash`, `model_effective=deepseek/deepseek-flash`, `provider_confirmed`. Verifica PM sulla cache OpenCode LETTA localmente (workstation): `api=https://api.deepseek.com`, pacchetto `@ai-sdk/openai-compatible`, `model_name=DeepSeek V4.1 Flash`.

## Cosa è attestato vs cosa no

- **Attestato**: i campi `model_request`/`model_effective` riportati dal runtime Matrix per le run citate e la cache provider LETTA in locale sulla workstation del PM.
- **NON attestato**:
  - Nessuna promessa su alias legacy (`deepseek-v4-flash`, `deepseek-flash`) o fatturazione `DeepSeek-V4.1-Flash` è parte della prova di questo task. La cache locale non include questa informazione, e il singolo run non la richiede.
  - Nessuna intercettazione TLS né prova di vendor modello fisico raw.
  - NON OpenRouter (verifica negativa implicita: cache locale non contiene override `deepseek` globale e nessuna run è passata per OpenRouter).

## Suggerimenti (con priorità e criteri di accettazione)

| ID | Suggerimento | Priorità | Criterio di accettazione misurabile |
| --- | --- | --- | --- |
| EP-06.A | Tracciato multilivello: `advertised` (datasheet) / `selected` (Matrix) / `provider_actual` (opzionale) | **P1** | Per ogni run, l'artefatto terminale riporta almeno `selected`; quando il provider supporta `provider_actual`, questo è valorizzato; mismatch tra `selected` e `provider_actual` è esplicito (NON implicito). |
| EP-06.B | Provider host metadata redatto di default (no header value) | **P2** | `provider.host` viene loggato come redatto (es. `api.deepseek.com` → `***`) salvo opt-in esplicito del PM; nessun header value è loggato. |
| EP-06.C | Policy fail-closed su fallback di modello | **P2** | Se `selected != provider_actual` per fallback automatico, la run termina con errore esplicito, NON con un modello diverso silenziosamente. |
| EP-06.D | Nessuna API key nello store Matrix | **P1** | Verifica che `matrix vault` (store locale) NON contenga `api.deepseek.com` key né alcun segreto provider; ADR 0003 (cloud Vault/refresh owner semantics) resta distinto e invariato. |

## Evidenze

- Report HalfPocket: `~/halfpocket/docs/cloud-transition/evidence/2026-09-28-deepseek-opencode-policy.md` 60–123, 141–148.
- Versione Matrix: `matrix 0.1.46`, commit `f0b97ab35e761cad6721a1ab985d3b51005f3e8e`. HEAD sorgente `2864953`.

## Follow-up

Nessuno in questo task.
