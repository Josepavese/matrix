# Reversioni — contratto di consegna

Ogni riga è un esperimento **eseguito**: la reversione compila e il test indicato
cade. Un numero senza elenco non è verificabile, quindi l'elenco sta qui e non in
un messaggio.

Metodo: si copia il file, si applica la reversione, si compila, si esegue il test,
si ripristina **dalla copia** (mai con `git`). Una reversione che colpisce un ramo
che il test non attraversa non è una prova: va verificato che il ramo sia
raggiunto, non solo che il test cada.

## `internal/logic/deliverycontract`

| nome | cosa reverte | test che cade |
|---|---|---|
| E1 | contratto non dichiarato riportato come `accepted` | `TestNotDeclaredIsNotAcceptance` |
| E2 | artifact mancante riportato come `accepted` | `TestCompletedRunWithoutTheDeclaredArtifactIsIncompleteDelivery` |
| E3 | mismatch di digest ignorato | `TestPresentArtifactWithTheWrongContentIsNotAccepted` |
| E4 | assenza di workspace riportata come `incomplete` | `TestNoWorkspaceIsUnverifiableNotIncomplete` |
| E5 | containment lessicale del path rimosso | `TestArtifactPathCannotLeaveTheRunWorkspace` |
| E6 | exit code del validatore ignorato | `TestValidatorExitCodeDecidesAcceptance`, `TestValidatorIsARealProcess` |
| E7 | timeout inghiottito e trattato come pass | `TestValidatorThatCannotBeRunIsUnverifiable` |
| E8 | comando dichiarato come stringa di shell accettato | `TestContractRejectsAShellString` |
| E9 | containment dopo `EvalSymlinks` rimosso | `TestArtifactSymlinkOutOfTheWorkspaceIsRefused` |
| Y1 | guard `unverifiable` in `Verdict.add` rimosso | `TestUnverifiableOutranksIncompleteInEitherOrder` (sotto-caso *unevaluable first*) |
| Y2 | regola "nessuna cattura": stdout scritto nel `Detail` | `TestARealNoisyValidatorCannotLeakItsOutput` |
| Y5 | ingoiamento degli errori di risoluzione del path | `TestAnUnresolvablePathSaysWhyItCouldNotBeResolved` |
| A3 | binario risolto omesso dal record di audit | `TestEveryValidatorExecutionLeavesAnAuditRecord` |

## `internal/providers/runapi`

| nome | cosa reverte | test che cade |
|---|---|---|
| W1 | `terminalResult` non salda il contratto | `TestTheTerminalPathSettlesTheContract` |
| W2 | guard "decisa una volta sola" rimosso | `TestTheVerdictIsDecidedOnceAndSurvivesTheWorkspaceChanging` |
| W3 | nessun contratto riportato come `accepted` | `TestARunWithoutAContractIsNotReportedAsAccepted` |
| W4 | contratto mai valutato riportato come `incomplete` | `TestADeclaredContractTheRunNeverEvaluatedIsUnverifiable` |
| W5 | verdetto non scritto nello `Status` dell'evento | `TestTheVerdictSurvivesARedactingTracePolicy` |
| W6 | `/explain` smette di esporre il verdetto | `TestACompletedRunWithoutItsArtifactIsReportedAsIncompleteDelivery` |
| W7 | opt-in non applicato: `{}` conta come contratto | `TestARunWithoutAContractIsNotReportedAsAccepted` |
| V1 | append della dichiarazione rimosso | `TestTheRunRecordsTheContractBeforeItIsDispatched` |
| V2 | validazione al confine rimossa | `TestAnUnevaluableContractStopsTheRunAtTheBoundary` |
| V3 | contratto vuoto trattato come dichiarato | `TestAContractIsRefusedBeforeTheRunExists` |
| V4 | tag JSON `delivery_contract` rinominato | `TestTheContractIsDecodedFromTheRequestJSON` |
| Y3 | ripiego su `event.Status` rimosso | `TestAVerdictEventWhosePayloadIsGoneStillAnswersFromItsStatus` |
| Y4 | `deliveryMu` rimosso | `TestSimultaneousTerminalsRecordExactlyOneVerdict` (sotto `-race`) |
| X1 | testo generico del 409 ripristinato | `TestModelIDConflictNamesTheAgentAndTheRemedy` |
| X2 | remedìa tolta dal 409 | `TestModelIDConflictNamesTheAgentAndTheRemedy`, `TestModelIDConflictKeepsTheRemediesTheRuntimeStatusOffers` |
| X3 | nome dell'agente tolto dal 409 | `TestModelIDConflictNamesTheAgentAndTheRemedy` |
| A1 | interruttore dei validatori non applicato al confine | `TestTheValidatorSwitchRefusesContractsThatDeclareOne` |
| A2 | evento di audit per esecuzione non scritto | `TestEveryValidatorExecutionLeavesAnAuditRecord` |

## `internal/logic/agentdoctor`

| nome | cosa reverte | test che cade |
|---|---|---|
| Z1 | accettazione promossa a consegna (`Promised` su `accepted`) | `TestAcceptanceIsNotDelivery` |
| Z2 | anche il rifiuto del provider promette consegna | `TestAProviderWithoutTheCapabilityIsRefusedWithoutPromisingDelivery` |
| Z3 | agente mai tentato letto come incapace | `TestAnUnobservedAgentIsUnknownNotIncapable` |
| Z4 | garanzia sul terminale rimossa | `TestTheTerminalDoesNotDependOnTheAttach` |
| Z5 | assenza di trasporto letta come disponibile | `TestNoTransportRefusesWithoutClaimingTheProviderWasAsked` |

## Limiti dichiarati accanto a queste prove

- Nessuna reversione prova che la proprietà valga su Windows o Darwin: la
  compilazione incrociata è verificata, l'esecuzione no. I test che dipendono da
  `sh` o dai symlink si **skippano** dichiarandolo invece di passare per il motivo
  sbagliato.
- `TestModelIDConflictKeepsTheRemediesTheRuntimeStatusOffers` è una **copia
  fissata**, non un confronto vivo con `agentmgr`: riscrivere solo `agentmgr` non
  lo fa cadere.
