package agents

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Josepavese/matrix/internal/middleware"
)

// cwdProbeScript is the smallest peer that answers an ACP initialize and reports
// the working directory it was really started in. It is deliberately generic:
// the script knows nothing about which agent Matrix thinks it is talking to, and
// nothing in Matrix knows which program this is. That is the point of the file:
// the child directory must come from the run, not from a name.
//
// It derives everything from its own location, so the same script can live in
// any workspace: $0 is `<workspace>/cwd-probe.sh` and the report is written
// beside it, inside the directory the process really runs in.
const cwdProbeScript = `#!/bin/sh
pwd -P > "${0%/*}/child-cwd.txt"
i=1
while IFS= read -r line; do
  case "$line" in
    *'"initialize"'*)
      printf '{"jsonrpc":"2.0","id":%s,"result":{"protocolVersion":1,"agentCapabilities":{}}}\n' "$i"
      i=$((i+1)) ;;
    *'"session/new"'*)
      printf '{"jsonrpc":"2.0","id":%s,"result":{"sessionId":"probe-session-%s"}}\n' "$i" "$$"
      i=$((i+1)) ;;
    *'"session/resume"'*)
      printf '{"jsonrpc":"2.0","id":%s,"result":{}}\n' "$i"
      i=$((i+1)) ;;
    *'"session/prompt"'*)
      printf '{"jsonrpc":"2.0","id":%s,"result":{"stopReason":"end_turn"}}\n' "$i"
      i=$((i+1)) ;;
  esac
done
`

// namedEndpointResolver answers the router with one endpoint for every agent id,
// so the proof never needs a registry and never special-cases an agent name.
type namedEndpointResolver struct{}

func (namedEndpointResolver) GetAgentEndpoint(string) (middleware.ProtocolEndpoint, error) {
	return middleware.ProtocolEndpoint{
		Kind: middleware.ProtocolKindACP, Transport: "stdio", Command: os.Getenv(cwdProbeEnv),
	}, nil
}

// cwdProbeEnv names the peer program to start. It is an environment variable
// rather than a package variable so each test owns its own copy: the resolver
// stays stateless and the peer is never left behind by another test.
const cwdProbeEnv = "MATRIX_CWD_PROBE"

// prepareCwdProbe plants the peer inside one workspace, points the resolver at
// that copy, and clears the workspace report, so a report found after a turn can
// only have been written by that turn.
func prepareCwdProbe(t *testing.T, workspace string) {
	t.Helper()

	script := filepath.Join(workspace, "cwd-probe.sh")
	if err := os.WriteFile(script, []byte(cwdProbeScript), 0o755); err != nil {
		t.Fatalf("writing probe script in %s: %v", workspace, err)
	}
	t.Setenv(cwdProbeEnv, script)
	_ = os.Remove(reportPathFor(workspace))
}

// reportPathFor is where the peer writes the working directory of the process
// it runs in: beside itself, inside the directory it was started in.
func reportPathFor(workspace string) string {
	return filepath.Join(workspace, "child-cwd.txt")
}

// readChildCwd reads what the child process reported and resolves it, because
// the operating system hands back a canonical path and the test compares paths.
func readChildCwd(t *testing.T, workspace string) string {
	t.Helper()

	reported, err := os.ReadFile(reportPathFor(workspace))
	if err != nil {
		t.Fatalf("the child never reported its working directory in %s: %v", workspace, err)
	}
	resolved, err := filepath.EvalSymlinks(strings.TrimSpace(string(reported)))
	if err != nil {
		t.Fatalf("child working directory %q does not resolve: %v", reported, err)
	}
	return resolved
}

// newCwdProbeRouter builds the production router around the probe endpoint.
func newCwdProbeRouter() *Router {
	router := NewRouter(namedEndpointResolver{})
	router.SetFS(nil, "")
	return router
}

// runCwdProbeTurn asks the router for one turn on the given workspace, which is
// the same call the runtime API makes for a run.
func runCwdProbeTurn(t *testing.T, router *Router, workspace, message string) string {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	output, remoteSessionID, _, _, err := router.Route(ctx, middleware.RouteRequest{
		AgentID: "cwd-probe", WorkspacePath: workspace, Message: message,
	})
	if err != nil {
		t.Fatalf("route on %s: %v", workspace, err)
	}
	if output != "" {
		t.Logf("probe output on %s: %q", workspace, output)
	}
	return remoteSessionID
}

