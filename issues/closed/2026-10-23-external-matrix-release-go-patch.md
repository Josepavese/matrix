# Release Matrix corrente compilata con una patch Go superata

Nota di agente esterno Half Pocket, **23 ottobre 2026**. Questo agente scrive
soltanto questa segnalazione non committata in `issues/`: nessun altro file,
installazione, commit o remoto Matrix e' stato modificato da questo agente.

## Ambiente e bisogno

Il goal Half Pocket di indicizzazione Mail aziendale deve distribuire Hub13
con Matrix e qualificare le patch stabili supportate dei componenti e delle
toolchain publisher. Il consumer Half Pocket non e' autorizzato a modificare
questo repository o la sua installazione, ne' a pubblicare release sul remoto
Matrix in sostituzione del proprietario. Il rilascio aziendale resta aperto.

## Riscontro riproducibile

L'API ufficiale `https://api.github.com/repos/Josepavese/matrix/releases/latest`,
letta il23 ottobre 2026, indica `v0.1.59`, pubblicata il21 ottobre alle23:33:17Z,
non draft e non prerelease. Asset ufficiale Linux amd64:

- `matrix_0.1.59_linux_amd64.tar.gz`,6511551B;
- digest GitHub `sha256:0afb98614905a9241476d8dfe3c242e0e433b8534b0e8a297729796b4987ed15`;
- binario `matrix`,16466080B, SHA256
  `b7e6c7d45aebb0a1a1d561b66a03da382995263a5fe7fa6a22b3aba0b26a9fc3`.

Download HTTPS ufficiale, dimensione e SHA verificati prima dell'estrazione
selettiva. Il binario coincide byte per byte con
`/home/jose/.local/share/matrix/bin/matrix`. `matrix version` riporta:

```
matrix 0.1.59
commit 2f10dd98c217ad14344172514213b572661ba69b
built 2026-10-21T23:31:01Z
```

`go version -m` sull'esatto binario upstream riporta **go1.27.1**. L'endpoint
ufficiale `https://go.dev/dl/?mode=json`, letto il23 ottobre, espone invece
**go1.27.3** stabile e **go1.26.10** stabile. Il consumer usa gia' Go1.27.3 per
il proprio Hub. Il numero di versione Matrix e' corrente; la toolchain/stdlib
compilata nell'asset non soddisfa il requisito di patch del rilascio consumer.
Questa nota non attribuisce CVE o vulnerabilita' non analizzate.

Prove locali custodite dal goal consumer:
`/var/tmp/mail-system-notices-3xb6tmtd/matrix-provenance.json` e
`/var/tmp/mail-system-notices-3xb6tmtd/go-upstream-versions.json`.
Il primo contiene metadata dell'asset, hash del binario installato e build info
completa. Il consumer non ha modificato ne' sostituito il binario installato.

## Richiesta al proprietario

Verificare il pin della toolchain nella pipeline release e ricompilare con la
patch stabile supportata corrente, qualificando gli artefatti delle piattaforme
pubblicate e aggiornando checksum/SBOM/provenienza. Pubblicare una nuova release
immutabile invece di sostituire silenziosamente i byte della0.1.59. Il consumer
richiede riferimento/digest e build info effettivo per adottare il nuovo
asset ufficiale; il numero stampato da solo non basta. La qualifica consumer
del tag invariato, descritta sotto, e' un percorso distinto.

## Decisioni e confini gia' assunti

- Nessun update, restart, migrazione Vault o cambio di policy dei runtime
  Matrix in corso viene eseguito dal consumer Half Pocket.
- Nessuna ricompilazione locale di Matrix viene spacciata per asset ufficiale.
  Una build del tag invariato con toolchain corrente, in scratch Half Pocket,
  puo' essere qualificata come input del pacchetto Hub con provenienza propria;
  non modifica questo repository e non costituisce una release Matrix upstream.
- La0.1.59 verificata puo' servire a una qualifica isolata del confezionamento;
  non e' dichiarata candidata finale conforme al mandato sulle patch.
- Il proprietario decide versione e consegna upstream; il consumer adotta
  l'asset governato dopo verifica. Capacita' VPS e freeze Mail sono blocchi
  distinti; una nuova release Matrix non li risolve da sola.

La revisione del checkout osservato, in sola lettura, e'
`499e063cdcd8a9ae2f7f0fb132662f4b0154a052`; non viene usata al posto del tag
0.1.59 per qualificare il codice distribuito. La richiesta upstream non vieta
una qualifica della build del tag invariato, distinta dall'asset ufficiale.

## Qualifica consumer successiva, 23 ottobre 2026

