package agentmgr

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Josepavese/matrix/internal/logic/agentcfg"
	"github.com/Josepavese/matrix/internal/logic/agentlaunch"
	"github.com/Josepavese/matrix/internal/middleware"
)

// ----------------------------------------------------------------------------
// The argument repair, observed at the child
//
// The other tests in this package stop at the installed record: what the
// registry reports as effective.args, which is also what `matrix agent show`
// prints. That view is one layer above the thing that actually breaks — the
// argv of the process an agent turn launches — and the issue it comes from
// (2026-09-30, "binary agent args dropped") was masked end-to-end precisely
// because a reader could not tell a repaired record from a launched command.
//
// These tests start from a record that is already broken (the shape the old
// installer wrote: a binary and no arguments), repair it through the documented
// install, and then walk the run path's own chain to the process: the record
// becomes a registry entry, the entry becomes an endpoint through
// protocolEndpointFromAgentConfig, the endpoint becomes an argv through
// agentlaunch.PrepareStdio — the same function the stdio transport calls before
// it spawns — and that argv is started for real. The installed binary records
// the argv it was handed, so the assertion is about the child's view of its
// command line — order, forms and absence included — rather than about a
// configuration that claims to produce it.
// ----------------------------------------------------------------------------

// recordingBinary is the installed program: it writes its own argv where the
// test can read it, then exits. Nothing here speaks ACP, because the claim under
// test ends at the launch.
func recordingBinary(t *testing.T, recordPath string) string {
	t.Helper()
	return "#!/bin/sh\n" +
		"{ printf 'argc=%s\\n' \"$#\"; for arg in \"$@\"; do printf 'arg=%s\\n' \"$arg\"; done; } > " +
		shellQuote(recordPath) + "\n"
}

// shellQuote single-quotes a path so the generated script cannot be broken by a
// temporary directory that contains spaces or quotes.
func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}

// launchedChildArgv repairs nothing and asserts nothing: it replays the run
// path's chain over the installed record and returns the argv the spawned child
// received.
func launchedChildArgv(t *testing.T, store middleware.Storage, recordPath string) []string {
	t.Helper()

	registry, err := NewRegistry(nil, store)
	if err != nil {
		t.Fatalf("NewRegistry failed: %v", err)
	}
	effective, err := registry.Get("opencode")
	if err != nil {
		t.Fatalf("registry.Get failed: %v", err)
	}
	endpoint := protocolEndpointFromAgentConfig(effective)

	if endpoint.Command == "" {
		t.Fatal("the installed record has no command: nothing would be launched")
	}
	if info, err := os.Stat(endpoint.Command); err != nil {
		t.Fatalf("the installed command %s is not on disk: %v", endpoint.Command, err)
	} else if info.Mode()&0o111 == 0 {
		t.Fatalf("the installed command %s is not executable (mode %s)", endpoint.Command, info.Mode())
	}

	command, args := agentlaunch.PrepareStdio(endpoint.Command, endpoint.Args, endpoint.EnvIsolation)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	// The argv is production's; this package only starts it. The protocol SDK
	// that spawns it in a run lives behind the provider adapters, and a package
	// under internal/logic may not import it, so the launch here is the plain
	// exec of exactly what PrepareStdio handed over.
	child := exec.CommandContext(ctx, command, args...)
	if err := child.Start(); err != nil {
		t.Fatalf("launching the installed agent failed: %v", err)
	}
	argv := waitForRecordedArgv(t, recordPath)
	if err := child.Wait(); err != nil {
		t.Fatalf("the launched agent exited with %v", err)
	}
	return argv
}

// waitForRecordedArgv polls for the child's record. The child is a process, so
// the write is the only signal that the launch happened; the deadline makes a
// launch that never recorded anything a failure instead of an empty read.
func waitForRecordedArgv(t *testing.T, recordPath string) []string {
	t.Helper()

	deadline := time.Now().Add(10 * time.Second)
	var raw []byte
	for time.Now().Before(deadline) {
		content, err := os.ReadFile(recordPath)
		if err == nil {
			raw = content
			break
		}
		time.Sleep(2 * time.Millisecond)
	}
	if raw == nil {
		t.Fatalf("the launched agent never recorded its argv at %s", recordPath)
	}

	lines := strings.Split(strings.TrimRight(string(raw), "\n"), "\n")
	if len(lines) == 0 || !strings.HasPrefix(lines[0], "argc=") {
		t.Fatalf("the recorded argv has no argc line: %q", string(raw))
	}
	declared, err := strconv.Atoi(strings.TrimPrefix(lines[0], "argc="))
	if err != nil {
		t.Fatalf("the recorded argc is not a number: %q", lines[0])
	}
	args := make([]string, 0, len(lines)-1)
	for _, line := range lines[1:] {
		args = append(args, strings.TrimPrefix(line, "arg="))
	}
	if declared != len(args) {
		t.Fatalf("the child counted %d argument(s) but recorded %d: %q", declared, len(args), string(raw))
	}
	return args
}

