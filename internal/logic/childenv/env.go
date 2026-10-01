// Package childenv builds the environment of a child process Matrix starts for a
// decision of its own.
//
// Two such children exist today, and both need the same rule: a delivery
// validator the caller supplies, which must not be able to read the daemon's
// keys, and the git probe that decides whether a workspace may be granted, whose
// verdict must not be influenced by what the parent process happened to hold. One
// list in one place, because two policies that can diverge are a defect waiting
// to happen.
//
// The rule is an allowlist of environment NAMES: everything the daemon holds and
// this list does not name is simply absent from the child's view, so the property
// holds by construction instead of by remembering to unset each sensitive name as
// it appears. A child that needs a variable of its own reads it from its own
// configuration, never from the daemon's process.
//
// The list is the smallest one that lets a normal process run at all: PATH to
// find its own helpers, HOME for the operator's configuration, LANG and LC_ALL
// for its message locale, TZ for timestamps, TMPDIR for scratch files. Names that
// decide what a child acts on - the git redirection and configuration variables
// are the ones this repository met - stay out: a child either does not need them,
// or needs them to be decided by Matrix rather than inherited.
package childenv

import "os"

// Names is the whole environment a child of Matrix may see. It is a policy, not a
// convenience: a name that is not here does not reach a child, and a name that is
// here reaches every child.
var Names = []string{"PATH", "HOME", "LANG", "LC_ALL", "TZ", "TMPDIR"}

// Environment builds a child's environment from the allowlist alone. A name the
// daemon does not hold is simply absent from the result: an empty variable and an
// unset one are not the same thing for a program that asks.
func Environment() []string {
	env := make([]string, 0, len(Names))
	for _, name := range Names {
		if value, ok := os.LookupEnv(name); ok {
			env = append(env, name+"="+value)
		}
	}
	return env
}

// IsAllowedName reports whether a name is on the allowlist. Tests assert on this
// rule rather than on a copy of the list, so a name added to Names is a name every
// child can see, and no test can agree with a change the policy did not make.
func IsAllowedName(name string) bool {
	for _, allowed := range Names {
		if allowed == name {
			return true
		}
	}
	return false
}
