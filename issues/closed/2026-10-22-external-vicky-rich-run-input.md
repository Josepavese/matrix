# VICKY: ingresso multimodale sul run governato e rifiuto dei campi ignorati

Nota di agente esterno Half Pocket/VICKY, 22 ottobre 2026. Questo agente ha
scritto soltanto questa segnalazione non committata in `issues/`: nessun altro
file, installazione, commit o remoto Matrix è stato modificato.

## Ambiente e bisogno reale

Consumer: verticale VICKY, backend Go e worker da SDK standard, Hub dedicato
sulla VPS. Modello selezionato nelle prove: `deepseek/deepseek-flash`, API diretta,
nessun intermediario/fallback. Il titolare richiede che il modello interpreti
anche pagine e immagini originali, non soltanto il testo prodotto da OCR.
Documentazione provider: https://api-docs.deepseek.com/guides/vision/ . Questo
supporto dichiarato non prova il trasporto completo.

VPS attuale: Matrix ufficiale0.1.56 nel pacchetto Hub0.2.6. Ispezione/probe locale
su commit `adc1bc087e1be231cf6a80fe1ab83b1d6642ddba`, Go1.27.3 linux/amd64.
Il probe non è una prova HTTP sulla VPS né una chiamata del modello.

## Evidenza nel codice e riproduzione

- Half Pocket `apps/agent-hub/internal/hub/matrix.go`, `Matrix.Start`: invia
  `/v1/runs` con `input: {text: ...}`, model pin, channel e contesto del turno.
- Matrix `internal/providers/runapi/types.go`, `runRequest`: `Input` è
  `runpayload.Input`; non c'è un ingresso di content blocks nel request.
- `internal/providers/runpayload/input.go`: `Input` è una stringa; il custom
  unmarshal dell'oggetto legge solo `text` con `json.Unmarshal`, ignorando altri
  campi. Un oggetto con `text` e `content` immagini è accettato e perde le immagini.
- `internal/providers/runapi/run_request.go`: il decoder outer non rifiuta
  i campi sconosciuti. Non è sufficiente irrigidire solo il livello outer
  lasciando permissivo il custom unmarshal annidato.
- ACP ha già `ContentBlocks`, `PromptRequest.Prompt`, controllo della capability
  `promptCapabilities.image`. A2A usa il rich router. Non ho dedotto che tale
  superficie abbia tutte le garanzie di un run Hub.

Probe compilato da una directory temporanea esterna, modulo di test con
`replace github.com/Josepavese/matrix => checkout locale`, `go run -mod=readonly`.
Nessun file di test inserito nel checkout Matrix. Usa un PNG valido generato
con la libreria standard, serializza input con testo/immagine, decodifica il
vero tipo owner e verifica il round-trip come sola stringa. Fallirebbe se il
payload fosse preservato. Risultato rc0/stderr vuoto:

```json
{"decodeError":null,"decodedTextEqualsPrompt":true,"encodedInputIsOnlyJSONString":true,"imageBytes":81,"imagePayloadAbsentAfterRoundTrip":true,"scope":"actual owner input decoder with real PNG; no HTTP run or LLM"}
```

Sorgente minimo del probe:

```go
package main
import("bytes";"encoding/base64";"encoding/json";"fmt";"image";"image/color";"image/png";"github.com/Josepavese/matrix/internal/providers/runpayload")
func main(){
 im:=image.NewRGBA(image.Rect(0,0,8,8));im.Set(3,4,color.RGBA{R:255,A:255});var pngBytes bytes.Buffer;if e:=png.Encode(&pngBytes,im);e!=nil{panic(e)}
 const prompt="Describe only the attached synthetic image"
 data:=base64.StdEncoding.EncodeToString(pngBytes.Bytes());raw,e:=json.Marshal(map[string]any{"text":prompt,"content":[]any{map[string]string{"type":"image","mimeType":"image/png","data":data}}});if e!=nil{panic(e)}
 var input runpayload.Input;if e=json.Unmarshal(raw,&input);e!=nil{panic(e)}
 out,e:=json.Marshal(input);if e!=nil{panic(e)};want,e:=json.Marshal(prompt);if e!=nil{panic(e)}
 if !bytes.Equal(out,want)||bytes.Contains(out,[]byte(data)){panic("decoder now preserves rich payload; old gap is not reproduced")}
 result:=map[string]any{"decodeError":nil,"decodedTextEqualsPrompt":input.String()==prompt,"encodedInputIsOnlyJSONString":bytes.Equal(out,want),"imageBytes":pngBytes.Len(),"imagePayloadAbsentAfterRoundTrip":!bytes.Contains(out,[]byte(data)),"scope":"actual owner input decoder with real PNG; no HTTP run or LLM"};encoded,e:=json.Marshal(result);if e!=nil{panic(e)};fmt.Println(string(encoded))
}

```

## Richiesta all'owner e decisioni già prese

