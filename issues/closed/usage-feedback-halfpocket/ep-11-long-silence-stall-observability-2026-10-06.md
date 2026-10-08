# EP-11 — Silenzio prolungato e diagnosi di attesa ambigua

Nota di agente esterno, Codex PM, 6 ottobre 2026. Solo questa nota in
issues/ e' stata aggiunta a Matrix, senza commit; codice, installazione e
servizio non modificati. Stato: osservazione certificata, attribuzione
Matrix/provider/LLM **non determinata**; proposta P2, non bug provato.

## Ambiente e prove minime

- Installato Matrix0.1.51, commit7854f5b15c61938c1d597368a5704ccc1527488d,
  build2026-10-02T08:18:28Z. Sorgente consultato HEAD5f45cac, distinto dal binario.
- Run C: run-8b0f0404-5191-42db-92d5-0a635d2904d7, MiMoCode/ACP,
  xiaomi/mimo-v2.6-pro, provider_confirmed, remote
  ses_ffe5eee136d71ffejxooNNvGpP. Avvio19:08:36UTC.
- Dopo timeout del listener, trace mostra ancora running; ultimo evento
  operativo read_file19:14:56UTC, pending shell19:10–19:14, waiting=tool_result,
  window_truncated=true. Sessione remota waiting=unknown; sintesi aggregata
  attende tool_result. Session inspect status=active non prova progresso.
- Riscontri indipendenti dopo oltre un'ora: worktree senza nuovi commit/WIP
  o verbale; nessun nuovo verifier atteso nell'origine VPS; nessun processo
  installer remoto. Processo persistente MiMo presente, ep_poll, nessun figlio
  operativo osservato. **Non** prova processo morto, deadlock o quota.
- Cancellazione puntuale tramite POST /v1/runs/<id>/actions action=cancel:
  provider cancellation signal sent, cancelled; evento terminale3728.
  Nessun kill, riavvio, transcript o segreto acquisito. Subentro soltanto
  dopo il terminale. Un altro run MiMo ha poi completato normalmente3729:
  nessuna generalizzazione sull'intero provider.

## Impatto e decisioni

Il PM non puo' distinguere dalle sole osservazioni thinking lento, retry,
trasporto, tool pendente o runtime fermo. Il campo pending storico in una
finestra troncata non deve diventare una falsa diagnosi. Il caso e' collegato
alla lacuna gia' dichiarata nell'indice del 2 ottobre: non e' una nuova
pretesa di causalita' su Matrix. Il timeout dell'osservatore, da solo, NON e'
stato usato per il subentro; dati/pending/worktree/servizi sono preservati.

## Suggerimento e accettazione

Esaminare quanto gia' consentono le capacita' native (compreso
activity_timeout_seconds, senza nuovo bridge) e documentare il significato
di attivita', pending, unknown e window_truncated. Se il provider non espone
una causa, restituire unknown, non dedurla dal testo o dall'eta' del run.
Un eventuale heartbeat metadati deve distinguere trasporto vivo e progresso
operativo; non importare ragionamenti/transcript nell'evento terminale.

Accettazione: banco causale con tool.result, silenzio lungo e processo ancora
presente produce una diagnosi delimitata/unknown; tool completati fuori
finestra non sono certificati come ancora pendenti; cancel resta puntuale,
con terminale recuperabile dal cursore. Nessuna implementazione richiesta o
eseguita qui: manutentori decidono sul proprio repository.

## CORREZIONE PM — 6 ottobre 2026, perimetro dell'oracolo artifact

Il controllo dell'origine usava packages/mail.server, non il namespace
effettivo releases/verification-repairs. L'elenco vuoto non dimostra assenza
globale del verifier; ritirata quella parte forte della diagnosi. Silenzio
operativo/worktree senza consegna/no installer remoto restano riscontri,
ma non attribuiscono una causa Matrix o provider. Il verifier byte-identico
pubblico e' stato successivamente trovato/qualificato dall'esecutore C.
Questo e' un errore di perimetro del caller, non una regressione Matrix.


## Decisione del manutentore — 2026-10-08

Corrette correlazione per sessione, completezza della finestra e causa unknown; avviso opzionale senza cancellazione automatica.

Scheda gestita e archiviata. [Decisione completa, motivi ed evidenze](../../../docs/governance/issue_resolution_2026-10-08.md).
Lo stato di pubblicazione e installazione è nel verbale della nuova release;
le evidenze del reporter sopra restano lo snapshot originale.
