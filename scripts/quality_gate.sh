#!/usr/bin/env bash
# Quality gate for the Matrix repository.
#
# The gate is a ratchet, not an aspiration: every threshold is a floor that must
# not regress, and the Tier A table exists because a percentage on a parser is
# worth less than a percentage on the protocol and the vault.
#
# Usage:
#   scripts/quality_gate.sh            # formatting, vet, build, race tests, coverage ratchet, governance
#   scripts/quality_gate.sh --static   # adds golangci-lint and govulncheck when installed
#   scripts/quality_gate.sh --fuzz     # adds a short fuzz campaign per target
#   scripts/quality_gate.sh --mutation # adds mutation testing on Tier A (slow)
#   scripts/quality_gate.sh --all      # everything
#   scripts/quality_gate.sh --light    # resource-constrained host: no repo-wide race run
#
# Exit status is 0 only when every selected check passes.

set -uo pipefail

cd "$(dirname "$0")/.." || exit 2

RUN_STATIC=0
RUN_FUZZ=0
RUN_MUTATION=0
LIGHT=0
for arg in "$@"; do
	case "$arg" in
	--static) RUN_STATIC=1 ;;
	--fuzz) RUN_FUZZ=1 ;;
	--mutation) RUN_MUTATION=1 ;;
	--all)
		RUN_STATIC=1
		RUN_FUZZ=1
		RUN_MUTATION=1
		;;
	--light)
		# For hosts where a repository-wide race run does not fit: the race
		# detector is applied to the packages that carry concurrency instead.
		LIGHT=1
		;;
	-h | --help)
		sed -n '2,20p' "$0" | sed 's/^# \{0,1\}//'
		exit 0
		;;
	*)
		echo "unknown flag: $arg" >&2
		exit 2
		;;
	esac
done

FAILED=0
step() { printf '\n\033[1m== %s\033[0m\n' "$1"; }
ok() { printf '   \033[32mok\033[0m %s\n' "$1"; }
bad() {
	printf '   \033[31mFAIL\033[0m %s\n' "$1"
	FAILED=1
}

# --- formatting ---------------------------------------------------------------
step "gofmt"
UNFORMATTED=$(gofmt -l . 2>/dev/null | grep -v '^$' || true)
if [ -n "$UNFORMATTED" ]; then
	bad "unformatted files:"
	echo "$UNFORMATTED"
else
	ok "all files formatted"
fi

# --- vet and build ------------------------------------------------------------
# Both tools write progress and download notices to stderr, so "any output" is
# not a failure signal: only a diagnostic in file:line:col form, or a non-zero
# exit with no diagnostics (the tool itself failed), are.
step "go vet"
VET_LOG=$(mktemp)
go vet ./... >"$VET_LOG" 2>&1
VET_STATUS=$?
if [ "$VET_STATUS" = 0 ]; then
	ok "vet clean"
elif grep -qE '^[^[:space:]]+\.go:[0-9]+:[0-9]+:' "$VET_LOG"; then
	bad "go vet findings:"
	grep -E '^[^[:space:]]+\.go:[0-9]+:[0-9]+:' "$VET_LOG" | head -15
else
	bad "go vet did not complete:"
	tail -5 "$VET_LOG"
fi
rm -f "$VET_LOG"

step "go build"
BUILD_LOG=$(mktemp)
if go build ./... >"$BUILD_LOG" 2>&1; then
	ok "build clean"
else
	bad "build failed:"
	grep -vE '^(go: downloading|go: finding)' "$BUILD_LOG" | tail -15
fi
rm -f "$BUILD_LOG"

