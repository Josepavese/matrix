# ACP trace: risultati di lettura pubblici classificati come delete/write con summary affermative

Segnalazione esterna di Josepavese, 14 novembre 2026, issue
[#18](https://github.com/Josepavese/matrix/issues/18). Conservata verbatim; la
decisione Matrix segue in coda.

## Originale

Observed on the published Matrix 0.1.60 executable (SHA-256 d4b2f74ae425bf86efa422af73e255a2d0bc9c29be4f0ea8d4a96ad2863e9b18), with OpenCode ACP V1.18.32. Read-only source inspection at repository revision 5354642b8483ff40245b5ddaedb4bbe249c6395a shows the related fallback in internal/logic/frontendevents/tool.go and affirmative summary rules in tool_summary.go; that checkout is not claimed to be the binary's build revision.

A public documentation question executed a web reader CLI help, two public page reads and one proof lookup. All four CLI results exited 0. The owner returned status=ok, network_policy=public, kind=page_read for the reads, and one proof record for the lookup. There was no requested filesystem mutation. The same result events instead expose tool_name=read_file with heuristic_fallback/low confidence; the page reads have tool_kind=delete, tool_effect=write and summary `Deleted /workspace`, and the proof lookup has kind=edit/effect=write with `Updated /workspace` (actual workspace path redacted).

Private trace retained by the consumer; its SHA-256 is c5b981f3c5f58ed11116a331f2fbb6186d176b974d70e4c015c27bf0c772dc98. No raw thoughts, private page contents, account identifiers, credentials or customer data are included here.

Expected: prefer protocol tool identity/effect when available; missing metadata should remain explicitly uncertain instead of asserting a deletion/update from result-body words. In particular, low-confidence substring classification over arbitrary public content must not become a factual write summary. Please diagnose whether metadata is lost in ACP normalization or the fallback over-interprets content, add a regression covering read/proof results containing mutation words, and qualify/release the owning Matrix fix.

Consumer decision: continue the authorized Hub/reader integration without modifying Matrix source or relabeling these events locally. Keep semantic trace presentation as a separately recorded upstream limitation until the owner releases the correction.

---

## Decisione Matrix (2026-11-14)

**Decisione: accettata. È un difetto reale di Matrix, non un'ambiguità del
fornitore. Corretto e rilasciato in v0.1.63.**

### Diagnosi

`tool_call_update` in ACP è una patch: un campo non cambiato può essere omesso, e
`kind` è uno di questi. Matrix normalizzava ogni update per conto proprio, quindi
l'identità già dichiarata dal `tool_call` non veniva portata nel suo risultato.
Senza `kind` nella patch, il fallback scandiva `name + content` per parole
d'azione nell'ordine delle regole: un corpo di risultato che citava "delete",
"remove", "write" o "edit" selezionava l'azione, e `tool_summary.go` rendeva una
summary affermativa al passato dal primo percorso assoluto che trovava nel testo.
I metadati ACP **non** sono andati persi nella normalizzazione: non erano nella
patch, e nulla correlava il risultato alla sua chiamata.

### Cosa è stato costruito

- **Precedenza dell'evidenza, etichettata.** Il `tool_kind` di protocollo vince
  ancora. Quando manca, un'identità dichiarata del vocabolario canonico
  (`read_file`, `list_files`, `search`, `write_file`, `edit_file`, `shell`)
  decide il tipo come fallback a bassa confidenza. Le parole libere restano solo
  per le chiamate, dove descrivono ciò che si sta per tentare.
- **Il corpo di un risultato non sceglie mai l'azione.** Senza `kind` né identità
  dichiarata, un update di risultato resta `other` con
  `tool_classification_source=unknown`, `tool_effect=unknown` e
  `tool_subject_kind=unknown`; eredita l'identità della chiamata correlata
  (`tool_call_id`), che è evidenza di protocollo e non inferenza sul testo.
- **Summary al passato solo con un tipo verificato.** `Created`, `Updated` e
  `Deleted` sono resi solo per un `kind` di protocollo o un'identità dichiarata;
  un'azione non verificata o ignota mantiene la forma neutra `Completed <tool>`.
  Una scrittura reale conserva la sua summary anche se il corpo del risultato
  contiene parole come delete/remove.
- **L'evidenza euristica resta visibile come tale.** Un'identità dichiarata non è
  metadato di protocollo: l'evento resta `heuristic_fallback`/`low`, e il
  contratto del trace documenta che un risultato ignoto è `unknown`, non un
  effetto inventato dal corpo.
- **Regressioni a due livelli**: proiezione (letture di pagina e proof record con
  parole di mutazione, risultato senza identità, scrittura verificata) e notifier
  end-to-end (chiamata `read_file`, patch che omette il `kind` invariato e porta
  testo con delete/update: il trace resta `read`/`read_only`, con summary neutra e
  correlazione `tool_call_id`).

### Cosa è stato risposto con un limite, non con codice

- **Un fornitore che manda solo una patch di risultato, senza `kind`, identità né
  una chiamata precedente nella finestra, resta ignoto.** Matrix lo dice
  onestamente (`other`/`unknown`, summary neutra) invece di indovinare dal corpo:
  l'identità richiede una chiamata o metadati di protocollo, e il trace non può
  fabbricare ciò che il fornitore non ha dichiarato.
- **Un percorso citato dal testo resta elencato come evidenza estratta** in
  `inputs`/`outputs`, ma non sceglie più azione, soggetto o summary. Rifiutare
  l'estrazione dal testo eliminerebbe anche i percorsi che Matrix recupera
  legittimamente per agenti che narrano le proprie chiamate.
- **Nessuna mappatura specifica per fornitore.** Il vocabolario è quello neutro
  già documentato: nessun nome di agente, alias di framework o caso speciale di
  formato.
- **Il trace privato del consumer non viene rietichettato qui.** È la sua prova;
  questa release cambia ciò che Matrix produce da ora in poi, con una nuova
  versione immutabile e senza riscrivere byte passati.

### Prove

- Payload equivalenti a quelli osservati, prima della correzione, proiettavano
  `kind=edit`, `effect=write`, `summary="Updated /workspace/docs"` (e `Deleted`
  quando il testo citato conteneva solo delete/remove). Dopo la correzione gli
  stessi payload proiettano `read`/`read_only`/`Completed read_file`, e un
  risultato senza identità proietta `other`/`unknown`/`Completed tool`.
- Gate locali completi: `scripts/quality_gate.sh --static` (formattazione, vet,
  build, race su 93 package, coverage ratchet, golangci-lint v2.14.0,
  govulncheck, governance) e `scripts/release_gate.sh` sulla candidata.
- Rilascio v0.1.63: [note di rilascio](../../docs/governance/releases/2026-11-14-v0.1.63.md).