// TestChildProcessRunsInTheRunWorkspace is the core of issues 9 and 10: the
// agent program must be started in the workspace of the run. Two runs on two
// directory trees must produce two children, each in its own tree, and a run in
// one workspace must not move the session of the other. Reverting the working
// directory out of the transport makes each child report the directory it
// inherited instead, and this test fails on the first comparison.
func TestChildProcessRunsInTheRunWorkspace(t *testing.T) {
	workspaces := twoWorkspacesOnDifferentMounts(t)
	prepareCwdProbe(t, workspaces[0])
	router := newCwdProbeRouter()
	defer router.Close()

	sessions := make([]string, len(workspaces))
	var secondReport string
	for i, workspace := range workspaces {
		if i > 0 {
			prepareCwdProbe(t, workspace)
		}
		sessions[i] = runCwdProbeTurn(t, router, workspace, "where are you?")

		expected, err := filepath.EvalSymlinks(workspace)
		if err != nil {
			t.Fatalf("workspace %q does not resolve: %v", workspace, err)
		}
		cwd := readChildCwd(t, workspace)
		if cwd != expected {
			t.Fatalf("run %d on %s: the child started in %s, want the run workspace",
				i+1, mountOf(t, workspace), cwd)
		}
		if i == 1 {
			secondReport = cwd
		}
		t.Logf("workspace %d (%s): child cwd %s", i+1, mountOf(t, workspace), expected)
	}

	// Each workspace kept its own session: the second run did not inherit the
	// first one's remote session, because the client is keyed by agent and
	// workspace, not by agent alone.
	for i := range workspaces {
		if sessions[i] == "" {
			t.Fatalf("run %d on %s returned no remote session", i+1, workspaces[i])
		}
	}
	if len(workspaces) == 2 && sessions[0] == sessions[1] {
		t.Fatalf("two workspaces shared the remote session %q, workspaces must not share a provider process", sessions[0])
	}

	// Reuse: a later run on the first workspace continues the session that
	// workspace already holds, and does not reach for the other workspace's
	// process. The peer reports its pid in the session id, so a continued
	// session is provable rather than assumed.
	firstReport := readChildCwd(t, workspaces[0])
	reused := runCwdProbeTurn(t, router, workspaces[0], "still there?")
	if reused != sessions[0] {
		t.Fatalf("reused run session = %q, want the workspace-bound session %q", reused, sessions[0])
	}
	if cwd := readChildCwd(t, workspaces[0]); cwd != firstReport {
		t.Fatalf("reused run reported child cwd %s, want the session's directory %s", cwd, firstReport)
	}
	if cwd := readChildCwd(t, workspaces[1]); cwd != secondReport {
		t.Fatalf("reuse on %s disturbed the other workspace: child cwd %s, want %s", mountOf(t, workspaces[0]), cwd, secondReport)
	}
}