# --- tests --------------------------------------------------------------------
# Concurrency-bearing packages get the race detector even in --light mode; they
# are the ones where a race can actually be introduced.
RACE_PACKAGES="internal/logic/elicitation internal/logic/runtrace internal/logic/session internal/providers/agents internal/providers/telegram internal/providers/runapi pkg/zedacp pkg/zedacpstdio internal/logic/rundelivery"
if [ "$LIGHT" = 1 ]; then
	step "go test (light: race on concurrency packages, plain elsewhere)"
	TEST_LOG=$(mktemp)
	if go test -p 2 ./... >"$TEST_LOG" 2>&1; then
		ok "$(grep -c '^ok' "$TEST_LOG") packages passed"
	else
		bad "test failures:"
		grep -E '^(FAIL|--- FAIL)' "$TEST_LOG" | head -20
	fi
	rm -f "$TEST_LOG"
	step "go test -race (concurrency packages)"
	RACE_LOG=$(mktemp)
	if go test -p 1 -race $(echo "$RACE_PACKAGES" | sed 's#[^ ]*#./&#g') >"$RACE_LOG" 2>&1; then
		ok "race detector clean on $(echo "$RACE_PACKAGES" | wc -w) packages"
	else
		bad "race or test failures:"
		grep -E '^(FAIL|--- FAIL|WARNING: DATA RACE)' "$RACE_LOG" | head -20
	fi
	rm -f "$RACE_LOG"
else
	step "go test -race"
	TEST_LOG=$(mktemp)
	if go test -race ./... >"$TEST_LOG" 2>&1; then
		ok "$(grep -c '^ok' "$TEST_LOG") packages passed"
	else
		bad "test failures:"
		grep -E '^(FAIL|--- FAIL)' "$TEST_LOG" | head -20
	fi
	rm -f "$TEST_LOG"
fi

# --- coverage ratchet ---------------------------------------------------------
# Tier A (protocol, SSOT, security, channel frontends) plus the module's
# remaining functional packages. Floors are the measured values at the time the
# gate was introduced; lower them only with a deliberate decision, never to make
# a change pass.
step "coverage ratchet"
COVERAGE_FLOORS=$(cat <<'EOF'
internal/middleware 93
internal/logic/elicitation 97
internal/logic/rundelivery 85
internal/logic/runconfig 100
internal/logic/runtrace 77
internal/logic/vaultsec 83
internal/logic/session 70
internal/logic/workspace 58
internal/providers/agents 70
internal/providers/runapi 69
internal/providers/telegram 54
pkg/zedacp 68
pkg/zedacpstdio 70
EOF
)
COVER_LOG=$(mktemp)
go test -cover $(echo "$COVERAGE_FLOORS" | awk '{printf "./%s ", $1}') >"$COVER_LOG" 2>&1
while read -r pkg floor; do
	actual=$(grep -E "^ok[[:space:]]+github.com/Josepavese/matrix/$pkg" "$COVER_LOG" | grep -oE 'coverage: [0-9.]+' | grep -oE '[0-9.]+' | head -1)
	if [ -z "$actual" ]; then
		bad "$pkg: no coverage reported"
		continue
	fi
	if awk -v a="$actual" -v f="$floor" 'BEGIN { exit !(a + 0 >= f + 0) }'; then
		ok "$pkg ${actual}% (floor ${floor}%)"
	else
		bad "$pkg ${actual}% is below the floor ${floor}%"
	fi
done <<<"$COVERAGE_FLOORS"
rm -f "$COVER_LOG"

# --- fuzz campaigns -----------------------------------------------------------
if [ "$RUN_FUZZ" = 1 ]; then
	step "fuzz targets (short campaigns)"
	# Every decoder that consumes external bytes has a target here. The list is
	# explicit so a new decoder without a target is visible in review.
	FUZZ_TARGETS=$(cat <<'EOF'
./pkg/zedacp FuzzDecodeInboundRaw
./pkg/zedacp FuzzJSONRPCIDInt64
./pkg/zedacp FuzzDecodeUpdateContent
./pkg/zedacp FuzzSessionNotificationDecode
./pkg/zedacp FuzzDecodeOptionalResult
./pkg/zedacpstdio FuzzReadBoundedLine
./internal/providers/agents FuzzWireToNeutralElicitation
./internal/providers/agents FuzzValidateElicitationValues
./internal/providers/agents FuzzFlexibleRequestID
./internal/providers/telegram FuzzDecodeCallbackData
./internal/providers/telegram FuzzCoerceElicitationValue
./internal/providers/telegram FuzzRedactURLUserinfo
./internal/providers/runapi FuzzElicitationAnswerEndpoint
./internal/providers/runapi FuzzElicitationPendingSerialisation
EOF
)
	FUZZ_TIME=${FUZZ_TIME:-20s}
	while read -r pkg target; do
		if out=$(go test "$pkg" -run 'XXX' -fuzz "^${target}$" -fuzztime "$FUZZ_TIME" 2>&1); then
			ok "$target"
		else
			bad "$target:"
			echo "$out" | grep -vE '^20[0-9]{2}/' | tail -8
		fi
	done <<<"$FUZZ_TARGETS"
