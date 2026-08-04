# Issue: la launch policy Codex configurata in MATRIX non raggiunge `codex app-server`

Data: 2026-08-04

Stato: closed

Priorita suggerita: alta

Ambiente verificato:

- MATRIX `v0.1.28`
- `@agentclientprotocol/codex-acp` `1.1.4`
- Windows, postazione Half Pocket di Roberto
- agente pubblico MATRIX: `codex`

## Sintesi

La configurazione documentata da MATRIX per eseguire Codex in trusted local
workspace mode viene salvata correttamente nella SSOT e aggiunta al comando del
provider:

```text
matrix config set agent.trust_mode true
matrix agent args set codex -- -c sandbox_mode=danger-full-access -c approval_policy=never
```

Gli argomenti risultano presenti in `matrix agent show codex` e nella command
line del processo Node che esegue `codex-acp`. Tuttavia il provider canonico
non li inoltra al processo figlio `codex app-server`; di conseguenza la policy
registrata da MATRIX non diventa la policy effettiva di Codex.

Il comportamento contraddice il contratto documentato in
`docs/matrix_protocol_neutral_runtime.md`, secondo cui gli SSOT append args
configurano esplicitamente la provider launch policy di Codex.

## Riproduzione

1. Configurare MATRIX:

   ```text
   matrix config set agent.trust_mode true
   matrix agent args set codex -- -c sandbox_mode=danger-full-access -c approval_policy=never
   ```

2. Riavviare il runtime MATRIX, così da escludere il riuso di un client creato
   prima della modifica.
3. Avviare una run `codex` che esegua un comando locale tramite gli strumenti
   Codex.
4. Ispezionare in sola lettura le command line dei processi `node.exe` della
   catena MATRIX -> `codex-acp` -> Codex.

## Risultato osservato

Il processo wrapper riceve gli argomenti configurati:

```text
node.exe ...\@agentclientprotocol\codex-acp\dist\index.js
  -c sandbox_mode=danger-full-access
  -c approval_policy=never
```

Il processo Codex figlio viene invece avviato soltanto come:

```text
node.exe ...\@openai\codex\bin\codex.js app-server
```

L'entrypoint distribuito da `@agentclientprotocol/codex-acp` `1.1.4` conferma
la causa: la command line del wrapper viene letta solo per `--version`, `login`
e `cli`; nel percorso ACP ordinario `startCodexConnection` genera il processo
figlio con gli argomenti fissi `app-server`.

Il README del provider espone invece `CODEX_CONFIG` e `INITIAL_AGENT_MODE` come
superfici supportate per la configurazione runtime.

La run MATRIX può completare e produrre una risposta finale, ma i comandi che
richiedono accesso in scrittura a database SQLite locali continuano a fallire
con errori di apertura del database. La presenza di
`protocol_meta.agent_launch_policy.trusted_terminal=true` nel trace descrive
quindi la configurazione richiesta, non prova che essa sia stata applicata al
processo Codex effettivo.

## Risultato atteso

- La configurazione trusted-terminal governata da MATRIX deve raggiungere il
  processo `codex app-server` usando una superficie supportata dal provider
  canonico corrente.
- La run trace deve distinguere policy richiesta e policy effettivamente
  applicata; non deve dichiarare `trusted_terminal=true` basandosi soltanto
  sugli argomenti del wrapper.
- `matrix agent doctor codex` deve rilevare una policy non applicabile prima
  del dispatch, oppure riportare esplicitamente che non è verificata.
- Il contratto deve restare protocol-neutral: il dettaglio Codex appartiene
  all'adapter/installazione del provider, non ai consumer Desk o Funnels.

## Impatto

- I consumer credono che Codex operi senza sandbox e senza approval
  interattive, mentre il processo effettivo può usare policy diverse.
- I trace possono fornire evidenza positiva fuorviante.
- Le automazioni non interattive possono fallire soltanto durante l'uso reale
  di tool locali, dopo che readiness e routing sono già risultati validi.
