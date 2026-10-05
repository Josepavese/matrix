# EP-04 — `matrix vault get` su field structured: errore d'uso del getter CLI

> Nota agente esterno (MiniMax M3, `MiniMax-M3`), 28/09/2026. Nessun file Matrix al di fuori di `issues/usage-feedback-halfpocket/` è stato toccato. Nessuna patch, nessun commit, nessuna azione mutante su provider/runs/daemon/servizi.

**Stato**: `certificato` (messaggio di errore come superficie) + `suggestion` (evoluzione CLI).
**Severità**: bassa su singolo comando, sintomo di CLI string-getter applicato a record typed. NON corruzione del Vault SSOT.

## Sintesi

Post-completion, comando:

```
matrix vault get runtrace.run.<id>
```

emette `ERR_VAULT_PARSE object cannot unmarshal Go string` sul campo strutturato. La CLI usa un getter stringa su un record typed (object); il getter non converte. Il Vault SSOT NON è corrotto: l'errore è del **modo d'uso del getter CLI**, non del dato.

## Cosa NON provare in questo task

- Decrittazione.
- `vault read`/estrazione chiavi.
- Dump DB.

## Suggerimenti (con priorità e criteri di accettazione)

| ID | Suggerimento | Priorità | Criterio di accettazione misurabile |
| --- | --- | --- | --- |
| EP-04.A | Endpoint/CLI bounded final-only content resolver per `summary_ref` | **P1** | `matrix vault summary <id>` ritorna solo la fase terminale (campo `summary` referenziato), con size cap esplicito (es. ≤ 32 KiB) e codice errore dedicato se la dimensione supera il cap. |
| EP-04.B | Separare `result` (diagnostico) da `summary` (referenziato) | **P2** | `result` non include transcript provider completo; `summary` contiene solo blocchi referenziati per la fase terminale. CLI li espone con getter distinti. |
| EP-04.C | Schema multi-livello: `help` prima, poi accesso per-field | **P2** | `matrix vault get runtrace.run.<id> --help` elenca i field disponibili e il tipo di ognuno PRIMA di emettere l'errore di parse. |
| EP-04.D | Documentare i getter attuali come limitati per discovery | **P3** | Pagina docs (`docs/`) dichiara esplicitamente: "questi getter sono string-oriented; per record typed usare l'endpoint bounded o un getter typed dedicato (futuro)". |
| EP-04.E | Size cap per accesso per-field | **P3** | Qualsiasi getter CLI che attraversa record typed applica un cap configurabile e restituisce errore esplicito se superato, senza abort del processo. |

## Evidenze

- Trace Matrix osservati dopo completion di run DeepSeek/GLM/M3.
- Source Matrix: solo CLI getter (`cmd/matrix/vault.go`); main del workspace letto mirato, non transcript.
- Versione Matrix: `matrix 0.1.46`, commit `f0b97ab35e761cad6721a1ab985d3b51005f3e8e`. HEAD sorgente `2864953`.

## Follow-up

Nessuno in questo task.
