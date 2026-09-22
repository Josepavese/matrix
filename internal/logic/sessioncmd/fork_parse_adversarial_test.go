package sessioncmd

import "testing"

// TestParseForkRejectsMalformedInvocations keeps a mistyped command from
// silently becoming a different operation: a flag with no value must not swallow
// the target, and the "--" separator must protect the free-form input.
func TestParseForkRejectsMalformedInvocations(t *testing.T) {
	cases := map[string]struct {
		args       string
		target     string
		input      string
		makeActive *bool
	}{
		"empty":                {"", "", "", nil},
		"only a flag":          {"--async", "", "", nil},
		"target and input":     {"sess-1 -- hello world", "sess-1", "hello world", nil},
		"input keeps flags":    {"-- s1 --async", "", "s1 --async", nil},
		"unknown flag ignored": {"sess-2 --nope value", "sess-2 value", "", nil},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			invocation := ParseFork(tc.args)
			if invocation.Target != tc.target {
				t.Fatalf("target = %q, want %q", invocation.Target, tc.target)
			}
			if invocation.Input != tc.input {
				t.Fatalf("input = %q, want %q", invocation.Input, tc.input)
			}
		})
	}
}

// TestParseForkDistinguishesUnsetFromFalse keeps --make-active absent from being
// read as an explicit false: the caller uses the pointer to decide whether to
// change the active session at all.
func TestParseForkDistinguishesUnsetFromFalse(t *testing.T) {
	unset := ParseFork("sess")
	if unset.MakeActive != nil {
		t.Fatal("an absent flag must stay unset")
	}
	explicitFalse := ParseFork("--make-active=false")
	if explicitFalse.MakeActive == nil || *explicitFalse.MakeActive {
		t.Fatal("an explicit false must be distinguishable from unset")
	}
	explicitTrue := ParseFork("--make-active=true")
	if explicitTrue.MakeActive == nil || !*explicitTrue.MakeActive {
		t.Fatal("an explicit true must be recorded")
	}
	if ParseFork("--async").Async != true {
		t.Fatal("a bare flag must be recorded as true")
	}
}
