# Nota esterna: MiMo ACP session/new fallisce su nuovo worktree

**Decisione Matrix (2026-10-01): chiusa nel perimetro di test.**
Matrix valida e normalizza il workspace PRIMA del provider: una run verso un workspace
non registrato e' rifiutata con `matrix_workspace_not_found` (fase
`matrix.workspace_preflight`) invece di proseguire al buio, e l'identita' del
workspace (id E path) e' risolta prima di accettare la run
(`internal/providers/runapi/run_workspace_resolution.go`, `ResolveIdentity`). La
diagnostica ACP originale e' esposta invece di `Internal error (map[])`:
`providerfailure` porta `rpc_error_code`/`rpc_error_message`/`rpc_error_data` (troncato
a 512 byte e marcato) e `failureReason` classifica `provider_rpc_error`. Nessun
fallback silenzioso ne' di modello ne' di sessione: l'unica autorizzazione al fallback
resta `fallback_model_id` diverso. Verifica: test del percorso HTTP piu' verifica
indipendente.

NON fatto: la riproduzione reale chiesta (una `session/new` MiMo su worktree Git con
file `.git` sotto `/media/jose/Data`, con diagnostica ACP redatta) e la conseguente
attribuzione esplicita causa Matrix / causa MiMoCode. Senza i log MiMo e il runtime
dell'operatore quella distinzione non si puo' stabilire e non e' stata inventata.

Nota di un agente esterno al repository Matrix. Nessun codice, installazione o commit di Matrix è stato modificato. È stata registrata soltanto la workspace operativa `halfpocket-mvp-auth-release-20261001` per il secondo tentativo, senza cambiare agenti/provider o sessioni esistenti.

## Osservazione e ambiente

Il 1 ottobre 2026, Matrix 0.1.46 (`f0b97ab35e761cad6721a1ab985d3b51005f3e8e`) ha accettato i POST asincroni per `agent_id=mimo`, `model_id=xiaomi/mimo-v2.6-pro`, ma due tentativi di creare una nuova sessione ACP sul worktree Git `/media/jose/Data/halfpocket-auth-release-20261001` sono terminati prima dell'esecuzione del provider con:

`[agent_preflight_failed] agent provider preflight failed agent=mimo protocol=acp phase=session/new: ACP new session failed: RPC error -32603: Internal error (map[])`

Run ID: `run-28bd4b13-237a-4c74-a831-96aa49102b30` (senza workspace ID) e `run-5473b49a-1c64-4686-90cb-047eea3807b1` (dopo `matrix workspace add` per il medesimo path). In entrambi: `effective_model=null`, `model_verification=unverified`, `remote_session_id=null`; nessuna modifica risulta da quei run. Il worktree esiste, è un checkout Git valido e disponeva di spazio su Data.

## Confronto utile

Una risposta contestuale sulla sessione MiMo esistente, con workspace path `/home/jose/halfpocket` e istruzione di scrivere solo nel worktree isolato, è partita: `run-c17d41b5-5a17-4e46-8dd9-97e9f26f59b2`, `effective_model=xiaomi/mimo-v2.6-pro`, `model_verification=provider_confirmed`, `remote_session_id=ses_ffe5f0a091c72ffeyk5AuJvReR`. Questo indica che il provider e la quota erano disponibili; non identifica ancora se il difetto sia nel nuovo cwd, nel client MiMo ACP o nella gestione Matrix della sessione nuova.

## Richiesta ai manutentori

Riprodurre una `session/new` MiMo su un worktree Git con `.git` file e cwd sotto `/media/jose/Data`, acquisendo diagnostica ACP redatta senza segreti. Chiarire se Matrix debba validare/normalizzare il workspace prima del provider o esporre l'errore MiMo originale al posto di `Internal error (map[])`. Distinguere esplicitamente causa Matrix da causa MiMoCode; non introdurre fallback silenzioso di modello o sessione.

Decisioni già prese: non sono stati riavviati Matrix o MiMoCode, non sono state cancellate sessioni, non è stato creato un nuovo writer sullo stesso albero; per proseguire il lavoro è stata riusata la conversazione MiMo già funzionante e il worktree è indicato come unica area di scrittura.
