package agentmgr

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Josepavese/matrix/internal/logic/agentcfg"
)

// TestInstallRefusesRegistryCommandThatEscapesTheAgentDirectory is the G6
// contract: the index may not point the launcher outside the agent directory,
// however it is spelled. Every case must be refused before the download, so the
// refusal leaves no temporary file, no agent directory, no registration and no
// evidence behind.
func TestInstallRefusesRegistryCommandThatEscapesTheAgentDirectory(t *testing.T) {
	cases := []struct {
		name string
		cmd  string
		want string
	}{
		{"absolute path", "/bin/sh", "absolute path"},
		{"absolute path to a real agent", "/usr/bin/env", "absolute path"},
		{"parent traversal", "../evil", "does not stay inside the agent directory"},
		{"dot-slash parent traversal", "./../evil", "does not stay inside the agent directory"},
		{"nested traversal", "bin/../../evil", "does not stay inside the agent directory"},
		{"empty cmd", "", "empty cmd"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			registry := startFakeRegistryEntry(t, publishRealDigest, test.cmd,
				map[string]string{"opencode": "#!/bin/sh\nexec opencode-acp\n"})
			installer, store, baseDir, tempDir := newTestInstaller(t, registry.registryURL())

			err := installer.Install(context.Background(), "opencode")
			if err == nil {
				t.Fatalf("registry cmd %q must be refused", test.cmd)
			}
			t.Logf("refused install: %v", err)
			for _, want := range []string{test.want, "opencode", test.cmd} {
				if !strings.Contains(err.Error(), want) {
					t.Fatalf("the refusal must contain %q, got %v", want, err)
				}
			}
			assertNoHalfInstallation(t, baseDir, tempDir, store)
		})
	}
}

// TestInstallRefusesRegistryCommandWithShellMetacharacters pins the second half
// of the shape rule. Matrix starts agents with exec, not a shell, so these are
// refused as defence in depth: the same value is echoed into progress, doctor
// output and support reports, and no live registry entry needs any of them.
func TestInstallRefusesRegistryCommandWithShellMetacharacters(t *testing.T) {
	cases := []struct {
		name string
		cmd  string
	}{
		{"command separator", "./opencode;id"},
		{"pipe", "./opencode|cat"},
		{"command substitution", "$(id)"},
		{"backticks", "./`id`"},
		{"redirection", "./opencode>out"},
		{"glob", "./opencode*"},
		{"home shorthand", "~/.opencode"},
		{"internal space", "./open code"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			registry := startFakeRegistryEntry(t, publishRealDigest, test.cmd,
				map[string]string{"opencode": "#!/bin/sh\nexec opencode-acp\n"})
			installer, store, baseDir, tempDir := newTestInstaller(t, registry.registryURL())

			err := installer.Install(context.Background(), "opencode")
			if err == nil {
				t.Fatalf("registry cmd %q must be refused", test.cmd)
			}
			if !strings.Contains(err.Error(), "not allowed in a launcher path") {
				t.Fatalf("the refusal must name the offending character, got %v", err)
			}
			assertNoHalfInstallation(t, baseDir, tempDir, store)
		})
	}
}

