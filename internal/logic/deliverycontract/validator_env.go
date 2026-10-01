package deliverycontract

import "os"

// validatorEnvNames is the whole environment a validator is allowed to see.
//
// The validator is code the caller supplies and Matrix executes. The daemon's
// environment carries the operator's keys - the matrix and daemon API keys, the
// vault passphrase, the agent credentials the onboarding wrote - and inheriting
// it would hand all of that to any validator that writes its environment into
// the run workspace. Discarding the child's output does not help against that:
// the workspace is exactly where a validator is expected to write.
//
// This is an allowlist of environment NAMES, which is a general property of
// running untrusted code and not a decision about any agent or provider. A
// validator that needs a variable of its own has to read it from its own
// configuration, not from the daemon's process.
var validatorEnvNames = []string{"PATH", "HOME", "LANG", "LC_ALL", "TZ", "TMPDIR"}

// validatorEnv builds the child's environment from the allowlist alone. Anything
// the daemon holds and the list does not name is simply absent from the child's
// view, so the leak is closed by construction rather than by remembering to
// unset the sensitive names as they appear.
func validatorEnv() []string {
	out := make([]string, 0, len(validatorEnvNames))
	for _, name := range validatorEnvNames {
		if value, ok := os.LookupEnv(name); ok {
			out = append(out, name+"="+value)
		}
	}
	return out
}

// isAllowedValidatorEnvName reports whether a name is on the allowlist. The
// test that walks the child's environment uses it so the assertion is about the
// rule and not about a hardcoded list inside the test.
func isAllowedValidatorEnvName(name string) bool {
	for _, allowed := range validatorEnvNames {
		if allowed == name {
			return true
		}
	}
	return false
}
