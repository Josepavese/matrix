package deliverycontract

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Josepavese/matrix/internal/logic/childenv"
)

// TestAValidatorCannotSeeTheDaemonsEnvironment pins the property that keeps the
// operator's keys away from code the caller supplies. The validator writes its
// own environment into the run workspace - which is exactly what it is allowed
// to do, and exactly why discarding its output is not a defence - and the
// sentinel the daemon holds must not be in what it wrote.
func TestAValidatorCannotSeeTheDaemonsEnvironment(t *testing.T) {
	const sentinel = "MATRIX_DAEMON_KEY_SENTINEL"
	t.Setenv(sentinel, "sk-live-should-never-reach-a-validator")
	t.Setenv("MATRIX_API_KEY", "sk-live-second-sentinel")

	workspace := t.TempDir()
	// A shell appears only because the caller wrote one into the argv.
	code, err := runValidatorProcess(context.Background(), workspace, []string{"sh", "-c", "env > envdump.txt; exit 0"})
	if err != nil {
		t.Fatalf("running the validator: %v", err)
	}
	if code != 0 {
		t.Fatalf("validator exit code = %d, want 0", code)
	}

	raw, err := os.ReadFile(filepath.Join(workspace, "envdump.txt"))
	if err != nil {
		t.Fatalf("the validator could not write its environment into the run workspace: %v", err)
	}
	dump := string(raw)

	if strings.Contains(dump, "sk-live-") {
		for _, line := range strings.Split(dump, "\n") {
			if strings.Contains(line, "sk-live-") {
				t.Fatalf("the validator read a daemon secret out of its environment: %s", line)
			}
		}
	}
	if strings.Contains(dump, sentinel+"=") {
		t.Fatalf("the validator saw %s, which only the daemon holds", sentinel)
	}
	if strings.Contains(dump, "MATRIX_API_KEY=") {
		t.Fatal("the validator saw the daemon's API key")
	}

	// The allowlist is not an empty environment: a validator may resolve what it
	// runs. If this fails the fix went too far and broke legitimate use.
	if !strings.Contains(dump, "PATH=") {
		t.Fatal("PATH is on the allowlist and must reach the validator")
	}
}

// TestTheValidatorEnvironmentIsAnAllowlistOfNames checks the shape rather than
// a particular secret: everything Matrix hands the child carries a name from
// the allowlist and nothing else. A denylist would need updating every time a
// new secret appears; this does not.
//
// The assertion sits on what Matrix hands over rather than on what the child
// ends up showing: a shell invents PWD and SHLVL for itself, and no test can
// tell those apart from a leak by looking at the child. The end-to-end half of
// the property - a real validator, a real leak attempt - is the test above.
func TestTheValidatorEnvironmentIsAnAllowlistOfNames(t *testing.T) {
	t.Setenv("SOME_FUTURE_OPERATOR_SECRET", "value")
	t.Setenv("MATRIX_DAEMON_API_KEY", "sk-live-another-sentinel")

	handed := map[string]bool{}
	for _, entry := range childenv.Environment() {
		name := entry
		if i := strings.IndexByte(entry, '='); i >= 0 {
			name = entry[:i]
		}
		handed[name] = true
		if !childenv.IsAllowedName(name) {
			t.Fatalf("Matrix handed the validator %q, which the allowlist does not name", name)
		}
	}
	for _, secret := range []string{"SOME_FUTURE_OPERATOR_SECRET", "MATRIX_DAEMON_API_KEY"} {
		if handed[secret] {
			t.Fatalf("Matrix handed the validator %q, which only the daemon holds", secret)
		}
	}
	if !handed["PATH"] {
		t.Fatal("PATH is on the allowlist and must reach the validator")
	}

	// The exact list is pinned where it lives: childenv holds the one decision a
	// validator and the git probe share, and its test is where changing it has to
	// be visible.
}
