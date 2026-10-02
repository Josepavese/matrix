# Reversioni — registro agenti (hot-enable)

Metodo del repo: si copia il file, si applica la reversione, si compila, si esegue
il test, si ripristina **dalla copia** (mai con `git`). Le reversioni V1–V3 stanno
tutte in `registry.go`.

| nome | cosa reverte | test che cadono |
|---|---|---|
| V1 | il miss non rilegge il vault (`Get` risponde "non trovato" dalla vista vecchia) | `TestAnAgentEnabledAfterTheDaemonStartedIsServedNotRefusedAsUnknown`, `TestTheRunPathResolvesAnAgentEnabledAfterStartup`, `TestAnAgentEnabledAfterTheDaemonStartedIsNotRefusedWithA409` |
| V2 | TTL ignorato: la vista in cache non si rinfresca mai | `TestAnEnabledAgentAppearsInTheViewWhenTheSnapshotExpires` |
| V3 | errore di rilettura inghiottito (vault illeggibile letto come "agente inesistente") | `TestRegistryKeepsTheAgentsItHasWhenAReloadFails` |

V1 è la reversione che riporta il **409**: il terzo test è nella run API e misura la
superficie che l'operatore ha incontrato, quindi cade insieme agli altri due invece
di passare per il motivo sbagliato.

## Che cosa chiude, e che cosa no

Il registry era una fotografia scattata all'avvio: `matrix agent enable` scrive nel
vault, e il demone continuava a rispondere da una mappa che non conosceva l'agente.
La run API chiede al resolver se l'agente parla ACP — resolver che nel demone è il
supervisor, che legge questo registry — e la risposta "non ACP" era il 409 con la
remedìa "riavvia il demone".

Adesso la vista si rilegge quando il miss la smentisce (immediato: una run può
arrivare un istante dopo l'enable) e comunque oltre `registryTTL` (1 s). Misurato:
l'agente abilitato dopo l'avvio viene risolto `acp/stdio` e la richiesta di run
riceve **201**, non 409.

| modo dell'agente | chi avvia il figlio | coperto dal rimedio |
|---|---|---|
| `acp` + `stdio` (tutti gli agenti di serie: claude, gemini, kimi, opencode) | il router, per ogni run | **sì**, entro 1 s |
| `acp` + `ws`/`http` (supervisionato) | il watchdog avviato da `StartAll`, **solo all'avvio** | **no** |

Limite dichiarato, con i numeri: un agente **supervisionato** abilitato a demone
acceso resta senza figlio, perché `StartAll` gira una volta sola e nessuno
riconcilia. Il rimedio non è una riga: `StartAll` oggi non è idempotente
(`startSupervised` lancia un watchdog senza guardare se l'agente è già tracciato),
quindi una riconciliazione richiede un marcatore "già avviato" e una cancellazione
per-agente per fermare i watchdog degli agenti disabilitati — cioè il ciclo di vita
della supervisione, che è la parte che tiene su il demone. Con i dati di questo
repo la lacuna non tocca nessun agente di serie: tutti e quattro sono `stdio`.

Limite della prova: nessun test avvia un demone vero su porte temporanee, quindi la
catena è provata a tre livelli (registry → resolver del supervisor → run API con
201) e non con un demone in esecuzione.
