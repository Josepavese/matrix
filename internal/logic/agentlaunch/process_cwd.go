package agentlaunch

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Josepavese/matrix/internal/middleware"
)

// ProcessCwdEnv declares where an agent's process must start, as distinct from
// the workspace its sessions are told about.
//
// The two are not the same thing and must not be collapsed: a session cwd is a
// statement the agent makes to its peer (a workspace it may serve virtually),
// while the process cwd is where the operating system actually starts the
// child. A provider that refuses a path outside its own process cwd — the
// original report for MiMo — needs the second one.
//
// The declaration lives on the endpoint, so it is per agent and opt-in. An
// endpoint that declares nothing starts wherever the caller starts, which keeps
// the daemon's own directory the default instead of pinning every agent to a
// checkout.
const ProcessCwdEnv = "MATRIX_AGENT_PROCESS_CWD"

// DeclaredProcessCwd reads the process cwd an endpoint declares. An endpoint
// that declares nothing returns "", which means "do not choose for me" rather
// than "choose something".
func DeclaredProcessCwd(endpoint middleware.ProtocolEndpoint) string {
	return declaredEnvValue(endpoint.Env, ProcessCwdEnv)
}

// declaredEnvValue reads one environment entry by exact key. A differently
// spelled variable that happens to end in the same words is not a declaration.
func declaredEnvValue(env []string, key string) string {
	value := ""
	for _, entry := range env {
		name, entryValue, ok := strings.Cut(entry, "=")
		if ok && strings.TrimSpace(name) == key {
			value = entryValue
		}
	}
	return strings.TrimSpace(value)
}

// ResolveProcessCwd turns a declared process cwd into the directory to start the
// child in, and refuses rather than guessing.
//
// A declared value must be an absolute path to a directory that exists. It is
// never repaired, never resolved against something else, and never replaced by
// the caller's directory or the filesystem root: a child that starts in the
// wrong place is exactly the failure this declaration exists to prevent, so an
// unusable value stops the launch and says what to fix.
func ResolveProcessCwd(endpoint middleware.ProtocolEndpoint) (string, error) {
	declared := DeclaredProcessCwd(endpoint)
	if declared == "" {
		return "", nil
	}
	if !filepath.IsAbs(declared) {
		return "", fmt.Errorf("%s must be an absolute path, got %q", ProcessCwdEnv, declared)
	}
	info, err := os.Stat(declared)
	if err != nil {
		return "", fmt.Errorf("%s %q is not usable: %w", ProcessCwdEnv, declared, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%s %q is not a directory", ProcessCwdEnv, declared)
	}
	resolved, err := filepath.EvalSymlinks(declared)
	if err != nil {
		return "", fmt.Errorf("%s %q cannot be resolved: %w", ProcessCwdEnv, declared, err)
	}
	return resolved, nil
}
