# Grant workspace: `workspace_id` non basta con `require_grant`

Nota di un agente esterno al repository Matrix, 27 settembre 2026. Questo file
e' l'unica scrittura qui effettuata: nessun codice, configurazione,
installazione, commit o push di Matrix e' stato toccato.

## Ambiente e revisione

- Matrix installato 0.1.46, commit `f0b97ab35e761cad6721a1ab985d3b51005f3e8e`.
- Workspace Matrix `halfpocket` con `root_path=/home/jose/halfpocket`.
- Grant `POST /v1/workspace-grants`: repository esatto
  `/home/jose/halfpocket`, `include_git_worktrees=false`, TTL 3600 s;
  `GET /v1/workspace-grants` lo mostra attivo.
- Agente richiesto `opencode`, modello `minimax-coding-plan/MiniMax-M3`,
  `execution_mode=async`, `workspace_policy=require_grant`.

## Osservazione riproducibile

`POST /v1/runs` con `workspace_id=halfpocket` e senza `workspace_path`
fallisce in preflight con `matrix_workspace_not_granted`. Il trace del run
`run-52445434-41f9-49f1-973c-20a97bcea66a` espone la causa:
`workspace path must be absolute`. Il workspace registrato ha pero' gia' una
root assoluta e il grant la copre. Una richiesta successiva con lo stesso
workspace e `workspace_path=/home/jose/halfpocket` esplicito ha superato il
preflight ed e' entrata in `running` (`run-ae676364-f562-4222-8f10-1d0dec087e48`).

Il prompt non e' stato consegnato al provider nei run rifiutati; non attribuire
l'errore a OpenCode, al modello o ai permessi del repository.

## Contratto atteso e decisioni

La documentazione `docs/wiki/API-Reference.md` presenta `workspace_id` e
`workspace_path` come parametri distinti opzionali e dice che `require_grant`
verifica il root esatto o un worktree collegato. Si chiede di risolvere il
percorso canonico dal workspace registrato prima del controllo grant, oppure
di documentare esplicitamente l'obbligo di inviare anche `workspace_path`.
Un test dovrebbe provare i due ingressi sul medesimo grant e il rifiuto di un
path esterno. Nel consumer Half Pocket il workaround e' passare entrambi i
campi; non si disabilita `require_grant` e non si modifica Matrix da qui.
