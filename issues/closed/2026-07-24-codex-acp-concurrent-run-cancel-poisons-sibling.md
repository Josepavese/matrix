# Issue: Codex ACP `run cancel` interrompe una run parallela nello stesso workspace

Data: 2026-07-24

Stato: closed

Origine: integrazione Local Companion / Half Pocket Desk e Funnels

Provider osservato: Codex via ACP

Versione Matrix installata:

```text
matrix 0.1.27
commit 85ad825c1a9ae950010dd3799b8aeb93666be46e
built 2026-07-22T09:52:53Z
```

## Sintesi

Due `/v1/runs` asincrone, associate a canali e sessioni logiche distinte ma allo
stesso `agent_id=codex` e workspace, condividono il client ACP locale.

La cancellazione run-scoped della seconda run tramite
`POST /v1/runs/{run_id}/actions` con `action=cancel` termina correttamente la
run selezionata, ma quasi simultaneamente fa fallire la prima run con
`client context cancelled`.

La cancellazione non resta quindi isolata al `run_id` indicato.

## Riproduzione reale minima

Sono state create prima due sessioni dedicate con
`POST /v1/session-actions` e `action=new`, usando due `channel_id` distinti e lo
stesso workspace. Successivamente sono state avviate due run asincrone.

### Run A, non cancellata

```text
channel_id: agent.task.5d99acc129634a16a56f7512c3b1e1ef
logical_session_id: f307a639-01e6-497c-bce7-519cb5a164a4
run_id: run-e62afbd7-33ec-4969-9590-c35bffaa0784
started_at: 2026-07-24T13:36:52.099994415Z
```

### Run B, cancellata

```text
channel_id: agent.task.4a570f0061014a6897170132f29e494e
logical_session_id: c9680dbc-7a8a-43ac-a109-216f5111ddd2
run_id: run-d0648b65-d464-40ba-a733-c9d86b6b76d4
started_at: 2026-07-24T13:36:52.123391112Z
```

Chiamata applicata esclusivamente alla run B:

```http
POST /v1/runs/run-d0648b65-d464-40ba-a733-c9d86b6b76d4/actions
Content-Type: application/json

{"action":"cancel"}
```

Esito B:

```text
status: cancelled
completed_at: 2026-07-24T13:36:52.426353616Z
```

Esito A, 13 millisecondi dopo:

```text
status: failed
completed_at: 2026-07-24T13:36:52.435458862Z
error: [agent_preflight_failed] agent provider preflight failed agent=codex protocol=acp phase=session/new: ACP new session failed: client context cancelled
```

Lo stesso difetto era stato osservato in precedenza durante
`phase=session/prompt`, quindi non sembra limitato alla materializzazione
iniziale della sessione.

## Verifica dell'alternativa `session/cancel`

E' stata verificata anche la seconda superficie documentata:

```http
POST /v1/session-actions
Content-Type: application/json

{
  "channel_id": "qa.session.active.b.matrixsessioncancelactiveqadhfmiI",
  "action": "cancel",
  "target": "019f945f-35ad-7800-a559-8f4e5e70d09a"
}
```

Durante il primo turno, il target logico restituisce inizialmente:

```text
session <logical-id> has no remote session id to cancel
```

perche' il mirror viene aggiornato soltanto alla fine del turno. Usando il
`remote_session_id` letto dalla run trace:

- la prima chiamata puo' restituire `session <remote-id> not found`, finche' il
  provider non rende la sessione elencabile;
- una chiamata successiva restituisce `201` e
  `Canceled remote acp session: <remote-id>`;
- la run resta tuttavia `running` e conclude normalmente con
  `status=completed`, `stop_reason=end_turn`.

Nel test:

```text
run B: run-d2e57fc2-1824-4a4d-afe8-5637f452091e
remote session B: 019f945f-35ad-7800-a559-8f4e5e70d09a
started_at: 2026-07-24T13:44:55.149754894Z
completed_at: 2026-07-24T13:46:10.635847822Z
status: completed
stop_reason: end_turn
```

Questa superficie non e' quindi oggi un sostituto affidabile del run cancel
per Codex ACP.

## Contratto documentato rilevante

La documentazione Matrix corrente stabilisce che:

- i provider client sono risorse router-lifetime, non request-lifetime;
- cancellare una `/v1/runs` non deve implicitamente terminare il client ACP
  condiviso;
- un client avvelenato deve essere espulso per proteggere le run successive;
- `attach_context` non e' un'alternativa a `cancel` per un prompt ACP attivo.

La protezione della run successiva e' presente, ma l'espulsione del client
durante una cancellazione sembra ancora interrompere le run concorrenti che
stanno usando lo stesso client `agent_id + workspace_path`.

Riferimenti:

- `docs/matrix_protocol_neutral_runtime.md`
- `docs/matrix_agent_communication_run_trace.md`
- `docs/matrix_live_context_interrupt_policy.md`
- `docs/wiki/API-Reference.md`

## Comportamento atteso

- `POST /v1/runs/{B}/actions` con `cancel` ferma soltanto B.
- Una run A contemporanea, legata a un'altra sessione remota/logica nello
  stesso workspace, continua senza ricevere la cancellazione del client ACP.
- L'eventuale eviction/restart del provider client non espone A a
  `client context cancelled`.
- Il terminale di B resta `run.cancelled`; A conserva il proprio terminale
  naturale.

## Criteri di accettazione proposti

1. Test con due run Codex ACP concorrenti, sessioni logiche e remote distinte,
   stesso `agent_id + workspace_path`.
2. Cancellazione run-scoped di B mentre A e B sono entrambe in
   `session/prompt`.
3. B termina `cancelled`.
4. A non termina `failed` e non riceve `client context cancelled`.
5. La prova copre anche la race in cui A o B sta ancora eseguendo
   `session/new`.
6. Il test usa il vero pool router-lifetime, non due mock client separati.
7. Se `session/cancel` resta una superficie supportata per l'interruzione del
   primo turno, il remote ID run-bound viene usato come SSOT e l'esito della run
   riflette effettivamente la cancellazione; altrimenti l'API documenta il
   limite senza restituire un successo fuorviante.

## Impatto sui consumer

Un task manager parallelo non puo' esporre un pulsante "Ferma" affidabile: la
cancellazione di una task selezionata puo' interrompere un'altra task in corso.
Il workaround consumer e' differire l'hard cancel finche' non restano altre run
attive nello stesso workspace, sacrificando l'immediatezza dello stop.

## Perimetro di questo report

Il report iniziale documentava soltanto il problema; la risoluzione seguente
registra le modifiche Matrix applicate dopo la triage.

## Risoluzione

Data: 2026-07-24

Matrix ora:

- invia `session/cancel` usando il `remote_session_id` run-bound prima di
  annullare il context locale della run selezionata;
- rimuove il client ACP interessato dalle nuove assegnazioni;
- mantiene lease per le run sibling gia' attive sul client draining;
- chiude fisicamente il client solo dopo il completamento dell'ultima sibling;
- crea tombstone `process_reaped` soltanto dopo la chiusura fisica reale.

Acceptance automatica:

- due run concorrenti, stesso client router-lifetime e workspace, sessioni
  remote distinte;
- cancellazione B durante `session/prompt`;
- cancellazione B durante `session/new`;
- B termina per cancellazione;
- A resta attiva e completa senza `client context cancelled`;
- il client draining chiude dopo A.

Test canonico:

```text
TestRouterConcurrentRunCancelDoesNotPoisonSibling
```
