# Reversioni — l'ambiente dei figli di Matrix

Ogni riga è un esperimento **eseguito**: la reversione compila, viene applicata
all'albero, il test indicato cade per la ragione scritta, e il file viene
ripristinato **dalla copia** (mai con `git`), verificato con `sha256`. Un numero
senza elenco non è verificabile, quindi l'elenco sta qui e non in un messaggio.

Metodo: si copia il file, si applica la reversione, si compila, si esegue il test,
si ripristina dalla copia. Una reversione che colpisce un ramo che il test non
attraversa non è una prova.

## Il figlio agente (`pkg/zedacpstdio`)

| nome | cosa reverte | test che cade |
|---|---|---|
| A1 | l'ambiente del figlio torna a `append(os.Environ(), spec.Env...)`: il daemon entra nel figlio | `TestTheAgentChildDoesNotInheritTheDaemonsEnvironment` — `the agent read MATRIX_DAEMON_KEY_SENTINEL out of the daemon's environment` |
| A2 | l'ambiente del figlio è la sola allowlist: le credenziali dichiarate dall'agente non arrivano | `TestTheAgentChildDoesNotInheritTheDaemonsEnvironment` — `the agent's own credential did not reach it: the launch would break a real agent`; `TestTheAgentChildStartsThroughTheRealLaunchPreamble` |
| A3 | il preambolo di lancio (`PrepareStdio`) non viene più attraversato | `TestTheAgentChildStartsThroughTheRealLaunchPreamble` — l'agente non riporta `NVM_DIR` |

A1 e A2 sono le due metà della stessa decisione: il daemon **non** entra, ciò che
l'agente dichiara **sì**. A1 è la fuga, A2 è il modo di rompere un agente reale
restringendo troppo.

## L'allowlist condivisa (`internal/logic/childenv`)

| nome | cosa reverte | test che cade |
|---|---|---|
| B1 | `Environment()` torna a `os.Environ()`: l'allowlist non filtra più nulla | `TestEnvironmentCarriesTheAllowlistAndNothingElse`; `TestAValidatorCannotSeeTheDaemonsEnvironment` (`the validator read a daemon secret…`); `TestTheGitProbeHandsTheChildOnlyTheAllowlistedEnvironment` (`the child saw "GIT_DIR=…"`) |
| B2 | `Names` torna senza i due nomi Windows (`SystemRoot`, `COMSPEC`) | `TestTheAllowlistIsPinnedAsData` — `the child allowlist is [PATH HOME LANG LC_ALL TZ TMPDIR], want [… SystemRoot COMSPEC]` |

B1 cade su **tre consumatori in tre package diversi**: è la prova che la politica
è una sola e che tutti la usano davvero. B2 è la prova che allargare la lista è
una decisione visibile.

## Strumenti di sistema su Windows — non eseguiti qui

| nome | cosa reverte | test che cade |
|---|---|---|
| C1 | `taskkill` torna a ereditare l'ambiente (`pkg/zedacpstdio/process_windows.go`) | non eseguibile su questa macchina: file `//go:build windows`. Evidenza disponibile: `go build`/`go vet` per `GOOS=windows`, e il job `windows-codex-policy` della CI che lancia un agente reale su `windows-latest` |
| C2 | `icacls` torna a ereditare l'ambiente (`internal/logic/vaultsec/permissions_windows.go`, due call site) | come C1 |

Dichiarato invece di nascosto: su linux/darwin questi due file non vengono
compilati, quindi qui non esiste un test che possa cadere. La reversione è
meccanica (una riga), la compilazione incrociata è verificata, e l'esecuzione la
copre la CI Windows.

## Perché l'allowlist ha due nomi Windows

Un processo Windows che lancia uno strumento di sistema — e un programma che ne
lancia uno attraverso `cmd.exe` — cerca `SystemRoot` e `COMSPEC`. Non sono
segreti e non decidono su cosa lavora il figlio, quindi stanno nella lista; un
daemon che non li possiede (ogni unix) non li passa a nessuno, e i figli unix
restano esattamente quelli di prima.

## Il reperto: la guardia che apriva il caso peggiore

Prima della correzione la riga era:

```go
if len(spec.Env) > 0 {
	cmd.Env = append(os.Environ(), spec.Env...)
}
```

Sembra una guardia — "se non c'è niente da aggiungere, non tocco l'ambiente" — e
invece apre il caso peggiore: con `spec.Env` vuoto `cmd.Env` resta `nil`, cioè
**eredità completa**, mentre con `spec.Env` pieno il figlio eredita comunque tutto
più le sue voci. Il figlio agente riceveva l'ambiente del daemon in **entrambi** i
rami; la guardia non proteggeva nessuno dei due e faceva sembrare deciso ciò che
non lo era. Una guardia che sembra proteggere e apre il caso peggiore è peggio di
nessuna guardia, perché nessuno la guarda.

La stessa forma in `internal/providers/exec/` è invece corretta: lì l'eredità è la
politica voluta, quindi `nil` e "tutto più le voci dichiarate" sono la stessa cosa.

## Due politiche d'ambiente, una per fiducia

L'allowlist d'ambiente serve dove il figlio **non** è scelto dall'operatore o non è
nella sua fiducia: il **processo agente** (guidato da un modello, esegue azioni
generate) e il **validatore del chiamante** (codice fornito via richiesta). Un
comando **scritto dall'operatore** e di breve durata — installer, toolchain — vive
nel dominio di fiducia di chi l'ha scritto e si aspetta il suo ambiente: toglierglielo
cambierebbe il mestiere di un runner di comandi.

Quindi: due politiche, e la differenza è **chi sceglie il programma**. La decisione
sta accanto al codice che la applica — `internal/providers/exec/doc.go` — e i sette
siti `os.Environ()` di quel package la richiamano uno per uno, così chi legge le due
righe a tre file di distanza non "sistema" quella sbagliata.
