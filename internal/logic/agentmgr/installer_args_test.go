package agentmgr

import (
	"context"
	"reflect"
	"testing"

	"github.com/Josepavese/matrix/internal/logic/agentcfg"
	"github.com/Josepavese/matrix/internal/middleware"
)

// TestResolveAnyDistributionKeepsBinaryPlatformArgs is the registry half of the
// issue-8 contract: a binary distribution whose platform entry declares
// arguments must carry them out of resolution. The binary branch used to return
// a ResolvedDist with no Args at all, so the loss was already there before the
// installer ever saw the distribution. Reverting this branch fails here.
func TestResolveAnyDistributionKeepsBinaryPlatformArgs(t *testing.T) {
	cases := []struct {
		name  string
		args  []string
		wants []string
	}{
		{name: "declared argument", args: []string{"acp"}, wants: []string{"acp"}},
		{name: "several declared arguments", args: []string{"acp", "--log-level", "warn"}, wants: []string{"acp", "--log-level", "warn"}},
		{name: "no declared arguments", args: nil, wants: []string{}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			client := NewRegistryClient(nil, "https://registry.invalid/registry.json")
			client.goos, client.arch = "linux", "amd64"
			manifest := &AgentManifest{
				ID: "some-agent", Version: "1.0.0",
				Distribution: RegistryDistribution{Binary: map[string]BinaryDist{
					testPlatform: {Archive: "https://cdn.invalid/a.tar.gz", Cmd: "./some-agent", Args: test.args},
				}},
			}

			resolved, err := client.ResolveAnyDistribution(manifest)
			if err != nil {
				t.Fatalf("ResolveAnyDistribution failed: %v", err)
			}
			if resolved.Type != "binary" {
				t.Fatalf("distribution type = %q, want binary", resolved.Type)
			}
			if !reflect.DeepEqual(resolved.Args, test.wants) {
				t.Fatalf("resolved args = %#v, want %#v: the platform entry declares them and resolution must not drop them", resolved.Args, test.wants)
			}
		})
	}
}

// TestInstallKeepsBinaryPlatformArgs is the acceptance proof for issue 8: after
// `matrix install`, the registered config carries exactly the arguments the
// registry index declares for this platform, and `matrix agent show` sees the
// same list because it is the same registry view. It fails if either loss point
// returns: resolution (registry_client.go) or the binary install branch
// (installer.go).
func TestInstallKeepsBinaryPlatformArgs(t *testing.T) {
	cases := []struct {
		name  string
		args  []string
		wants []string
	}{
		{name: "command without arguments", args: nil, wants: []string{}},
		{name: "acp subcommand", args: []string{"acp"}, wants: []string{"acp"}},
		{name: "flag and value", args: []string{"acp", "--log-level", "warn"}, wants: []string{"acp", "--log-level", "warn"}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			registry := startFakeRegistryWithArgs(t, publishRealDigest, "./opencode", test.args,
				map[string]string{"opencode": "#!/bin/sh\nexec opencode-acp\n"})
			installer, store, _, _ := newTestInstaller(t, registry.registryURL())

			if err := installer.Install(context.Background(), "opencode"); err != nil {
				t.Fatalf("install failed: %v", err)
			}

			// This is the read-back behind `matrix agent show`: the effective
			// config comes from the registry loaded over the vault entry.
			assertEffectiveArgs(t, store, test.wants)
		})
	}
}