fi

# --- mutation testing ---------------------------------------------------------
if [ "$RUN_MUTATION" = 1 ]; then
	step "mutation testing (Tier A)"
	GREMLINS=$(command -v gremlins || echo "$(go env GOPATH)/bin/gremlins")
	if [ ! -x "$GREMLINS" ]; then
		bad "gremlins not installed: go install github.com/go-gremlins/gremlins/cmd/gremlins@latest"
	else
		# threshold-efficacy is the share of reachable mutants the tests kill.
		# A survivor means a test that asserts less than it appears to.
		MUTATION_PACKAGES="internal/middleware internal/logic/elicitation internal/logic/rundelivery internal/logic/runconfig internal/logic/vaultsec internal/logic/runtrace pkg/zedacp pkg/zedacpstdio internal/providers/runapi internal/providers/telegram"
		for pkg in $MUTATION_PACKAGES; do
			if out=$("$GREMLINS" unleash --workers 4 --test-cpu 2 --threshold-efficacy 80 --threshold-mcover 60 "./$pkg/" 2>&1); then
				ok "$pkg $(echo "$out" | grep -E '^Test efficacy' | tail -1)"
			else
				bad "$pkg mutation thresholds not met:"
				echo "$out" | grep -E '^(Lived|Test efficacy|Mutator coverage)' | tail -3
			fi
		done
	fi
fi

# --- static analysis ----------------------------------------------------------
if [ "$RUN_STATIC" = 1 ]; then
	step "golangci-lint"
	if command -v golangci-lint >/dev/null 2>&1; then
		golangci-lint run ./... >/tmp/matrix_lint.log 2>&1
		LINT_STATUS=$?
		LINT_FINDINGS=$(grep -cE '^[^[:space:]]+\.go:[0-9]+:[0-9]+:' /tmp/matrix_lint.log || true)
		if [ "$LINT_STATUS" = 0 ]; then
			ok "lint clean"
		elif [ "${LINT_FINDINGS:-0}" -gt 0 ]; then
			bad "lint findings ($LINT_FINDINGS):"
			grep -E '^[^[:space:]]+\.go:[0-9]+:[0-9]+:' /tmp/matrix_lint.log | head -15
		else
			# A crashed linter is not a clean run, but it is also not a finding.
			# It usually means the binary was built with another Go toolchain.
			bad "golangci-lint did not complete (rebuild it with the toolchain on PATH):"
			tail -5 /tmp/matrix_lint.log
		fi
	else
		printf '   skipped (not installed)\n'
	fi

	step "govulncheck"
	if command -v govulncheck >/dev/null 2>&1; then
		govulncheck ./... >/tmp/matrix_vuln.log 2>&1
		VULN_STATUS=$?
		if [ "$VULN_STATUS" = 0 ]; then
			ok "no known vulnerabilities"
		elif grep -q 'Vulnerability #' /tmp/matrix_vuln.log; then
			bad "vulnerability findings:"
			grep -E 'Vulnerability #|Your code is affected' /tmp/matrix_vuln.log | head -10
		else
			bad "govulncheck did not complete:"
			tail -5 /tmp/matrix_vuln.log
		fi
	else
		printf '   skipped (not installed)\n'
	fi
fi

# --- repository governance ----------------------------------------------------
step "governance check"
if go run ./scripts/governance_check --manifest governance/manifest.toml 2>&1 | tail -3 | grep -q GOVERNANCE_CHECK_OK; then
	ok "governance manifest satisfied"
else
	bad "governance check failed"
fi

printf '\n'
if [ "$FAILED" = 0 ]; then
	printf '\033[32mQUALITY_GATE_OK\033[0m\n'
	exit 0
fi
printf '\033[31mQUALITY_GATE_FAILED\033[0m\n'
exit 1
