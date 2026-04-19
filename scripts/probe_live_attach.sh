#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'USAGE'
Usage: scripts/probe_live_attach.sh AGENT_ID [WORKSPACE_PATH]

Runs a real Matrix /v1/runs async live-context probe against an installed,
running Matrix runtime.

Environment:
  MATRIX_URL                 Runtime URL. Default: http://127.0.0.1:9091
  MATRIX_PROBE_SESSION_WAIT  Seconds to wait for a live session. Default: 90
  MATRIX_PROBE_FINAL_WAIT    Seconds to wait for terminal delivery evidence. Default: 210
  TRACE_OUT_DIR              Directory for full trace JSON. Default: /tmp
USAGE
}

if [[ "${1:-}" == "-h" || "${1:-}" == "--help" || $# -lt 1 ]]; then
  usage
  exit 0
fi

require_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "missing required command: $1" >&2
    exit 1
  fi
}

require_cmd curl
require_cmd jq

agent_id="$1"
repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
workspace_path="${2:-$repo_root}"
matrix_url="${MATRIX_URL:-http://127.0.0.1:9091}"
session_wait="${MATRIX_PROBE_SESSION_WAIT:-90}"
final_wait="${MATRIX_PROBE_FINAL_WAIT:-210}"
trace_out_dir="${TRACE_OUT_DIR:-/tmp}"
marker="MATRIX_LIVE_ATTACH_${agent_id}_$(date +%s)"

run_payload="$(jq -n \
  --arg ws "$workspace_path" \
  --arg marker "$marker" \
  --arg agent "$agent_id" \
  '{
    channel_id: ("real." + $agent + ".live-attach"),
    agent_id: $agent,
    execution_mode: "async",
    workspace_path: $ws,
    input: {
      text: (
        "Matrix real-agent live context test. Do not edit files. Keep this run active for about 35 seconds. " +
        "If useful, run a harmless terminal sleep/tick command. During this run Matrix may inject live sidecar context. " +
        "Do not invent the marker. If you receive live context before final answer, output exactly the injected marker " +
        "followed by LIVE_CONTEXT_PROCESSED. If not, output exactly NO_LIVE_CONTEXT_BEFORE_FINAL. " +
        "Marker prefix for validation only: " + $marker
      )
    },
    trace_policy: {
      content_mode: "inline",
      redaction_profile: "debug",
      include_protocol_meta: false
    }
  }')"

run_resp="$(curl -sS -X POST "$matrix_url/v1/runs" \
  -H 'Content-Type: application/json' \
  -d "$run_payload")"
run_id="$(printf '%s' "$run_resp" | jq -r '.run_id // empty')"
if [[ -z "$run_id" ]]; then
  jq -n --arg agent "$agent_id" --argjson response "$run_resp" \
    '{agent: $agent, error: "run_create_failed", response: $response}'
  exit 1
fi

trace=""
run_status=""
remote_session_id=""
logical_session_id=""
for _ in $(seq 1 "$session_wait"); do
  trace="$(curl -sS "$matrix_url/v1/runs/$run_id/trace")"
  run_status="$(printf '%s' "$trace" | jq -r '.run.status // empty')"
  remote_session_id="$(printf '%s' "$trace" | jq -r '.run.remote_session_id // empty')"
  logical_session_id="$(printf '%s' "$trace" | jq -r '.run.logical_session_id // empty')"
  if [[ -n "$remote_session_id" && -n "$logical_session_id" && "$run_status" == "running" ]]; then
    break
  fi
  if [[ "$run_status" == "completed" || "$run_status" == "failed" || "$run_status" == "cancelled" ]]; then
    break
  fi
  sleep 1
done

attach_payload="$(jq -n \
  --arg marker "$marker" \
  --arg agent "$agent_id" \
  '{
    action: "attach_context",
    reason: ("real_live_agent_probe_" + $agent),
    source_event_id: ("real-test-" + $agent),
    sidecar_capsules: [
      {
        provider: "matrix-real-test",
        id: $marker,
        schema: "matrix.live_context_probe.v0",
        version: "0.1",
        visibility: "llm_visible",
        format: "noema_xml",
        content: (
          "<matrix-live-context id=\"" + $marker + "\">" +
          "This context was injected while the run was active. Immediately output exactly: " +
          $marker + " LIVE_CONTEXT_PROCESSED</matrix-live-context>"
        ),
        metadata: {
          probe: "real-live-attach"
        }
      }
    ]
  }')"

attach_resp="$(curl -sS -X POST "$matrix_url/v1/runs/$run_id/actions" \
  -H 'Content-Type: application/json' \
  -d "$attach_payload")"
delivery_id="$(printf '%s' "$attach_resp" | jq -r '.delivery_id // empty')"

final_trace=""
for _ in $(seq 1 "$final_wait"); do
  final_trace="$(curl -sS "$matrix_url/v1/runs/$run_id/trace")"
  run_status="$(printf '%s' "$final_trace" | jq -r '.run.status // empty')"
  attach_status="$(printf '%s' "$final_trace" | jq -r \
    --arg did "$delivery_id" \
    '[.events[] | select(.kind=="run.context.attached" and (.metadata.delivery_id==$did))][-1].status // empty')"
  if { [[ "$run_status" == "completed" || "$run_status" == "failed" || "$run_status" == "cancelled" ]]; } &&
    { [[ "$attach_status" == "delivered" || "$attach_status" == "late" || "$attach_status" == "failed" || "$attach_status" == "unsupported" ]]; }; then
    break
  fi
  sleep 1
done

mkdir -p "$trace_out_dir"
trace_file="$trace_out_dir/matrix-${agent_id}-live-trace-$run_id.json"
printf '%s' "$final_trace" > "$trace_file"

printf '%s\n' "$final_trace" | jq -r \
  --arg agent "$agent_id" \
  --arg marker "$marker" \
  --arg did "$delivery_id" \
  --arg trace_file "$trace_file" \
  --argjson attach_response "$attach_resp" \
  '
    def eventseq($kind; $status): ([.events[] | select(.kind==$kind and (.status==$status)) | .sequence] | first);
    def attach_events: [.events[] | select(.kind=="run.context.attached" and (.metadata.delivery_id==$did))];
    def latest_attach: (attach_events[-1] // {});
    def has_marker_in_provider_output:
      ([.events[]
        | select(.kind=="run.context.attached" or .kind=="agent.message.delta" or .kind=="agent.message.final")
        | select(((.message // "") | contains($marker)) or ((.summary // "") | contains($marker)))
      ] | length > 0);
    {
      agent: $agent,
      run_id: .run.id,
      run_status: .run.status,
      outcome_status: (.outcome.status // null),
      delivery_id: $did,
      attach_status: (latest_attach.status // null),
      attach_sequence: (latest_attach.sequence // null),
      run_completed_sequence: (eventseq("run.completed"; "completed") // null),
      delivered_before_completed: (
        (latest_attach.status == "delivered") and
        ((latest_attach.sequence // 0) < (eventseq("run.completed"; "completed") // 999999999))
      ),
      sidecar_delivered_events: ([.events[] | select(.kind=="sidecar.capsule.delivered" and (.metadata.delivery_id==$did))] | length),
      marker_seen_in_provider_output: has_marker_in_provider_output,
      marker_seen_anywhere_in_trace: ([.. | strings | select(contains($marker))] | length > 0),
      attach_response: $attach_response,
      trace_file: $trace_file
    }
  '