// TestInstallRepairsDroppedPlatformArgsIdempotently covers the half issue 8
// warns about: a fix in the install path alone leaves already-installed workers
// with args: []. The repair is a documented, idempotent install: it fills the
// declared arguments into an existing record that lost them and never appends a
// second copy, while the operator's explicit override stays untouched.
func TestInstallRepairsDroppedPlatformArgsIdempotently(t *testing.T) {
	registry := startFakeRegistryWithArgs(t, publishRealDigest, "./opencode", []string{"acp"},
		map[string]string{"opencode": "#!/bin/sh\nexec opencode-acp\n"})
	installer, store, _, _ := newTestInstaller(t, registry.registryURL())

	if err := installer.Install(context.Background(), "opencode"); err != nil {
		t.Fatalf("first install failed: %v", err)
	}
	installed, err := agentcfg.LoadEntry(store, "opencode")
	if err != nil {
		t.Fatalf("LoadEntry failed: %v", err)
	}

	// The legacy record: the very shape the old installer wrote, plus an
	// explicit operator override, which the repair must preserve verbatim.
	legacy := installed
	legacy.Config.Args = []string{}
	legacy.Override.AppendArgs = []string{"--operator-flag"}
	if err := agentcfg.SaveEntry(store, "opencode", legacy); err != nil {
		t.Fatalf("SaveEntry failed: %v", err)
	}

	if err := installer.Install(context.Background(), "opencode"); err != nil {
		t.Fatalf("repairing install failed: %v", err)
	}
	repaired, err := agentcfg.LoadEntry(store, "opencode")
	if err != nil {
		t.Fatalf("LoadEntry failed: %v", err)
	}
	if !reflect.DeepEqual(repaired.Config.Args, []string{"acp"}) {
		t.Fatalf("repaired args = %#v, want exactly the declared [acp]", repaired.Config.Args)
	}
	if !reflect.DeepEqual(repaired.Override.AppendArgs, []string{"--operator-flag"}) {
		t.Fatalf("override args = %#v, the repair must not touch the operator override", repaired.Override.AppendArgs)
	}
	assertEffectiveArgs(t, store, []string{"acp", "--operator-flag"})

	// Idempotence: a second repair must not duplicate the argument.
	if err := installer.Install(context.Background(), "opencode"); err != nil {
		t.Fatalf("second repairing install failed: %v", err)
	}
	again, err := agentcfg.LoadEntry(store, "opencode")
	if err != nil {
		t.Fatalf("LoadEntry failed: %v", err)
	}
	if !reflect.DeepEqual(again.Config.Args, []string{"acp"}) {
		t.Fatalf("args after the second repair = %#v, want exactly one copy of [acp]", again.Config.Args)
	}
	assertEffectiveArgs(t, store, []string{"acp", "--operator-flag"})
}

// TestInstallResolvedKeepsBinaryPlatformArgsForAFreshRecord pins the second loss
// point on its own. The end-to-end test cannot see it while the repair also
// fills empty arguments, so this one asks the install branch directly, with no
// existing record to fall back on: the config the binary branch returns must
// already carry what the index declared. Reverting that one line fails here even
// though every end-to-end case still passes.
func TestInstallResolvedKeepsBinaryPlatformArgsForAFreshRecord(t *testing.T) {
	registry := startFakeRegistryWithArgs(t, publishRealDigest, "./opencode", []string{"acp", "--headless"},
		map[string]string{"opencode": "#!/bin/sh\nexec opencode-acp\n"})
	installer, _, _, _ := newTestInstaller(t, registry.registryURL())

	manifest, err := installer.registry.FetchManifest(context.Background(), "opencode")
	if err != nil {
		t.Fatalf("FetchManifest failed: %v", err)
	}
	resolved, err := installer.registry.ResolveAnyDistribution(manifest)
	if err != nil {
		t.Fatalf("ResolveAnyDistribution failed: %v", err)
	}
	cfg, _, err := installer.installResolved(context.Background(), "opencode", manifest, resolved)
	if err != nil {
		t.Fatalf("installResolved failed: %v", err)
	}
	if !reflect.DeepEqual(cfg.Args, []string{"acp", "--headless"}) {
		t.Fatalf("binary install config args = %#v, want the declared [acp --headless] before any record exists", cfg.Args)
	}
	if cfg.Command == "" {
		t.Fatal("the binary branch must still resolve the launcher it extracted")
	}
}

// assertEffectiveArgs reads the agent config the way `matrix agent show` and
// the launch path do, through the registry over the vault, and asserts the
// effective argument list.
func assertEffectiveArgs(t *testing.T, store middleware.Storage, wants []string) {
	t.Helper()

	registry, err := NewRegistry(nil, store)
	if err != nil {
		t.Fatalf("NewRegistry failed: %v", err)
	}
	effective, err := registry.Get("opencode")
	if err != nil {
		t.Fatalf("registry.Get failed: %v", err)
	}
	if !reflect.DeepEqual(effective.Args, wants) {
		t.Fatalf("effective.args = %#v, want %#v: this is what `matrix agent show` reports", effective.Args, wants)
	}
}