// TestInstallAcceptsLegitimateRegistryCommandShapes is the other side of the
// gate: the shapes the live index uses must still install, and the registered
// command must be the file inside the agent directory.
func TestInstallAcceptsLegitimateRegistryCommandShapes(t *testing.T) {
	files := map[string]string{
		"opencode":          "#!/bin/sh\nexec opencode-acp\n",
		"bin/kimi":          "#!/bin/sh\nexec kimi-acp\n",
		"nested/dir/cortex": "#!/bin/sh\nexec cortex-acp\n",
		"opencode.exe":      "MZ",
	}
	cases := []struct {
		cmd  string
		want string
	}{
		{"./opencode", "opencode"},
		{"opencode", "opencode"},
		{"opencode.exe", "opencode.exe"},
		{"bin/kimi", "bin/kimi"},
		{"./bin/kimi", "bin/kimi"},
		{"nested/dir/cortex", "nested/dir/cortex"},
	}
	for _, test := range cases {
		t.Run(test.cmd, func(t *testing.T) {
			registry := startFakeRegistryEntry(t, publishRealDigest, test.cmd, files)
			installer, store, baseDir, tempDir := newTestInstaller(t, registry.registryURL())

			if err := installer.Install(context.Background(), "opencode"); err != nil {
				t.Fatalf("legitimate registry cmd %q must install, got %v", test.cmd, err)
			}

			entry, err := agentcfg.LoadEntry(store, "opencode")
			if err != nil {
				t.Fatalf("LoadEntry failed: %v", err)
			}
			want := filepath.Join(baseDir, "opencode", filepath.FromSlash(test.want))
			if entry.Config.Command != want {
				t.Fatalf("registered command = %q, want %q", entry.Config.Command, want)
			}
			if _, err := os.Stat(entry.Config.Command); err != nil {
				t.Fatalf("the registered command must exist inside the agent directory: %v", err)
			}
			if !strings.HasPrefix(entry.Config.Command, filepath.Join(baseDir, "opencode")) {
				t.Fatalf("the registered command %q must stay inside the agent directory", entry.Config.Command)
			}
			assertTemporaryDownloadRemoved(t, tempDir)
		})
	}
}

// TestInstallRefusesRegistryCommandMissingFromTheArchive covers the last gate:
// a cmd that is lexically contained but matches nothing in the archive would
// register an agent that cannot start. The digest gate has already passed here,
// so the extracted files remain on disk; what must not happen is registration.
func TestInstallRefusesRegistryCommandMissingFromTheArchive(t *testing.T) {
	registry := startFakeRegistryEntry(t, publishRealDigest, "bin/absent",
		map[string]string{"opencode": "#!/bin/sh\nexec opencode-acp\n"})
	installer, store, baseDir, tempDir := newTestInstaller(t, registry.registryURL())

	err := installer.Install(context.Background(), "opencode")
	if err == nil {
		t.Fatal("a registry cmd that the archive does not contain must refuse the install")
	}
	t.Logf("refused install: %v", err)
	for _, want := range []string{"bin/absent", "does not exist after extraction", "opencode"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("the refusal must contain %q, got %v", want, err)
		}
	}

	entry, err := agentcfg.LoadEntry(store, "opencode")
	if err != nil {
		t.Fatalf("LoadEntry failed: %v", err)
	}
	if entry.Config.Command != "" {
		t.Fatalf("a refused launcher must not be registered, got %+v", entry.Config)
	}
	meta, err := agentcfg.LoadMeta(store, "opencode")
	if err != nil {
		t.Fatalf("LoadMeta failed: %v", err)
	}
	if meta.ArtifactVerification != nil {
		t.Fatalf("a refused launcher must not record evidence, got %+v", meta.ArtifactVerification)
	}
	if _, err := os.Stat(filepath.Join(baseDir, "opencode")); err != nil {
		t.Fatalf("the archive was verified and extracted, so the directory exists: %v", err)
	}
	assertTemporaryDownloadRemoved(t, tempDir)
}

// TestInstallRefusesRegistryCommandPointingAtADirectory keeps a cmd that names
// the agent directory itself (or a subdirectory) from being registered as an
// executable.
func TestInstallRefusesRegistryCommandPointingAtADirectory(t *testing.T) {
	registry := startFakeRegistryEntry(t, publishRealDigest, ".",
		map[string]string{"opencode": "#!/bin/sh\nexec opencode-acp\n"})
	installer, store, _, tempDir := newTestInstaller(t, registry.registryURL())

	err := installer.Install(context.Background(), "opencode")
	if err == nil {
		t.Fatal("a registry cmd that resolves to a directory must refuse the install")
	}
	t.Logf("refused install: %v", err)
	for _, want := range []string{"resolves to the directory", "opencode", `"."`} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("the refusal must contain %q, got %v", want, err)
		}
	}

	entry, err := agentcfg.LoadEntry(store, "opencode")
	if err != nil {
		t.Fatalf("LoadEntry failed: %v", err)
	}
	if entry.Config.Command != "" {
		t.Fatalf("a refused launcher must not be registered, got %+v", entry.Config)
	}
	assertTemporaryDownloadRemoved(t, tempDir)
}
