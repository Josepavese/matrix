# Halfdesk browser client: preflight CORS su `/v1/runs`

Data: 2026-06-23
Stato: closed
Origine: integrazione Half Pocket Desk

## Contesto

Half Pocket Desk espone una dashboard web locale che invia prompt operativi a
MATRIX locale tramite browser:

- dashboard: `http://127.0.0.1:30031`
- MATRIX locale: `http://127.0.0.1:9091`
- endpoint usato: `POST /v1/runs`

La chiamata da browser usa `fetch` con `Content-Type: application/json`, quindi
il browser esegue un preflight `OPTIONS` prima del `POST`.

## Problema osservato

Con MATRIX `v0.1.20`, il runtime e' raggiungibile via `curl` e il `POST
/v1/runs` funziona se chiamato direttamente, ma il browser puo' fallire prima
del `POST` perche' il preflight CORS non riceve una risposta valida.

Evidenza:

```bash
curl -i -X OPTIONS http://127.0.0.1:9091/v1/runs \
  -H 'Origin: http://127.0.0.1:30031' \
  -H 'Access-Control-Request-Method: POST' \
  -H 'Access-Control-Request-Headers: content-type'
```

Risultato osservato su `v0.1.20`:

```text
HTTP/1.1 405 Method Not Allowed
```

Effetto lato Halfdesk: la dashboard mostra un errore simile a "MATRIX locale
non raggiungibile dal browser", anche quando il demone MATRIX e' attivo.

## Valutazione richiesta

Il team MATRIX dovrebbe decidere se il server HTTP locale deve supportare CORS
per client browser locali.

Possibile policy conservativa:

- consentire solo origin loopback/locali:
  - `http://localhost:*`
  - `http://127.0.0.1:*`
  - `http://[::1]:*`
- non consentire origin remoti;
- mantenere invariata l'autenticazione `X-Matrix-Key` quando configurata;
- supportare almeno:
  - metodi: `GET`, `POST`, `OPTIONS`;
  - header: `Content-Type`, `X-Matrix-Key`, `Authorization`;
  - `Vary: Origin`.

## Criteri di accettazione proposti

Preflight locale accettato:

```bash
curl -i -X OPTIONS http://127.0.0.1:9091/v1/runs \
  -H 'Origin: http://127.0.0.1:30031' \
  -H 'Access-Control-Request-Method: POST' \
  -H 'Access-Control-Request-Headers: content-type'
```

Risultato atteso:

```text
HTTP/1.1 204 No Content
Access-Control-Allow-Origin: http://127.0.0.1:30031
```

Origin remoto rifiutato:

```bash
curl -i -X OPTIONS http://127.0.0.1:9091/v1/runs \
  -H 'Origin: https://example.com' \
  -H 'Access-Control-Request-Method: POST'
```

Risultato atteso:

```text
HTTP/1.1 403 Forbidden
```

POST locale da browser/origin locale:

```bash
curl -i -X POST http://127.0.0.1:9091/v1/runs \
  -H 'Origin: http://127.0.0.1:30031' \
  -H 'Content-Type: application/json' \
  -d '{"channel_id":"halfdesk.pm.smoke","agent_id":"codex","workspace_id":"halfpocket","input":{"text":"Rispondi solo OK."}}'
```

Risultato atteso: risposta applicativa normale di MATRIX, con header CORS per
l'origin locale.

## Alternative

- Non supportare browser diretti e richiedere sempre un bridge CLI/proxy locale.
- Esporre un endpoint health/bridge specifico per client locali.
- Rendere la allowlist CORS configurabile via vault/config invece che
hardcoded.

## Nota operativa

Halfdesk non dovrebbe patchare MATRIX direttamente. Il percorso corretto e'
aprire questa issue e lasciare al team MATRIX la decisione su implementazione,
release e compatibilita' cross-platform.

## Maintainer Response

Decision: accepted and implemented.

Reason: il problema e' un contratto Matrix-owned dell'ingress HTTP locale.
Browser dashboard, strumenti supervisory e client locali devono poter usare
`/v1/runs` senza un proxy dedicato, ma solo da origin loopback. La soluzione non
aggiunge comportamento Halfdesk-specifico e mantiene invariata l'autenticazione.

Scope:

- `cmd/matrix/run.go`: il mux HTTP Matrix viene avvolto dal middleware CORS
  locale.
- `internal/providers/matrixapi/cors.go`: policy CORS loopback-only.
- `internal/providers/matrixapi/cors_test.go`: test per preflight locale,
  origin remoto e POST locale con header CORS.
- `docs/wiki/API-Reference.md`, `docs/matrix_protocol_neutral_runtime.md` e
  `docs/matrix_threat_model.md`: contratto CORS pubblico e guardrail ingress.

Evidence:

- `go test ./internal/providers/matrixapi`
- `go test ./...`
- `go run ./scripts/code_governance.go --config code-governance.toml`
- `go run ./scripts/governance_check --manifest governance/manifest.toml`
