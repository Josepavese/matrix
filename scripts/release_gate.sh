#!/usr/bin/env bash
# Pre-release gate: it checks the commit that is about to be released, not the
# working tree that happens to be open in front of you.
#
# Why this file exists. The check that used to catch a broken release was a line
# someone remembered: archive HEAD, build it, and build it for the platforms you
# are not sitting at. It is the line that caught a commit whose Windows build was
# broken while every local check was green, and the one time it was skipped was
# the time the build broke. A check that depends on who remembers it is not a
# check, so it lives here, it runs on every push, and it fails loudly.
#
# The archive is the whole point: a working tree can build while the committed
# tree does not, and the release is cut from the committed tree. Every check
# below runs inside the export, never in the working tree.
#
# The lists this gate uses - the concurrency packages under the race detector and
# the coverage floors - are read out of scripts/quality_gate.sh in the exported
# tree, so there is one source of truth. If that file stops carrying them, this
# gate reports that it could not derive them and fails, rather than checking
# nothing and reporting success.
#
# Usage:
#   scripts/release_gate.sh                  # check HEAD; refuse a dirty tree
#   scripts/release_gate.sh --ref v0.1.48    # check another revision
#   scripts/release_gate.sh --allow-dirty    # check the commit anyway, and say so
#   scripts/release_gate.sh --keep           # keep the export on disk
#
# Exit status is 0 only when every check passes.

set -uo pipefail

cd "$(dirname "$0")/.." || exit 2

REF=HEAD
ALLOW_DIRTY=0
KEEP=0
while [ $# -gt 0 ]; do
	case "$1" in
	--ref)
		REF=${2:-}
		if [ -z "$REF" ]; then
			echo "--ref needs a revision" >&2
			exit 2
		fi
		shift 2
		;;
	--allow-dirty)
		ALLOW_DIRTY=1
		shift
		;;
	--keep)
		KEEP=1
		shift
		;;
	-h | --help)
		sed -n '2,27p' "$0" | sed 's/^# \{0,1\}//'
		exit 0
		;;
	*)
		echo "unknown flag: $1" >&2
		exit 2
		;;
	esac
done

FAILED=0
REPORT=$(mktemp)
step() { printf '\n\033[1m== %s\033[0m\n' "$1"; }
ok() {
	printf '   \033[32mok\033[0m %s\n' "$1"
	printf 'ok   %s\n' "$1" >>"$REPORT"
}
bad() {
	printf '   \033[31mFAIL\033[0m %s\n' "$1"
	printf 'FAIL %s\n' "$1" >>"$REPORT"
	FAILED=1
}
note() { printf '   \033[33mnote\033[0m %s\n' "$1"; }

# The two shared lists live in the quality gate. Deriving them from the exported
# tree keeps this gate and CI's gate describing the same thing.
race_packages() {
	sed -n 's/^RACE_PACKAGES="\(.*\)"$/\1/p' "$1/scripts/quality_gate.sh"
}
coverage_floors() {
	awk '/^COVERAGE_FLOORS=\$\(cat <</{inside=1; next} inside && /^EOF$/{exit} inside' "$1/scripts/quality_gate.sh"
}

SHA=$(git rev-parse --verify "$REF^{commit}" 2>/dev/null)
if [ -z "$SHA" ]; then
	echo "cannot resolve revision: $REF" >&2
	exit 2
fi
SUBJECT=$(git log -1 --format=%s "$SHA")
BRANCH=$(git rev-parse --abbrev-ref HEAD)

printf '\033[1mMatrix release gate\033[0m\n'
printf '   commit  %s\n' "$SHA"
printf '   subject %s\n' "$SUBJECT"
printf '   branch  %s (working tree)\n' "$BRANCH"

# The tree is the subject of the check. Changes that are not committed are not in
# it, and a gate that silently ignores them would repeat the mistake it exists to
# prevent: verifying one artifact and releasing another.
if [ -n "$(git status --porcelain)" ]; then
	if [ "$ALLOW_DIRTY" = 1 ]; then
		note "the working tree has uncommitted changes: they are NOT what this gate checks"
	else
		step "working tree"
		bad "the working tree has uncommitted changes: commit or stash them, or pass --allow-dirty to check $SHA anyway"
	fi
fi

EXPORT=$(mktemp -d "${TMPDIR:-/tmp}/matrix-release-gate.XXXXXX")
cleanup() { [ "$KEEP" = 1 ] || rm -rf "$EXPORT"; }
trap cleanup EXIT

step "archive: the tree that will be released is extracted, not read from the working tree"
if git archive "$SHA" | tar -x -C "$EXPORT"; then
	ok "git archive $SHA extracted into $EXPORT"
else
	bad "git archive $SHA failed: nothing below this line means anything"
	printf '\n\033[31mRELEASE_GATE_FAILED\033[0m the export could not be produced\n'
	exit 1