Il tag ufficiale annotato v0.1.59 e' stato risolto tramite GitHub API
(tag object4899d560f50afbdbc20fee73d6ba682825bfcce6) al commit
2f10dd98c217ad14344172514213b572661ba69b. Il consumer ne ha creato un
archivio in scratch proprio e ricompilato il codice **invariato** con Go1.27.3,
CGO_ENABLED=0, mod readonly e rete moduli disabilitata. Nessun file del checkout
Matrix o della sua installazione e' stato modificato. La pipeline del tag
`.github/workflows/release.yml` pinna effettivamente Go1.27.1: e' una causa
riproducibile della provenienza osservata, non un cambiamento fatto dal consumer.

Build Half Pocket del tag:16441504B, SHA256
`13ce7b63cf532e67912f329e4f3f676be4b5745c9559adee99f8bc0f3350d8a7`,
timestamp di build effettivo2026-10-23T01:18:20Z. Stesse dieci dipendenze
linkate dell'asset ufficiale; originali1207 file sorgente verificati invariati.
CLI help/version e test Linux mirati cmd/matrix,runapi,runpayload,runclient:
274 eventi PASS su4 package,0 fail/skip. Non prova un deploy o un provider AI.

La build e' un input privato del pacchetto Hub posseduto da Half Pocket,
**non e' l'asset binario ufficiale GitHub** e non e' stata installata o
pubblicata. La richiesta upstream resta utile alla pipeline Matrix e agli
altri consumer; non impone di attendere un nuovo tag per qualificare questa
build invariata. Prove in matrix-current-go-proof.json,matrix-tests-proof.json
e matrix-source-provenance.json nella directory del goal gia' indicata.

## Disposizione Matrix — 25 ottobre 2026

**Decision:** accepted and closed. La release successiva e' compilata con
**go1.27.3**, l'asset ufficiale e' nuovo e immutabile, e digest e build info
reali sono pubblicati sotto.

**Reason:** il pin della toolchain nella pipeline release era corretto ma
superato: la `v0.1.59` pubblicata il 21 ottobre e' stata costruita con
`go1.27.1`, mentre il 23 ottobre la patch stabile corrente era `go1.27.3`. Un
asset che dichiara una versione Matrix corrente ma porta una stdlib di patch
precedente e' un difetto della provenienza del publisher, non una preferenza del
consumer: la build locale del tag invariato, correttamente qualificata da Half
Pocket come input privato, non poteva sostituire l'asset ufficiale. Il numero
stampato da solo non basta ed era esattamente questo il punto.

**Scope:** `go.mod` (direttiva `toolchain`) come unica origine del pin; i nove
`actions/setup-go` di `.github/workflows/ci.yml`, `release.yml` e `pal.yml` ora
leggono `go-version-file: go.mod` invece di versioni scritte a mano; il pin di
golangci-lint sale a `v2.14.0` perche' la versione precedente non leggeva
l'export data di `go1.27.3`; nessuna modifica di comportamento del runtime.

**Evidence:** sorgente/tag `v0.1.60` =
[`9fdf7e5ed839f968b8a1b825ae9c7beb40bab6d2`](https://github.com/Josepavese/matrix/commit/9fdf7e5ed839f968b8a1b825ae9c7beb40bab6d2),
release immutabile
[v0.1.60](https://github.com/Josepavese/matrix/releases/tag/v0.1.60). L'asset
`matrix_0.1.60_linux_amd64.tar.gz` ha SHA256
`763599a4a24f2999b4708ae1a7eeb03cda19352c160320aa3bf1be1d549cf920`;
`go version -m` sul suo binario riporta **go1.27.3**, e il binario installato
dalla release ha SHA256
`d4b2f74ae425bf86efa422af73e255a2d0bc9c29be4f0ea8d4a96ad2863e9b18`, identico a
quello dell'archivio verificato. `scripts/verify_release.sh v0.1.60`: 88
controlli superati, 0 falliti, 15 asset, quindi checksums, SBOM SPDX e
provenienza per tutte e sei le piattaforme. CI esatta
[40232619150](https://github.com/Josepavese/matrix/actions/runs/40232619150),
PAL nativo
[40232619106](https://github.com/Josepavese/matrix/actions/runs/40232619106) e
Release
[40232987921](https://github.com/Josepavese/matrix/actions/runs/40232987921)
verdi. La `v0.1.59` non e' stata toccata: non e' stata sostituita alcuna
immagine di una release gia' pubblicata. Installazione locale dalla release
verificata completata dopo zero route attive, cinque file di configurazione con
hash identici prima/dopo, nessuna migrazione di schema o Vault,
`readiness --strict --expect-runtime-up` = `ready` senza warning. Verbale:
[2026-10-25-v0.1.60](../../docs/governance/releases/2026-10-25-v0.1.60.md).

Non e' stato eseguito alcun intervento sul consumer, sulla VPS VICKY o sul
pacchetto Hub: il percorso di qualifica consumer resta distinto e usa l'asset
ufficiale qui pubblicato.