- Prompt workaround o ACL più permissive rischiano di mascherare il difetto e
  allargare inutilmente i permessi della postazione.

## Criteri di accettazione

- Un test MATRIX-owned usa un fake provider a due processi e dimostra che la
  policy configurata raggiunge il processo runtime effettivo, non soltanto il
  wrapper.
- Con il provider canonico installato da `matrix install codex`, una prova
  Windows verifica la policy effettiva del processo Codex dopo un riavvio
  pulito del runtime.
- `routing.decision.protocol_meta.agent_launch_policy` indica separatamente
  almeno policy richiesta, meccanismo di applicazione e stato verificato.
- Una run trusted esegue una prova locale bounded che richiede davvero la
  policy configurata e termina senza fallback interattivi o errori sandbox.
- Documentazione, installer, doctor e runtime usano lo stesso contratto.

## Confine della diagnosi

Questa issue non attribuisce automaticamente a MATRIX ogni errore SQLite: dopo
la correzione della propagazione, eventuali errori residui di ACL o ownership
dei database devono essere diagnosticati separatamente nel relativo tool/PAL.

## Decisione maintainer proposta

- Decisione: accepted.
- Motivo: il contratto pubblico di launch policy è registrato e tracciato ma
  non viene applicato al runtime Codex effettivo con il provider canonico.
- Scope: installer/adapter Codex, validazione `agent doctor`, metadata di run e
  test real-provider Windows.
- Evidenza richiesta: test a due processi, doctor negativo sul contratto non
  applicabile e canary reale con provider canonico.

## Risposta maintainer

- Decisione: accepted.
- Motivo: Matrix registrava policy richiesta come evidenza positiva anche se
  wrapper canonico ignorava argv; difetto rompeva automazioni e audit.
- Scope: registry adapter launch-policy, installer Codex, router ACP,
  doctor/show, run trace, test Linux/Windows e documentazione.
- Architettura: contratto comune `requested` / `effective` /
  `application_mechanism` / `verification_status`; mapping provider-specifici
  registrati in `internal/logic/agentlaunch`, senza branch nei consumer.

## Risoluzione

- Installer canonico marca contratto `codex-acp-env-v1` e preserva override
  SSOT durante reinstallazione.
- Adapter Codex rimuove policy riconosciuta da argv wrapper e la traduce in
  `INITIAL_AGENT_MODE` e `CODEX_CONFIG`.
- Trusted mode richiede coppia completa
  `sandbox_mode=danger-full-access` / `approval_policy=never`, mappata a
  `agent-full-access`; combinazioni incomplete o non supportate falliscono.
- Router verifica o imposta modalità esatta dopo ACP `session/new`,
  `session/resume` e `session/load`; downgrade a `agent` non accettato.
- Doctor e daemon riportano `launch_policy_invalid` per provider vecchio,
  contratto mancante, config JSON invalida o modalità incompatibile.
- Run trace non inferisce più trusted state da argv. Espone policy richiesta ed
  effettiva, meccanismo, stato verificato; `trusted_terminal=true` soltanto su
  policy full-access effettiva e verificata.

## Evidenza

- `TestRouterCodexPolicyReachesRuntimeChild`: fake provider a due processi;
  runtime child riceve `agent-full-access`, `never` e `danger-full-access`.
- `TestApplyPreferredSessionModeUsesExactConfiguredValue`: exact-mode, senza
  downgrade.
- `TestApplyPreferredSessionModeFailsClosedWhenUnavailable`: fail-closed.
- `TestResolveEndpointFailsClosedWithoutInstalledProviderContract`: doctor /
  runtime contract negativo.
- Canary Linux pulito: installer Matrix + provider canonico
  `@agentclientprotocol/codex-acp@1.1.9` + nuova sessione ACP; PASS in `93.00s`.
- CI `windows-codex-policy`: install pulita provider canonico e nuova sessione
  ACP su `windows-latest`; gate obbligatorio prima del tag `v0.1.29`.