fi
if [ "$KEEP" = 1 ]; then
	note "the export is kept at $EXPORT"
fi

step "build ./... for every platform a release ships"
for target in linux/amd64 windows/amd64 darwin/arm64; do
	os=${target%/*}
	arch=${target#*/}
	BUILD_LOG=$(mktemp)
	if (cd "$EXPORT" && GOOS="$os" GOARCH="$arch" go build ./...) >"$BUILD_LOG" 2>&1; then
		ok "GOOS=$os GOARCH=$arch go build ./..."
	else
		bad "GOOS=$os GOARCH=$arch go build ./..."
		grep -vE '^(go: downloading|go: finding)' "$BUILD_LOG" | tail -12
	fi
	rm -f "$BUILD_LOG"
done

# `go build` does not compile test files, and today's failures included a test
# file that only compiled on one platform. `go vet` compiles them all.
step "vet ./... (the builds above do not compile test files)"
VET_LOG=$(mktemp)
if (cd "$EXPORT" && go vet ./...) >"$VET_LOG" 2>&1; then
	ok "go vet ./..."
else
	bad "go vet ./..."
	tail -12 "$VET_LOG"
fi
rm -f "$VET_LOG"

step "go test -count=1 -race on the packages that carry concurrency"
RACE=$(race_packages "$EXPORT")
if [ -z "$RACE" ]; then
	bad "cannot derive RACE_PACKAGES from scripts/quality_gate.sh in the export"
else
	RACE_LOG=$(mktemp)
	RACE_TARGETS=()
	for pkg in $RACE; do
		RACE_TARGETS+=("./$pkg")
	done
	if (cd "$EXPORT" && go test -count=1 -p 1 -race -timeout 20m "${RACE_TARGETS[@]}") >"$RACE_LOG" 2>&1; then
		ok "race detector clean on ${#RACE_TARGETS[@]} packages"
	else
		bad "race or test failures:"
		grep -E '^(FAIL|--- FAIL|WARNING: DATA RACE)' "$RACE_LOG" | head -20
	fi
	rm -f "$RACE_LOG"
fi

step "governance: manifest and code budget"
GOV_LOG=$(mktemp)
if (cd "$EXPORT" && go run ./scripts/governance_check --manifest governance/manifest.toml) >"$GOV_LOG" 2>&1; then
	ok "governance manifest satisfied"
else
	bad "governance manifest:"
	tail -12 "$GOV_LOG"
fi
rm -f "$GOV_LOG"
CODE_LOG=$(mktemp)
if (cd "$EXPORT" && go run ./scripts/code_governance.go -config code-governance.toml) >"$CODE_LOG" 2>&1; then
	ok "code governance"
else
	bad "code governance:"
	tail -12 "$CODE_LOG"
fi
rm -f "$CODE_LOG"

step "coverage ratchet: every floor in scripts/quality_gate.sh"
FLOORS=$(coverage_floors "$EXPORT")
if [ -z "$FLOORS" ]; then
	bad "cannot derive COVERAGE_FLOORS from scripts/quality_gate.sh in the export"
else
	COVER_LOG=$(mktemp)
	FLOOR_TARGETS=()
	while read -r pkg _floor; do
		[ -n "$pkg" ] && FLOOR_TARGETS+=("./$pkg")
	done <<<"$FLOORS"
	if (cd "$EXPORT" && go test -count=1 -cover "${FLOOR_TARGETS[@]}") >"$COVER_LOG" 2>&1; then
		while read -r pkg floor; do
			[ -n "$pkg" ] || continue
			actual=$(grep -E "^ok[[:space:]]+github.com/Josepavese/matrix/$pkg" "$COVER_LOG" | grep -oE 'coverage: [0-9.]+' | grep -oE '[0-9.]+' | head -1)
			if [ -z "$actual" ]; then
				bad "$pkg: no coverage reported"
			elif awk "BEGIN{exit !($actual < $floor)}"; then
				bad "$pkg $actual% is below the floor $floor%"
			else
				ok "$pkg $actual% (floor $floor%)"
			fi
		done <<<"$FLOORS"
	else
		bad "the packages under the ratchet did not all build and pass:"
		grep -E '^(FAIL|--- FAIL)' "$COVER_LOG" | head -20
	fi
	rm -f "$COVER_LOG"
fi

step "summary"
printf '   commit %s (%s)\n' "$SHA" "$SUBJECT"
sed 's/^/   /' "$REPORT"
rm -f "$REPORT"
if [ "$FAILED" = 1 ]; then
	printf '\n\033[31mRELEASE_GATE_FAILED\033[0m at least one check failed on %s\n' "$SHA"
	exit 1
fi
printf '\n\033[32mRELEASE_GATE_OK\033[0m every check passed on %s\n' "$SHA"
