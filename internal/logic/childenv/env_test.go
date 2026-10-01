package childenv

import (
	"os"
	"strings"
	"testing"
)

// TestTheAllowlistIsPinnedAsData keeps the one decision both children depend on
// visible. Without this, adding a name to Names would make every behavioural test
// agree with the change - the predicate would simply answer true for the new name
// - and a leak or a moved verdict would arrive with the suite green.
func TestTheAllowlistIsPinnedAsData(t *testing.T) {
	want := []string{"PATH", "HOME", "LANG", "LC_ALL", "TZ", "TMPDIR"}
	if len(Names) != len(want) {
		t.Fatalf("the child allowlist is %v, want %v", Names, want)
	}
	for i := range want {
		if Names[i] != want[i] {
			t.Fatalf("the child allowlist is %v, want %v", Names, want)
		}
	}
}

// TestEnvironmentCarriesTheAllowlistAndNothingElse checks the builder against the
// rule: every entry it hands over is a name the policy allows, a name the daemon
// holds reaches the child, and a name it does not hold stays absent instead of
// arriving empty.
func TestEnvironmentCarriesTheAllowlistAndNothingElse(t *testing.T) {
	t.Setenv("PATH", "/usr/bin:/bin")
	t.Setenv("SOME_FUTURE_OPERATOR_SECRET", "sk-live-sentinel")
	previousTmpdir, hadTmpdir := os.LookupEnv("TMPDIR")
	if err := os.Unsetenv("TMPDIR"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if hadTmpdir {
			_ = os.Setenv("TMPDIR", previousTmpdir)
		}
	})

	handed := map[string]string{}
	for _, entry := range Environment() {
		name, value := entry, ""
		if i := strings.IndexByte(entry, '='); i >= 0 {
			name, value = entry[:i], entry[i+1:]
		}
		handed[name] = value
		if !IsAllowedName(name) {
			t.Fatalf("the child was handed %q, which the allowlist does not name", name)
		}
	}
	if handed["PATH"] != "/usr/bin:/bin" {
		t.Fatalf("PATH must reach the child, got %q", handed["PATH"])
	}
	if _, found := handed["SOME_FUTURE_OPERATOR_SECRET"]; found {
		t.Fatal("a secret only the daemon holds reached the child")
	}
	if _, found := handed["TMPDIR"]; found {
		t.Fatal("a name the daemon does not hold must stay absent, not arrive empty")
	}
	if IsAllowedName("SOME_FUTURE_OPERATOR_SECRET") {
		t.Fatal("the rule allowed a name the policy does not name")
	}
}