// TestChildProcessRefusesUnusableWorkspaceBeforeFork pins the other half of the
// contract: the workspace is checked before a process exists, and the refusal
// names the three paths that have to agree instead of leaving an opaque start
// failure. The peer would report if it had ever started.
func TestChildProcessRefusesUnusableWorkspaceBeforeFork(t *testing.T) {
	root := t.TempDir()
	prepareCwdProbe(t, root)
	report, script := reportPathFor(root), os.Getenv(cwdProbeEnv)

	missing := filepath.Join(root, "absent-workspace")
	_, err := createTransport(context.Background(), transportSpec{
		Protocol: "stdio", Command: script, Args: []string{"acp", "--cwd", missing}, Cwd: missing,
	})
	if err == nil {
		t.Fatal("an absent run workspace must refuse the launch, not start a child that fails later")
	}
	message := err.Error()
	for _, want := range []string{missing, script, "acp", "--cwd", "child working directory"} {
		if !strings.Contains(message, want) {
			t.Fatalf("the refusal must name %q, got %v", want, err)
		}
	}
	if _, statErr := os.Stat(report); !os.IsNotExist(statErr) {
		t.Fatalf("no child may be started for an unusable workspace, probe report stat error = %v", statErr)
	}

	aFile, notDir := filepath.Join(root, "a-file"), filepath.Join(root, "not-a-directory")
	if err := os.WriteFile(aFile, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := createTransport(context.Background(), transportSpec{Protocol: "stdio", Command: script, Cwd: aFile}); err == nil {
		t.Fatal("a file is not a workspace and must be refused")
	}
	if _, err := createTransport(context.Background(), transportSpec{Protocol: "stdio", Command: script, Cwd: notDir}); err == nil {
		t.Fatalf("a missing workspace %s must be refused", notDir)
	}

	// The same call with a usable workspace still starts the child: the gate
	// refuses unusable directories, not launches.
	transport, err := createTransport(context.Background(), transportSpec{Protocol: "stdio", Command: script, Cwd: root})
	if err != nil {
		t.Fatalf("a usable workspace must still launch: %v", err)
	}
	_ = transport.Close()
}

// TestChildProcessRefusesWorkspaceTheAgentWasToldToIgnore pins the third path of
// the issue: the directory the agent's own launch arguments name must agree with
// the run workspace. `env -C <dir>` is the documented workaround that pins a
// provider to one checkout; when a run asks for another workspace, the child
// would run in the run's directory while the agent was told to use the pinned
// one, and the refusal has to say so with all three paths instead of forking
// that process. The same holds when the declared directory does not exist.
func TestChildProcessRefusesWorkspaceTheAgentWasToldToIgnore(t *testing.T) {
	root := t.TempDir()
	prepareCwdProbe(t, root)
	script := os.Getenv(cwdProbeEnv)
	pinned := filepath.Join(root, "pinned-checkout")
	runWorkspace := filepath.Join(root, "run-workspace")
	for _, path := range []string{pinned, runWorkspace} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	_, err := createTransport(context.Background(), transportSpec{
		Protocol: "stdio", Command: "/usr/bin/env",
		Args: []string{"-C", pinned, script, "acp", "--cwd", pinned}, Cwd: runWorkspace,
	})
	if err == nil {
		t.Fatal("a child told to ignore the run workspace must not be forked")
	}
	for _, want := range []string{runWorkspace, pinned, "/usr/bin/env", "agent workspace"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("the refusal must name %q, got %v", want, err)
		}
	}
	if _, statErr := os.Stat(reportPathFor(runWorkspace)); !os.IsNotExist(statErr) {
		t.Fatalf("no child may be started when the workspaces disagree, report stat error = %v", statErr)
	}

	// A declared directory that does not exist is not a workspace claim and
	// must not block a run on its own: the run workspace still decides.
	if _, err := createTransport(context.Background(), transportSpec{
		Protocol: "stdio", Command: script, Args: []string{"acp", "--cwd", filepath.Join(root, "absent")}, Cwd: pinned,
	}); err != nil {
		t.Fatalf("a declared directory that does not exist must not refuse the launch: %v", err)
	}

	// The declared workspace and the run workspace agreeing is a valid launch,
	// and so is an endpoint that declares no directory at all.
	for name, spec := range map[string]transportSpec{
		"agreeing declared workspace": {Protocol: "stdio", Command: script, Args: []string{"acp", "--cwd=" + pinned}, Cwd: pinned},
		"no declared workspace":       {Protocol: "stdio", Command: script, Args: []string{"acp"}, Cwd: pinned},
	} {
		transport, err := createTransport(context.Background(), spec)
		if err != nil {
			t.Fatalf("%s must launch: %v", name, err)
		}
		_ = transport.Close()
	}
}