Serve un contratto pubblico governato per contenuti tipizzati dei run, con
trasporto fino al prompt ACP, ammissione per capability agente/modello, byte/MIME
limitati, nessun fetch arbitrario/SSRF e nessuna perdita silenziosa. Il team
owner scelga evoluzione della superficie o un'altra API pubblica equivalente,
preservando channel/sessione, pin modello, trace, outcome, cancellazione,
deadline e idempotenza su tutto il payload (non solo il testo).

Nel frattempo un input non supportato deve essere rifiutato esplicitamente,
prima del dispatch; una risposta testuale riuscita non certifica visione.
Il contratto deve evitare di copiare byte immagine e segreti in notifiche/log
non necessari, distinguere immagine passata da modello effettivamente usato,
e consentire al consumer di legare SHA/pagina/revisione della fonte alla prova.

Decisioni già assunte nel consumer: niente chiavi DeepSeek nel verticale,
chiamate dirette a OpenCode/provider o A2A come bypass del lifecycle Hub;
niente immagini Base64 incollate nel prompt testuale e dichiarate visione;
nessun nuovo mount, trust_mode, export o lettura degli storage privati.
Il testo nativo e l'OCR restano supporti espliciti. Dati solo sintetici nelle
prove, revisione umana obbligatoria prima della pubblicazione assicurativa.
Mail e il suo WIP non sono coinvolti.

## Qualifica richiesta e punti aperti

Regressioni pertinenti del vero ingress: byte/MIME/limiti, campi sconosciuti e
duplicati, immagine non supportata, digest/idempotenza sul payload, terminale e
cleanup. Poi prova nativa Matrix→OpenCode→DeepSeek con immagini sintetiche
visivamente distinguibili e richiesta priva della risposta testuale. Il consumer
VICKY dovrà qualificare separatamente l'integrazione Hub sul pacchetto rilasciato:
né questo probe né un test ACP simulato possono chiudere quel gate.

Aperto: forma/versione dell'ingresso, proprietà e durata dei byte, segnalazione
della capacità effettiva e prova sul pacchetto ufficiale. Nessuna correzione o
release Matrix è stata effettuata da questo agente.


## Valutazione maintainer — 22 ottobre 2026

- Decisione: **accolta**, aperta per implementazione. Nessuna chiusura o release
  deriva dalla sola valutazione.
- Motivo: perdita silenziosa di contenuti e ambiguità della richiesta sono
  responsabilità Matrix. Il trasporto multimodale governato è una capacità
  generica, utile oltre il consumer segnalante e coerente con il prodotto.
- Priorità proposta: **P1** per perdita silenziosa e validazione dell'ingresso;
  evolutiva collegata per i contenuti tipizzati lungo il lifecycle dei run.
- Baseline verificata: `adc1bc087e1be231cf6a80fe1ab83b1d6642ddba`.

### Difetti confermati

Probe locale sul vero `decodeRunRequest`, compilato con overlay Go temporaneo
senza modificare sorgenti o test del checkout. Go 1.27.1, dati sintetici,
nessun servizio reale, modello o progetto esterno eseguito. Esito rc0:

- Due PNG validi e differenti, con testo identico, vengono accettati e producono
  richieste normalizzate identiche: le immagini spariscono. Di conseguenza anche
  il materiale serializzato sul quale `reserveRequestedRun` calcola il digest
  è identico; `startRun` registra un `InputDigest` basato sul solo testo.
- Campi sconosciuti esterni e annidati sono accettati.
- Chiavi duplicate `input` e `input.text` sono accettate: prevale l'ultimo valore.
- Un secondo documento JSON dopo la richiesta viene ignorato.

Le prove riguardano decoder e normalizzazione effettivi. Non certificano un
invio HTTP sulla VPS, un dispatch completo o la visione del modello.
Lo scratch e il test overlay sono stati ritirati al termine del probe.

### Perimetro accolto

1. Validazione rigorosa prima di riserva/dispatch: campi ignoti e duplicati,
   documenti JSON aggiuntivi e contenuti non supportati producono errore
   esplicito, senza eco di byte sensibili. Un controllo sul solo oggetto esterno
   non protegge il custom unmarshal annidato.
2. Contratto pubblico di input tipizzato, riusando `middleware.Content` e
   `ConversationRequest.ContentBlocks`, con una sola rappresentazione canonica.
   Il run conserva channel, sessione, modello, deadline, cancellazione, trace,
   outcome e cleanup. Nessun secondo percorso riservato a un consumer.
3. Digest e idempotenza coprono tutto il payload semanticamente rilevante,
   compresi ordine, tipo, MIME, byte e riferimenti di provenienza. Payload
   differenti sulla stessa chiave devono generare conflitto.
4. Byte, numero di elementi, MIME, dimensioni e durata hanno limiti e owner
   SSOT. Il limite attuale del body run è 1 MiB: il limite del provider non
   diventa automaticamente la policy Matrix. Niente fetch arbitrario di URL,
   letture di path privati o byte in notifiche/log ordinari.
