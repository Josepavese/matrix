#!/usr/bin/env bash
# Host side of the Windows release validation.
#
# Why this exists: the guest cannot reach the workstation's filesystem, so the
# validation script has to be served over HTTP and its report collected by a POST.
# That was done by hand, with a throwaway Python server rewritten every time the
# temporary directory was cleaned; this keeps it in the repository, where it can be
# reviewed, re-run and self-tested.
#
# Usage:
#   tests/windows_validation_host.sh [--version vX.Y.Z] [--port 8000] [--timeout 900]
#   tests/windows_validation_host.sh --self-test
#
# It serves tests/windows_release_validation.ps1, prints the two lines to type in
# the guest console, waits for the report, and exits 0 only when every check passed.
set -uo pipefail

cd "$(dirname "$0")/.." || exit 2

VERSION=""
PORT=8000
TIMEOUT=900
SELF_TEST=0

while [ $# -gt 0 ]; do
	case "$1" in
	--version) VERSION="${2:-}"; shift 2 ;;
	--port) PORT="${2:-}"; shift 2 ;;
	--timeout) TIMEOUT="${2:-}"; shift 2 ;;
	--self-test) SELF_TEST=1; shift ;;
	-h | --help)
		sed -n '2,16p' "$0"
		exit 0
		;;
	*)
		echo "unknown argument: $1" >&2
		exit 2
		;;
	esac
done

SCRIPT="tests/windows_release_validation.ps1"
[ -f "$SCRIPT" ] || {
	echo "missing $SCRIPT" >&2
	exit 2
}

worker="$(mktemp -d)"
report="$worker/report.txt"
server_pid=""
cleanup() {
	[ -n "$server_pid" ] && kill "$server_pid" 2>/dev/null
	wait "$server_pid" 2>/dev/null
	rm -rf "$worker"
}
trap cleanup EXIT

# The guest reaches the host through the QEMU user-network gateway, so the server
# listens on every interface rather than loopback only.
cat >"$worker/serve.py" <<'PY'
import http.server, os, sys

root, report_path, port = sys.argv[1], sys.argv[2], int(sys.argv[3])

class Handler(http.server.BaseHTTPRequestHandler):
    def do_GET(self):
        # The guest asks for v.ps1; anything else under the repository root is
        # served too, so a patched installer can be exercised before release.
        name = os.path.basename(self.path.split("?")[0]) or "v.ps1"
        target = os.path.join(root, "tests", "windows_release_validation.ps1") if name == "v.ps1" else os.path.join(root, name)
        try:
            with open(target, "rb") as handle:
                body = handle.read()
        except OSError:
            self.send_response(404); self.end_headers(); return
        self.send_response(200)
        self.send_header("Content-Type", "text/plain")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def do_POST(self):
        length = int(self.headers.get("Content-Length", "0"))
        with open(report_path, "wb") as handle:
            handle.write(self.rfile.read(length))
        self.send_response(200); self.end_headers(); self.wfile.write(b"ok")

    def log_message(self, *args):
        pass

http.server.HTTPServer(("0.0.0.0", port), Handler).serve_forever()
PY

start_server() {
	python3 "$worker/serve.py" "$PWD" "$report" "$PORT" &
	server_pid=$!
	for _ in $(seq 1 20); do
		if curl -fsS -m 2 -o /dev/null "http://127.0.0.1:$PORT/v.ps1"; then
			return 0
		fi
		sleep 0.25
	done
	echo "the collector did not start on port $PORT" >&2
	return 1
}

# verdict reads the collected report and decides, so the same logic serves a real
# run and the self-test.
verdict() {
	local text
	text="$(cat "$report" 2>/dev/null || true)"
	if [ -z "$text" ]; then
		echo "no report was collected" >&2
		return 1
	fi
	printf '%s\n' "$text"
	local ok failed
	ok="$(printf '%s\n' "$text" | grep -c '^ok ')"
	failed="$(printf '%s\n' "$text" | grep -cE '^(FAIL|install\.ps1 threw|.* : )' )"
	echo "== summary =="
	echo "checks passed: $ok"
	echo "checks failed: $failed"
	if [ "$failed" -ne 0 ] || [ "$ok" -eq 0 ]; then
		echo "WINDOWS_VALIDATION_FAILED"
		return 1
	fi
	echo "WINDOWS_VALIDATION_OK"
	return 0
}

if [ "$SELF_TEST" = 1 ]; then
	# Drives the collector with a synthetic report, so the plumbing is verified on a
	# host that has no Windows guest at all.
	start_server || exit 1
	curl -fsS -m 5 -o /dev/null "http://127.0.0.1:$PORT/v.ps1" || {
		echo "self-test could not fetch the validation script" >&2
		exit 1
	}
	{
		echo "host_os=Self Test"
		for i in 1 2 3 4 5 6 7 8 9; do echo "ok   check $i -> yes"; done
	} | curl -fsS -m 5 --data-binary @- "http://127.0.0.1:$PORT/log" >/dev/null || {
		echo "self-test could not post a report" >&2
		exit 1
	}
	verdict || exit 1

	# And once with a failure, which must not pass.
	printf 'host_os=Self Test\nok   one -> yes\nmatrix.exe runs : boom\n' |
		curl -fsS -m 5 --data-binary @- "http://127.0.0.1:$PORT/log" >/dev/null
	if verdict >/dev/null 2>&1; then
		echo "self-test: a failing report must not be reported as valid" >&2
		exit 1
	fi
	echo "WINDOWS_VALIDATION_SELF_TEST_OK"
	exit 0
fi

game_version="${VERSION:-latest}"
echo "serving $SCRIPT on port $PORT"
echo
echo "In the Windows guest console, run:"
echo "  curl.exe -sS -o v.ps1 http://10.0.2.2:$PORT/v.ps1"
echo "  powershell -ExecutionPolicy Bypass -File v.ps1 -Version $game_version -ReportUrl http://10.0.2.2:$PORT/log"
echo
echo "waiting up to ${TIMEOUT}s for the report"
start_server || exit 1

waited=0
while [ ! -s "$report" ] && [ "$waited" -lt "$TIMEOUT" ]; do
	sleep 5
	waited=$((waited + 5))
done
verdict
