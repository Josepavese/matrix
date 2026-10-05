# EP-03 — Workspace_id inventati → fail pre-provider; correzione path-only / `workspace add`

> Nota agente esterno (MiniMax M3, `MiniMax-M3`), 28/09/2026. Nessun file Matrix al di fuori di `issues/usage-feedback-halfpocket/` è stato toccato. Nessuna patch, nessun commit, nessuna azione mutante su provider/runs/daemon/servizi.

**Stato**: `suggestion` evolutiva; `caller_error` per i primi due run (PM ha ammesso path inventato); `ipotesi` per `54f552c5…` (NON bug Matrix provato).
**Severità**: media. Branch isolation a rischio quando l'orchestrazione passa solo `workspace_id`.

## Sintesi

I primi run GLM/M3 con `workspace_id` inventati sono falliti prima del provider con `workspace not found`. Dopo correzione iniziale path-only, e successivamente `workspace add` + `workspace_id` esplicito con guard `pwd`/`head`, DeepSeek/GLM/MM worktree sono andati a buon fine.

## Run ID

- `41fc6d63…`, `eef05ea8e…` — workspace_id inventati (GLM/M3), `caller_error` PM.
- `54f552c5…` (MiniMax) — provider ha committato su ROOT main benché PM intendesse worktree. `ipotesi`: provider può eseguire `git -C root` indipendentemente dal `workspace_id`. NON bug Matrix provato.
- `98447c88…`, `cab3788c…` (DeepSeek) — OK con worktree esplicito.

## Suggerimenti (con priorità e criteri di accettazione)

| ID | Suggerimento | Priorità | Criterio di accettazione misurabile |
| --- | --- | --- | --- |
| EP-03.A | Confronto strutturato `requested` vs `resolved` di `{workspace_id, path, real_child_cwd, git_commonDir, branch}` | **P1** | Per ogni run, l'artefatto terminale riporta sia `requested` sia `resolved`; mismatch → fail-closed. |
| EP-03.B | Fail-closed su mismatch (no fallthrough silenzioso a root) | **P1** | Una run con `requested.workspace_id` non risolvibile NON viene eseguita; l'errore è esplicito e non degradato a root. |
| EP-03.C | Indagine dedicata su `54f552c5…` (MiniMax → commit ROOT main) | **P2** | Report separato che distingua se la causa è nel provider (esecuzione `git -C root`), nel binding HalfPocket, o in Matrix. Nessuna conclusione implicita da questo task. |

## Evidenze

- Issue pre-esistente (HalfPocket): `Matrix/issues/2026-09-27-external-workspace-grant-id-does-not-resolve-path.md` (letta, NON duplicata, NON toccata).
- Trace Matrix osservati; nessun transcript provider allegato.
- Versione Matrix: `matrix 0.1.46`, commit `f0b97ab35e761cad6721a1ab985d3b51005f3e8e`. HEAD sorgente `2864953`.

## Follow-up

Nessuno in questo task.
