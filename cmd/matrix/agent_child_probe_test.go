//go:build linux

package main

import (
	"os"
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

	processCwd, err := childProcessCwd(endpoint)
	if err != nil {
		t.Fatalf("childProcessCwd: %v", err)
	}
	child, warnings := probeChild(endpoint, processCwd)
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
}

// TestDoctorChildIdentityDoesNotInventAProcessCwd is the other half of the
// guardrail: an endpoint that declares nothing starts where the caller starts.
// The doctor must not substitute a directory of its own, because that is how a
// global override pins every agent to one checkout.
func TestDoctorChildIdentityDoesNotInventAProcessCwd(t *testing.T) {
	endpoint := acpStdioEndpoint(nil)

	processCwd, err := childProcessCwd(endpoint)
	if err != nil {
		t.Fatalf("childProcessCwd: %v", err)
	}
	if processCwd != "" {
		t.Fatalf("an undeclared process cwd resolved to %q, want no choice at all", processCwd)
	}
	child, warnings := probeChild(endpoint, processCwd)
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

	if _, err := childProcessCwd(endpoint); err == nil {
		t.Fatal("expected the probe to refuse a declared process cwd that does not exist")
	}
}