5. Capability effettive del trasporto e del modello restano distinte.
   `promptCapabilities.image` esiste già nel backend ACP, ma da sola non
   certifica il modello selezionato né il percorso completo. Rifiuto esplicito
   quando la capacità richiesta manca; nessun declassamento silenzioso a testo.
6. Regressioni di ingresso, limiti, digest/idempotenza, ciclo terminale e
   cleanup; prova Matrix→OpenCode→DeepSeek con immagini sintetiche distinguibili
   e risposta non contenuta nel testo. Qualifica Hub/VPS separata al team esterno.

La [documentazione ufficiale DeepSeek Vision](https://api-docs.deepseek.com/guides/vision/)
è stata consultata: il supporto dichiarato del modello non risolve il difetto
Matrix e non sostituisce la prova di interoperabilità della catena selezionata.

### Confini e stato

PAL Linux/macOS/Windows e isolamento tramite contratti restano vincolanti.
Il trasporto di byte immagine non richiede accesso a device: nessun nuovo HAL,
OCR, rasterizzatore PDF o logica assicurativa entra nel perimetro Matrix.
La forma pubblica definitiva e l'eventuale custodia dei byte vanno decise
nell'implementazione, senza introdurre oggi un upload/storage parallelo.

Nessuna richiesta del report è respinta. Resta esclusa un'implementazione
specifica VICKY o un bypass tramite chiamate dirette al provider. Questa
valutazione non modifica codice, installazione, dati, sessioni o ambiente VPS.


## Implementazione maintainer — 22 ottobre 2026

Implementato nel perimetro generico Matrix; pubblicazione e installazione sono
ancora da completare nel goal corrente. Il decoder è rigoroso anche nei blocchi
annidati e nei metadati; l'input usa `middleware.Content`, passa al router senza
appiattimenti e partecipa integralmente a digest/idempotenza. Il manifest non
contiene byte o metadati arbitrari. Limiti e tipi ammessi hanno un owner SSOT.

L'ammissione verifica il router e il modello esplicito; il probe live della
capability immagine avviene nel run governato, prima del prompt, con deadline,
fallimento tipizzato e cleanup. Un replay idempotente non riavvia probe/client.
La capability del trasporto non diventa una falsa attestazione per modello:
`model_content_verification` resta `unverified`, distinta dal model pin.

Regressioni: HTTP autenticato reale in-process, chiavi ignote/duplicate/casing,
trailing JSON, MIME/base64/byte/pixel/numero di blocchi, immagini e testo,
provenienza nel digest, conflitto e replay senza nuovo probe, refusal terminale
con codice durevole, sessione effimera e cleanup. Suite PAL aggiornata per
Linux/macOS/Windows; nessuna API device o dipendenza hardware aggiunta.

Prova nativa del codice implementato tramite il nuovo test ripetibile
`TestSmokeRealRichRunVision` sul runtime candidato isolato, con OpenCode 1.18.31:

- modello richiesto/configurato/effettivo `deepseek/deepseek-flash`,
  `provider_confirmed`, nessun fallback;
- immagine sintetica senza risposta nel prompt → **Red square**,
  run `run-2605d16c-876e-4874-b54c-c632be00d9d0`;
- seconda immagine distinguibile → **blue circle**,
  run `run-eadf3344-6370-4a37-94f4-cd5bb544d8eb`;
- entrambe completate con cleanup forte; nessuna immagine nel trace;
- la prima installazione isolata ha rifiutato il run per setup non provisionato,
  prima del prompt. Il vecchio fallimento e il suo replay sono preservati;
  provisioning verificato tramite bootstrap/doctor prima della nuova esecuzione.

Il test reale richiede esplicitamente URL runtime, credenziale, agente e modello;
lo skip ordinario di CI non è presentato come qualifica reale. Nessun workflow
VICKY/VPS eseguito o modificato. La qualifica del verticale resta al team esterno.
Riferimento operativo generico: `docs/wiki/API-Reference.md`, Typed inline input.


## Chiusura maintainer — 22 ottobre 2026

**Implementata, rilasciata e installata localmente in v0.1.59.**
La decisione di apertura sopra resta la cronologia della valutazione iniziale.

CI e PAL sui contratti modificati sono verdi sulla sorgente esatta
`2f10dd98c217ad14344172514213b572661ba69b`; release e 88 controlli pubblici
passati. Upgrade isolato conserva i byte di Vault/configurazione; servizio
locale aggiornato con sette configurazioni invariate e binari installato/in
esecuzione identici all'artefatto verificato.

Anche dall'artefatto ufficiale installato le due immagini sintetiche sono
riconosciute correttamente, con modello confermato, nessun fallback e cleanup
forte. Sette negativi HTTP reali sono rifiutati prima del prompt, senza eco
input. Readiness strict: `ready`, zero warning/blocker e probe autenticato.

Dettagli, hash, run e limiti Windows esterni al delta sono nel
[ledger v0.1.59](../../docs/governance/releases/2026-10-22-v0.1.59.md).
La qualifica del pacchetto Hub/VPS resta al team esterno, senza attribuirle le
prove locali. Nessun requisito specifico del verticale entra nel prodotto.