// TestDeclaredWorkspaceDirReadsArgumentForms pins the parser as data, not as a
// table of agents: the same spellings behave the same for every program.
func TestDeclaredWorkspaceDirReadsArgumentForms(t *testing.T) {
	cases := []struct {
		name  string
		args  []string
		want  string
		found bool
	}{
		{name: "cwd flag and value", args: []string{"acp", "--cwd", "/tmp/ws"}, want: "/tmp/ws", found: true},
		{name: "cwd flag equals value", args: []string{"acp", "--cwd=/tmp/ws"}, want: "/tmp/ws", found: true},
		{name: "chdir short flag", args: []string{"-C", "/tmp/ws"}, want: "/tmp/ws", found: true},
		{name: "env pins the directory", args: []string{"-C", "/tmp/ws", "agent", "acp"}, want: "/tmp/ws", found: true},
		{name: "no directory declared", args: []string{"acp", "--log-level", "warn"}, found: false},
		{name: "empty arguments", args: nil, found: false},
		{name: "flag without a value", args: []string{"acp", "--cwd"}, found: false},
		{name: "empty value is not a claim", args: []string{"--cwd="}, found: false},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			got, found := declaredWorkspaceDir(test.args)
			if found != test.found || got != test.want {
				t.Fatalf("declaredWorkspaceDir(%v) = %q, %v; want %q, %v", test.args, got, found, test.want, test.found)
			}
		})
	}
}

// TestChildProcessCwdIsNotInheritedFromTheDaemon is the negative control for the
// two-workspace proof: when the daemon's own directory differs from the run
// workspace, the child must not report the daemon directory. Without it, the
// positive test could pass because the harness happened to run in the workspace.
func TestChildProcessCwdIsNotInheritedFromTheDaemon(t *testing.T) {
	daemon, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	workspace := filepath.Join(root, "run-workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	prepareCwdProbe(t, workspace)

	router := newCwdProbeRouter()
	defer router.Close()
	runCwdProbeTurn(t, router, workspace, "where are you?")

	expected, err := filepath.EvalSymlinks(workspace)
	if err != nil {
		t.Fatal(err)
	}
	reported := readChildCwd(t, workspace)
	if reported == filepath.Clean(daemon) && reported != expected {
		t.Fatalf("the child ran in the daemon directory %s: its working directory was inherited, not governed", daemon)
	}
	if reported != expected {
		t.Fatalf("child working directory = %s, want the run workspace %s", reported, expected)
	}
}

// twoWorkspacesOnDifferentMounts returns two real directories to run agents in.
// By default they are two distinct trees under different parents, which is the
// portable form of "different mounts". Set MATRIX_TEST_WORKSPACE_A and
// MATRIX_TEST_WORKSPACE_B to two real workspace paths on different mounts to
// exercise the reported case verbatim; the test refuses to pretend if only one
// is set or if both resolve to the same mount.
func twoWorkspacesOnDifferentMounts(t *testing.T) []string {
	t.Helper()

	first, second := os.Getenv("MATRIX_TEST_WORKSPACE_A"), os.Getenv("MATRIX_TEST_WORKSPACE_B")
	if (first == "") != (second == "") {
		t.Fatal("set both MATRIX_TEST_WORKSPACE_A and MATRIX_TEST_WORKSPACE_B, or neither")
	}
	if first != "" {
		for _, path := range []string{first, second} {
			info, err := os.Stat(path)
			if err != nil || !info.IsDir() {
				t.Fatalf("verify MATRIX_TEST_WORKSPACE_*: %s is not an existing directory (%v)", path, err)
			}
		}
		if mountOf(t, first) == mountOf(t, second) {
			t.Fatalf("MATRIX_TEST_WORKSPACE_A and _B must be on different mounts, both are on %s", mountOf(t, first))
		}
		return []string{first, second}
	}

	root := t.TempDir()
	first, second = filepath.Join(root, "mount-a", "workspace"), filepath.Join(root, "mount-b", "workspace")
	for _, path := range []string{first, second} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return []string{first, second}
}

// mountOf names the filesystem a path lives on, so the two-workspace proof can
// state which mounts it used instead of asserting an unstated assumption.
func mountOf(t *testing.T, path string) string {
	t.Helper()

	data, err := os.ReadFile("/proc/mounts")
	if err != nil {
		return fmt.Sprintf("unresolved(%s)", path)
	}
	var best string
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 || !strings.HasPrefix(path, fields[1]) {
			continue
		}
		if len(fields[1]) >= len(best) {
			best = fields[1]
		}
	}
	if best == "" {
		return fmt.Sprintf("unresolved(%s)", path)
	}
	return best
}