// TestInstallRepairReachesTheChildCommandLine is the end-to-end half of the
// repair: a record that lost its arguments, an install that fills them back in,
// and the argv of the process that a turn would launch. Order, flag/value forms
// and the operator override are asserted on the child's own report, and a third
// install proves the repair is idempotent at the same place.
func TestInstallRepairReachesTheChildCommandLine(t *testing.T) {
	recordPath := filepath.Join(t.TempDir(), "argv.txt")
	registry := startFakeRegistryWithArgs(t, publishRealDigest, "./opencode",
		[]string{"acp", "--log-level", "warn"},
		map[string]string{"opencode": recordingBinary(t, recordPath)})
	installer, store, _, _ := newTestInstaller(t, registry.registryURL())

	if err := installer.Install(context.Background(), "opencode"); err != nil {
		t.Fatalf("first install failed: %v", err)
	}
	installed, err := agentcfg.LoadEntry(store, "opencode")
	if err != nil {
		t.Fatalf("LoadEntry failed: %v", err)
	}

	// The legacy record, plus the operator's override: the repair must restore
	// the declared arguments and leave the override as the last word.
	legacy := installed
	legacy.Config.Args = []string{}
	legacy.Override.AppendArgs = []string{"--operator-flag", "value with spaces"}
	if err := agentcfg.SaveEntry(store, "opencode", legacy); err != nil {
		t.Fatalf("SaveEntry failed: %v", err)
	}
	assertEffectiveArgs(t, store, []string{"--operator-flag", "value with spaces"})

	if err := installer.Install(context.Background(), "opencode"); err != nil {
		t.Fatalf("repairing install failed: %v", err)
	}

	want := []string{"acp", "--log-level", "warn", "--operator-flag", "value with spaces"}
	if got := launchedChildArgv(t, store, recordPath); !reflect.DeepEqual(got, want) {
		t.Fatalf("the repaired agent was launched with %#v, want %#v", got, want)
	}

	// Idempotence, observed at the child rather than in the record: another
	// install must not append a second copy of the declared arguments.
	if err := os.Remove(recordPath); err != nil {
		t.Fatalf("clearing the first record failed: %v", err)
	}
	if err := installer.Install(context.Background(), "opencode"); err != nil {
		t.Fatalf("second repairing install failed: %v", err)
	}
	if got := launchedChildArgv(t, store, recordPath); !reflect.DeepEqual(got, want) {
		t.Fatalf("after a second repair the agent was launched with %#v, want the same %#v", got, want)
	}
}

// TestInstallKeepsAnExplicitArgumentOverrideAtTheChild pins the other direction
// of the repair: a record that already carries arguments has them because an
// operator set them, so an install must not replace them with what the registry
// declares. The record-level test cannot see the difference — a repair that
// always copied the resolved arguments would satisfy its assertion too — while
// the child's argv can.
func TestInstallKeepsAnExplicitArgumentOverrideAtTheChild(t *testing.T) {
	recordPath := filepath.Join(t.TempDir(), "argv.txt")
	registry := startFakeRegistryWithArgs(t, publishRealDigest, "./opencode",
		[]string{"acp"},
		map[string]string{"opencode": recordingBinary(t, recordPath)})
	installer, store, _, _ := newTestInstaller(t, registry.registryURL())

	if err := installer.Install(context.Background(), "opencode"); err != nil {
		t.Fatalf("first install failed: %v", err)
	}
	installed, err := agentcfg.LoadEntry(store, "opencode")
	if err != nil {
		t.Fatalf("LoadEntry failed: %v", err)
	}
	configured := installed
	configured.Config.Args = []string{"--custom-flag"}
	configured.Override.AppendArgs = []string{"--operator-flag"}
	if err := agentcfg.SaveEntry(store, "opencode", configured); err != nil {
		t.Fatalf("SaveEntry failed: %v", err)
	}

	if err := installer.Install(context.Background(), "opencode"); err != nil {
		t.Fatalf("installing over an explicit configuration failed: %v", err)
	}
	want := []string{"--custom-flag", "--operator-flag"}
	if got := launchedChildArgv(t, store, recordPath); !reflect.DeepEqual(got, want) {
		t.Fatalf("the agent was launched with %#v, want the operator's %#v: an install must not overwrite explicit arguments", got, want)
	}
}

// TestInstallRepairInventsNoArgumentForASilentDeclaration is the other half of
// the same contract: when the registry declares no arguments at all, the repair
// must leave the argv empty. A repair that "helpfully" wrote a default would
// launch a peer with flags nobody asked for, and the empty case is exactly the
// one a fix for dropped arguments is tempted to fill.
func TestInstallRepairInventsNoArgumentForASilentDeclaration(t *testing.T) {
	recordPath := filepath.Join(t.TempDir(), "argv.txt")
	registry := startFakeRegistryWithArgs(t, publishRealDigest, "./opencode", nil,
		map[string]string{"opencode": recordingBinary(t, recordPath)})
	installer, store, _, _ := newTestInstaller(t, registry.registryURL())

	if err := installer.Install(context.Background(), "opencode"); err != nil {
		t.Fatalf("first install failed: %v", err)
	}
	installed, err := agentcfg.LoadEntry(store, "opencode")
	if err != nil {
		t.Fatalf("LoadEntry failed: %v", err)
	}
	legacy := installed
	legacy.Config.Args = []string{}
	if err := agentcfg.SaveEntry(store, "opencode", legacy); err != nil {
		t.Fatalf("SaveEntry failed: %v", err)
	}

	if err := installer.Install(context.Background(), "opencode"); err != nil {
		t.Fatalf("repairing install failed: %v", err)
	}
	if got := launchedChildArgv(t, store, recordPath); len(got) != 0 {
		t.Fatalf("an agent whose distribution declares no arguments was launched with %#v", got)
	}
}
