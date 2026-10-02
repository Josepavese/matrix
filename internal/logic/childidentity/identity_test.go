package childidentity

import (
	"os"
	"strings"
	"testing"
)

// TestARecycledPIDIsRefusedByTheStartTimeItShows is the acceptance evidence for
// the reuse window: the same pid observed twice is the same process only while
// the kernel reports the start time it reported first. The first observation
// arms the guard, the second — a different start time, which is what a recycled
// pid looks like — is refused instead of being reported as the child.
func TestARecycledPIDIsRefusedByTheStartTimeItShows(t *testing.T) {
	guard := &reuseGuard{parent: os.Getpid()}
	first := Identity{PID: 4242, ParentPID: os.Getpid(), StartTicks: 1000}

	if err := guard.accept(first); err != nil {
		t.Fatalf("the first observation of our own child was refused: %v", err)
	}
	// The other side of the property: the same process, observed again with the
	// start time it already showed, is still evidence.
	if err := guard.accept(first); err != nil {
		t.Fatalf("a second observation of the same process was refused: %v", err)
	}

	recycled := first
	recycled.StartTicks = 2000
	err := guard.accept(recycled)
	if err == nil {
		t.Fatal("a pid whose start time changed was accepted as the child the probe started")
	}
	if !strings.Contains(err.Error(), "recycled") {
		t.Fatalf("the refusal does not name what happened: %v", err)
	}
	var refused *refusedIdentityError
	if !asRefusal(err, &refused) {
		t.Fatalf("the refusal is not terminal, so the probe would keep polling: %T", err)
	}
}

// TestAnObservationOfAnotherProcessIsRefused pins the first of the two kernel
// facts: the child a probe starts is its own child, and a pid that now belongs
// to a process with another parent is not that child — however plausible its
// command line looks.
func TestAnObservationOfAnotherProcessIsRefused(t *testing.T) {
	guard := &reuseGuard{parent: os.Getpid()}
	err := guard.accept(Identity{PID: 4242, ParentPID: 1, StartTicks: 1000})
	if err == nil {
		t.Fatal("a process parented by init was accepted as the child this probe started")
	}
	if !strings.Contains(err.Error(), "parent") {
		t.Fatalf("the refusal does not name the parent: %v", err)
	}
	// A reference observed for another process must not arm the guard: the
	// start time has to belong to our own child.
	guard = &reuseGuard{parent: os.Getpid()}
	if err := guard.accept(Identity{PID: 4242, ParentPID: 1, StartTicks: 1000}); err == nil {
		t.Fatal("the refused observation armed the guard")
	}
	if guard.armed {
		t.Fatal("a refused observation armed the start-time reference")
	}
}

// TestStatFieldsReadsPositionsNotNames: the executable name in /proc/<pid>/stat
// is parenthesised and may contain spaces and parentheses of its own, so the
// parent and the start time are read by position after the last ')' — a name
// with a parenthesis in it must not shift them.
func TestStatFieldsReadsPositionsNotNames(t *testing.T) {
	const stat = "1234 (codex agent (v2)) S 4321 1234 1234 0 -1 4194560 100 0 0 0 5 0 0 0 20 0 1 0 987654 1234 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0"

	parent, startTicks, err := statFields(stat)
	if err != nil {
		t.Fatalf("statFields: %v", err)
	}
	if parent != 4321 {
		t.Fatalf("parent = %d, want 4321: the parent is field 4, read after the last parenthesis", parent)
	}
	if startTicks != 987654 {
		t.Fatalf("start time = %d, want 987654: the start time is field 22", startTicks)
	}
}

// TestMalformedStatIsRefused: an unreadable or truncated stat is an error, never
// a zero start time that would then be compared as if it were evidence.
func TestMalformedStatIsRefused(t *testing.T) {
	for _, stat := range []string{
		"",
		"1234 no parenthesis in this line",
		"1234 (agent) S 4321",
		"1234 (agent) S notanumber 1234 1234 0 -1 4194560 100 0 0 0 5 0 0 0 20 0 1 0 987654",
		"1234 (agent) S 4321 1234 1234 0 -1 4194560 100 0 0 0 5 0 0 0 20 0 1 0 notanumber",
	} {
		if _, _, err := statFields(stat); err == nil {
			t.Fatalf("malformed stat %q was read as evidence", stat)
		}
	}
}

// asRefusal reports whether the error is the terminal refusal type, without
// importing errors just for one assertion.
func asRefusal(err error, target **refusedIdentityError) bool {
	refused, ok := err.(*refusedIdentityError)
	if ok {
		*target = refused
	}
	return ok
}
