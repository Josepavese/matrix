# EP-02 — Cwd del child ACP non propagata al processo MiMo

> Nota agente esterno (MiniMax M3, `MiniMax-M3`), 28/09/2026. Nessun file Matrix al di fuori di `issues/usage-feedback-halfpocket/` è stato toccato. Nessuna patch, nessun commit, nessuna azione mutante su provider/runs/daemon/servizi.

**Stato**: `certificato` (workaround applicato, cwd verificata Linux-only) + `suggestion` (fix strutturale fuori scope).
**Severità**: alta per l'uso con agent che richiedono `server-cwd == process-cwd`. NON prova universale Matrix (ACP workspace cwd può essere virtuale).

## Sintesi

argv `mimo acp --cwd /home/jose/halfpocket` arriva corretto a MiMo. La cwd del processo child resta `/home/jose/.local/share/matrix` (ereditata dal daemon Matrix via `os.Chdir(home)`). MiMo rifiuta con `directory_not_allowed` perché il path non è discendente della propria cwd interna; `session/new` → RPC `-32603`.

## Causa letta in Matrix (sola lettura)

- `internal/logic/matrixhome/home.go:28` — `os.Chdir(home)`.
- `internal/logic/agentmgr/supervisor.go:201` — `CommandSpec` costruito senza `Dir`.
- `internal/providers/exec/exec_unixlike.go:255` — `cmd.Dir = spec.Dir` solo se popolato.
- `internal/providers/agents/router_clients.go:292` — `effectiveCwd(workspacePath)` esiste; in `router.go:287-289` è propagato solo come `deps.Cwd`, non come `cmd.Dir`.

## Workaround nativo applicato dal PM (Linux, post-idle)

Configurazione runtime (NON wrapper su disco), via CLI `matrix agent set-binary`:

```
command = /usr/bin/env
args    = -C /home/jose/halfpocket /home/jose/.mimocode/bin/mimo acp --cwd /home/jose/halfpocket
```

`--args=-C` e `--args=--cwd` espliciti (pflag). Verifica: PID MiMo `62831`, `readlink /proc/62831/cwd` = `/home/jose/halfpocket`.

## Limiti workaround

- GNU env only (`-C` coreutils >= 8.1). Documentato come rimedio **Linux locale**. Non si propone `brew install coreutils` né wrapper `bash -c` come fallback di questa nota (fuori scope e non verificati qui).

## Suggerimenti (con priorità e criteri di accettazione)

| ID | Suggerimento | Priorità | Criterio di accettazione misurabile |
| --- | --- | --- | --- |
| EP-02.A | Knob `process cwd` separato da `session cwd` nel CommandSpec | **P1** | Dopo aver popolato solo `process_cwd` per un agent, `readlink /proc/<pid-child>/cwd` corrisponde al valore senza richiedere wrapper `env -C` né modifica di argv. |
| EP-02.B | Validazione realpath/workspace binding pre-fork | **P2** | Se `process_cwd` non risolve a un path reale o non è workspace-bound, la registrazione/`enable` fallisce con errore esplicito (NO esecuzione del child). |
| EP-02.C | `agent doctor` probe actual child identity (`readlink /proc/<pid>/cwd`), non `env --version` | **P2** | `matrix agent doctor <id>` riporta `child.cwd=<resolved>` e `child.argv=<argv reale>` letti da `/proc/<pid>/{cwd,cmdline}` per il child effettivo. |
| EP-02.D | Nessuna escalation automatica a directory globale come soluzione | **P2** | Test di regressione: con `process_cwd` invalido/non workspace-bound, il child NON viene lanciato con cwd = root/parent; il sistema rifiuta e chiede correzione. |

## Evidenze

- Log: `~/.local/share/mimocode/log/2026-09-28T082215848Z-main-47605-abc0a033.active.log`, precedente `…081916385Z-main-45363-3e8f4845.log`.
- Report: `~/halfpocket/docs/cloud-transition/evidence/2026-09-28-mimo-matrix-cwd.md` 32–92, 287–312.
- Versione Matrix: `matrix 0.1.46`, commit `f0b97ab35e761cad6721a1ab985d3b51005f3e8e`. HEAD sorgente `2864953`.

## Follow-up

Nessuno in questo task. ADR HalfPocket separato (NON incluso qui) da proporre al proprietario Matrix per fix strutturale.

---

## CORREZIONE: 2026-10-02 — il workaround è ritirato (prevale sulle righe sopra)

Nota da manutentore, non committata, come da processo di questo archivio. Le
righe storiche sopra non sono state toccate.

Il workaround `env -C` **e** il `--cwd` dichiarato sono stati **ritirati il 2
ottobre 2026**, non solo la parte `env -C`. Motivo: `declaredWorkspaceDir` legge
`--cwd` come rivendicazione di workspace (`workspaceDirFlags` lo include),
quindi lasciandolo si sarebbe continuato a far rifiutare ogni run fuori dalla
checkout principale — la stessa causa che aveva fatto nascere questa scheda.
Con la risoluzione strutturale della cwd i tre termini dell'accordo (cwd del
figlio, dichiarazione dell'agente, workspace ACP) concordano senza override.

La vostra **EP-02.A è quindi soddisfatta** come la chiedevate: il knob
`process_cwd` esiste (`MATRIX_AGENT_PROCESS_CWD` → `CommandSpec.Dir`, con
rifiuto del valore inutilizzabile e nessun fallback globale), e per il percorso
run la cwd è risolta strutturalmente, non dichiarata per comando.

**EP-02.B, C, D** risultano tutti soddisfatti; la prova di accettazione a due
worktree su mount diversi con agente reale e sessione riusata è stata eseguita
ed è riuscita (4 run `completed`). Limite dichiarato da chi l'ha eseguita:
prompi minimali, quindi avvio sessione e completamento, non un turno con
strumenti.

Aggiunta utile alla vostra scheda operativa: con `execution_mode` sincrono
predefinito la POST resta aperta per l'intera run, e un client con timeout breve
(30 s) cancella il contesto uccidendo il turno in `phase=session/prompt`. Per
orchestrazioni esterne: `execution_mode=async` + `matrix run wait`.
