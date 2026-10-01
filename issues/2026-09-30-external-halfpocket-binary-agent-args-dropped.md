# Nota esterna: `matrix install` perde gli argomenti del binario ACP

Questa è una segnalazione di un agente esterno impegnato nel collaudo Half Pocket. Non ho modificato codice, installazione, configurazioni, commit o remoto del repository Matrix; l'unica scrittura qui è questo file non tracciato in `issues/`.

## Osservazione e ambiente

- Matrix installato sulla VPS preview: **0.1.46**; sorgente locale esaminato alla revisione `2864953`. Agente `opencode` distribuito come binario Linux x86_64 **1.18.32** dal registro ACP pinnato `https://software.halfpocket.net/acp/opencode/registry.json`.
- La voce `distribution.binary.linux-x86_64` del registro dichiara `cmd: "./opencode"` e `args: ["acp"]`.
- Dopo `matrix install opencode`, `matrix agent show opencode` nella home del worker riporta `effective.command` al binario installato, ma `effective.args: []`.
- Matrix avvia quindi OpenCode senza `acp`: la preparazione di una sessione Hub termina con `session_prepare_failed`; il log Matrix riporta `agent_initialize_failed` in fase `initialize`, con `ACP initialize failed: context deadline exceeded`, e chiude il processo dopo circa 60 secondi.
- Prova discriminante nello stesso utente/ambiente della VPS: `opencode acp` risponde entro pochi secondi a `initialize` ACP sia v1 sia v2. Il binario `opencode --version` risponde `1.18.32`; non si è osservato un OOM. Il problema è quindi negli argomenti di lancio, non nell'eseguibile o nel protocollo ACP.

## Pista nel sorgente Matrix

In `internal/logic/agentmgr/registry_client.go`, `BinaryDist` conserva `Args` e il resolved distribution li espone. In `internal/logic/agentmgr/installer.go`, ramo `installResolved` per `resolved.Type == "binary"`, la `agentcfg.Config` ritornata contiene `Command`, `Kind`, `Transport`, ma **non `Args: resolved.Args`**. I rami `npx`/`uvx` li conservano. Questo spiega esattamente la divergenza osservata.

## Correzione richiesta e prova di accettazione

Preservare gli argomenti dichiarati dalla piattaforma nel config del binario installato; verificare con un test di installazione da manifest ACP con `args: ["acp"]` che `matrix agent show` riporti gli stessi argomenti e che il processo sia avviato con essi. Il test deve coprire anche il caso senza argomenti senza inventarne.

Half Pocket userà nel frattempo il comando nativo `matrix agent args set opencode -- acp` **solo quando** il read-back mostra `args: []`, con preflight e prova ACP. La riconciliazione è un presidio idempotente dell'installer Half Pocket, non una modifica di Matrix; dopo la fix a monte non dovrà duplicare `acp`.

Decisione già presa: non cambiare il registry index (è corretto), non incapsulare OpenCode in un wrapper e non modificare Matrix da questo repository. Resta ai manutentori decidere se aggiungere una riparazione dei record già installati; un fix del solo percorso di nuova installazione non aggiorna automaticamente `effective.args` dei worker esistenti.
