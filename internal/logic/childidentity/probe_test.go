//go:build linux

package childidentity

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Josepavese/matrix/internal/logic/agentlaunch"
	"github.com/Josepavese/matrix/internal/middleware"
)

// acpStdioEndpoint is the shape the doctor probes: a real local command spoken
// over stdio.
func acpStdioEndpoint(env []string) middleware.ProtocolEndpoint {
	return middleware.ProtocolEndpoint{
		Kind:      middleware.ProtocolKindACP,
		Transport: "stdio",
		Command:   "/bin/sleep",
		Args:      []string{"30"},
		Env:       env,
	}
}

func resolved(t *testing.T, path string) string {
	t.Helper()
	value, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatalf("EvalSymlinks(%q): %v", path, err)
	}
	return value
}

// TestDoctorChildIdentityProvesTheDeclaredProcessCwd is the acceptance evidence
// for the process cwd knob: populating only the declaration makes the kernel
// report that directory for the child, with the argv Matrix handed over and no
// launcher in between.
func TestDoctorChildIdentityProvesTheDeclaredProcessCwd(t *testing.T) {
	dir := t.TempDir()
	endpoint := acpStdioEndpoint([]string{agentlaunch.ProcessCwdEnv + "=" + dir})

	processCwd, err := DeclaredProcessCwd(endpoint)
	if err != nil {
		t.Fatalf("DeclaredProcessCwd: %v", err)
	}
	child, warnings := Probe(endpoint, processCwd)
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	if child.Status != "observed" {
		t.Fatalf("child status = %q (%s), want observed", child.Status, child.Error)
	}
	if want := resolved(t, dir); child.Cwd != want {
		t.Fatalf("child.cwd = %q, want %q: the declared process cwd must be where the child actually runs", child.Cwd, want)
	}
	if got := strings.Join(child.Argv, " "); got != "/bin/sleep 30" {
		t.Fatalf("child.argv = %q, want %q: the declaration must not enter argv", got, "/bin/sleep 30")
	}
	for _, arg := range child.Argv {
		if arg == "-C" || arg == "--cwd" {
			t.Fatalf("child.argv carries a launcher flag %q: %v", arg, child.Argv)
		}
	}
	if !strings.Contains(child.Source, "/proc/") {
		t.Fatalf("child evidence must say where it was read from, got %q", child.Source)
	}
	if child.StartTicks == 0 {
		t.Fatal("the observed evidence carries no start time, so a pid recycled into another process could not be told apart")
	}
}

// TestReadIdentityReadsTheStartTimeTheKernelReports checks the parser against
// the kernel's own answer for a process whose parent and start time are known
// independently of it: this test's own process. A parser that read the field
// beside the start time, or that took the name for a position, would disagree
// with `os.Getppid()` here.
func TestReadIdentityReadsTheStartTimeTheKernelReports(t *testing.T) {
	identity, err := readIdentity(os.Getpid())
	if err != nil {
		t.Fatalf("readIdentity(self): %v", err)
	}
	if identity.ParentPID != os.Getppid() {
		t.Fatalf("parent = %d, want %d: /proc/<pid>/stat field 4 is the parent, not the process group",
			identity.ParentPID, os.Getppid())
	}
	if identity.StartTicks == 0 {
		t.Fatal("no start time reported for a live process")
	}

	again, err := readIdentity(os.Getpid())
	if err != nil {
		t.Fatalf("readIdentity(self) again: %v", err)
	}
	if again.StartTicks != identity.StartTicks {
		t.Fatalf("the start time of one live process changed between reads: %d then %d",
			identity.StartTicks, again.StartTicks)
	}
}

// TestTheObservationLoopRefusesAChildTheGuardDoesNotRecognise pins the wiring,
// not the rule: the guard's own tests prove what it refuses, and this one proves
// the probe asks it before reporting a child as evidence. The guard is armed with
// a start time nobody else has, so the live child in front of it is a process it
// cannot claim — and the loop must answer with a refusal instead of an
// observation. Removing the guard call from the loop leaves every other test in
// this package green, which is exactly why this one exists.
func TestTheObservationLoopRefusesAChildTheGuardDoesNotRecognise(t *testing.T) {
	cmd := exec.Command("/bin/sleep", "30")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start child: %v", err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})

	guard := &reuseGuard{parent: os.Getpid(), started: 1, armed: true}
	report, warnings := observeChild(cmd.Process.Pid, guard, t.TempDir())

	if report.Status != "unreadable" {
		t.Fatalf("status = %q (%s), want unreadable: the loop reported a process the guard had refused", report.Status, report.Error)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "refused") {
		t.Fatalf("warnings = %v, want the refusal the guard produced", warnings)
	}
	if report.Argv != nil || report.Cwd != "" {
		t.Fatalf("a refused observation carried evidence anyway: %+v", report)
	}
}

// TestDoctorChildIdentityDoesNotInventAProcessCwd is the other half of the
// guardrail: an endpoint that declares nothing starts where the caller starts.
// The doctor must not substitute a directory of its own, because that is how a
// global override pins every agent to one checkout.
func TestDoctorChildIdentityDoesNotInventAProcessCwd(t *testing.T) {
	endpoint := acpStdioEndpoint(nil)

	processCwd, err := DeclaredProcessCwd(endpoint)
	if err != nil {
		t.Fatalf("DeclaredProcessCwd: %v", err)
	}
	if processCwd != "" {
		t.Fatalf("an undeclared process cwd resolved to %q, want no choice at all", processCwd)
	}
	child, warnings := Probe(endpoint, processCwd)
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	if child.Status != "observed" {
		t.Fatalf("child status = %q (%s), want observed", child.Status, child.Error)
	}
	here, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if want := resolved(t, here); child.Cwd != want {
		t.Fatalf("child.cwd = %q, want the caller's %q", child.Cwd, want)
	}
}

// TestDoctorChildRefusesAnUnusableDeclaredProcessCwd pins that an unusable
// declaration stops the probe before a child is started anywhere else.
func TestDoctorChildRefusesAnUnusableDeclaredProcessCwd(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "absent-checkout")
	endpoint := acpStdioEndpoint([]string{agentlaunch.ProcessCwdEnv + "=" + missing})

	if _, err := DeclaredProcessCwd(endpoint); err == nil {
		t.Fatal("expected the probe to refuse a declared process cwd that does not exist")
	}
}
