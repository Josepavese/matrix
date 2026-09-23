package agentinstall

import (
	"path/filepath"
	"strings"
	"testing"
)

// liveLauncherPaths is every unique cmd the ACP registry index (schema 1.0.0,
// 100 binary platform entries) published on 2026-09-23, across all platform
// mappings. It is the compatibility floor: the validator must never be the
// reason a legitimate agent stops installing.
var liveLauncherPaths = []string{
	"./agy_acp_server.exe",
	"./agy_acp_server.par",
	"./amp-acp",
	"amp-acp.exe",
	"./Applications/junie.app/Contents/MacOS/junie",
	"./bin/devin",
	`./bin\devin.exe`,
	"./bin/kimchi",
	"./bin/kimchi.exe",
	"./coco-1.0.73+180523.e6179a031de9-darwin-amd64/cortex",
	"./coco-1.0.73+180523.e6179a031de9-darwin-arm64/cortex",
	"./coco-1.0.73+180523.e6179a031de9-linux-amd64/cortex",
	"./coco-1.0.73+180523.e6179a031de9-linux-arm64/cortex",
	"./coco-1.0.73+180523.e6179a031de9-windows-amd64/cortex.exe",
	"./coco-1.0.73+180523.e6179a031de9-windows-arm64/cortex.exe",
	"./corust-agent-acp",
	"./corust-agent-acp.exe",
	"./crow-cli",
	"./crow-cli.exe",
	"./dist-package/cursor-agent",
	`./dist-package\cursor-agent.cmd`,
	"./goose",
	`./goose-package\goose.exe`,
	"./harn",
	"harn.exe",
	"./junie-app/bin/junie",
	"./junie/junie.exe",
	"./kilo",
	"./kilo.exe",
	"./kimi",
	"./kimi.exe",
	"./opencode",
	"./opencode.exe",
	"./pool-darwin-amd64",
	"./pool-darwin-arm64",
	"./pool-linux-amd64",
	"./pool-linux-arm64",
	"./pool-windows-amd64.exe",
	"./pool-windows-arm64.exe",
	"./sigit",
	"./sigit-linux-amd64",
	"./sigit-linux-arm64",
	"./sigit-win-amd64.exe",
	"./sigit-win-arm64.exe",
	"./stakpak",
	"./stakpak.exe",
	"./vibe-acp",
	"./vibe-acp.exe",
	"./vtcode",
	"vtcode.exe",
}

// TestValidateLauncherPathAcceptsEveryLiveRegistryShape keeps the launcher gate
// from over-reaching: every cmd the live index publishes for a binary
// distribution still validates.
func TestValidateLauncherPathAcceptsEveryLiveRegistryShape(t *testing.T) {
	for _, cmd := range liveLauncherPaths {
		if err := validateLauncherPath(cmd); err != nil {
			t.Fatalf("live registry cmd %q must be accepted, got %v", cmd, err)
		}
	}
}

// TestValidateLauncherPathRejectsHostileShapes names, one by one, what the
// index has no business publishing: an absolute launcher, a traversal, a shell
// metacharacter or a padded value. Each must be refused with the reason, not
// silently repaired.
func TestValidateLauncherPathRejectsHostileShapes(t *testing.T) {
	cases := []struct {
		name string
		cmd  string
		want string
	}{
		{"empty", "", "empty cmd"},
		{"whitespace only", "   ", "empty cmd"},
		{"leading whitespace", " ./opencode", "surrounding whitespace"},
		{"trailing whitespace", "./opencode ", "surrounding whitespace"},
		{"absolute unix path", "/bin/sh", "absolute path"},
		{"absolute path with arguments", "/usr/bin/env sh", "absolute path"},
		{"parent traversal", "../opencode", "does not stay inside the agent directory"},
		{"dot-slash parent traversal", "./../opencode", "does not stay inside the agent directory"},
		{"nested parent traversal", "bin/../../opencode", "does not stay inside the agent directory"},
		{"command separator", "./opencode;id", "not allowed in a launcher path"},
		{"pipe", "./opencode|cat", "not allowed in a launcher path"},
		{"command substitution", "$(id)", "not allowed in a launcher path"},
		{"backticks", "./`id`", "not allowed in a launcher path"},
		{"redirection", "./opencode>out", "not allowed in a launcher path"},
		{"glob", "./opencode*", "not allowed in a launcher path"},
		{"home shorthand", "~/.opencode", "not allowed in a launcher path"},
		{"internal space", "./open code", "not allowed in a launcher path"},
		{"internal newline", "./open\ncode", "not allowed in a launcher path"},
		{"trailing newline", "./opencode\n", "surrounding whitespace"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			err := validateLauncherPath(test.cmd)
			if err == nil {
				t.Fatalf("cmd %q must be refused", test.cmd)
			}
			if !strings.Contains(err.Error(), test.want) {
				t.Fatalf("refusal for %q must say %q, got %v", test.cmd, test.want, err)
			}
		})
	}

	if err := validateLauncherPath(strings.Repeat("a", maxLauncherPathLength+1)); err == nil {
		t.Fatal("a launcher path over the length limit must be refused")
	}
}

// TestResolveLauncherPathKeepsEveryAcceptedPathInsideTheAgentDirectory is the
// containment contract: whatever the index sends, the resolved launcher is a
// descendant of the agent directory, and a traversal returns no path at all.
// The Windows-style values are included because on this host "\" is a filename
// character while on Windows filepath treats it as a separator; either way the
// result stays inside.
func TestResolveLauncherPathKeepsEveryAcceptedPathInsideTheAgentDirectory(t *testing.T) {
	agentDir := filepath.Join(t.TempDir(), "opencode")
	accepted := append([]string{"opencode", "./opencode", "bin/opencode", "./bin/opencode"}, `bin\opencode.exe`, `dist-package\cursor-agent.cmd`)

	for _, cmd := range accepted {
		resolved, err := ResolveLauncherPath(agentDir, cmd)
		if err != nil {
			t.Fatalf("cmd %q must resolve, got %v", cmd, err)
		}
		relative, err := filepath.Rel(agentDir, resolved)
		if err != nil || relative == ".." || filepath.IsAbs(relative) ||
			strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			t.Fatalf("cmd %q resolved to %q, outside %q", cmd, resolved, agentDir)
		}
	}

	for _, cmd := range []string{"../opencode", "./../opencode", "/bin/sh", "bin/../../opencode"} {
		if resolved, err := ResolveLauncherPath(agentDir, cmd); err == nil {
			t.Fatalf("cmd %q must be refused, got %q", cmd, resolved)
		}
	}
}

// TestValidatePackageSpecAcceptsEveryLiveRegistryPackage is the same
// compatibility floor for npx/uvx: the package identifiers the live index
// publishes must validate, version pin included.
func TestValidatePackageSpecAcceptsEveryLiveRegistryPackage(t *testing.T) {
	npxPackages := []string{
		"agoragentic-mcp@1.3.0", "@augmentcode/auggie@0.36.0",
		"@autohandai/autohand-acp@0.2.1", "@agentclientprotocol/claude-agent-acp@0.81.1",
		"cline@3.0.64", "@tencent-ai/codebuddy-code@2.156.0",
		"@agentclientprotocol/codex-acp@1.13.1", "deepagents-acp@0.1.7",
		"dimcode@0.5.11", "dirac-cli@0.5.15", "droid@0.225.2",
		"@google/gemini-cli@0.60.0", "@github/copilot@1.0.88", "glm-acp-agent@1.12.0",
		"@xai-official/grok@1.0.41", "@kilocode/cli@7.7.9", "@minimax-ai/code@0.2.7",
		"@compass-ai/nova@1.1.46", "pi-acp@0.0.33", "@qoder-ai/qodercli@0.2.14",
		"@qwen-code/qwen-code@0.24.4", "@smbcloud/sigit@1.5.10",
	}
	for _, pkg := range npxPackages {
		if err := ValidatePackageSpec("npx", pkg); err != nil {
			t.Fatalf("live npx package %q must be accepted, got %v", pkg, err)
		}
	}
	for _, pkg := range []string{"fast-agent-acp==0.10.1", "minion-code@0.1.44"} {
		if err := ValidatePackageSpec("uvx", pkg); err != nil {
			t.Fatalf("live uvx package %q must be accepted, got %v", pkg, err)
		}
	}
}

// TestValidatePackageSpecRejectsOptionShapedAndMalformedIdentifiers covers the
// attack the package gate exists for: npx and uvx parse their own options
// anywhere on the command line, so an index-supplied identifier that starts
// with "-" stops being a package and becomes an instruction.
func TestValidatePackageSpecRejectsOptionShapedAndMalformedIdentifiers(t *testing.T) {
	cases := []struct {
		name string
		kind string
		pkg  string
		want string
	}{
		{"npx short option", "npx", "-c", "would read as an option"},
		{"npx long option", "npx", "--registry=http://evil.test", "would read as an option"},
		{"npx injected option", "npx", "opencode --registry=http://evil.test", "not a valid package name"},
		{"npx version range with space", "npx", "opencode@>=1.0.0 <2.0.0", "not a valid package name"},
		{"npx scoped traversal", "npx", "@scope/../evil", "not a valid package name"},
		{"npx url", "npx", "http://evil.test/opencode", "not a valid package name"},
		{"npx shell metacharacter", "npx", "opencode;id", "not a valid package name"},
		{"npx command substitution", "npx", "$(id)", "not a valid package name"},
		{"npx padded", "npx", " opencode", "empty or padded"},
		{"npx unclosed scope", "npx", "@scope/", "not a valid package name"},
		{"npx empty version", "npx", "opencode@", "not a valid package name"},
		{"uvx long option", "uvx", "--from", "would read as an option"},
		{"uvx index redirection", "uvx", "--index-url=http://evil.test/simple", "would read as an option"},
		{"uvx empty specifier", "uvx", "opencode===", "not a valid package name"},
		{"uvx traversal", "uvx", "../../evil", "not a valid package name"},
		{"unsupported kind", "npm", "opencode", "unsupported distribution type"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			err := ValidatePackageSpec(test.kind, test.pkg)
			if err == nil {
				t.Fatalf("%s package %q must be refused", test.kind, test.pkg)
			}
			if !strings.Contains(err.Error(), test.want) {
				t.Fatalf("refusal for %q must say %q, got %v", test.pkg, test.want, err)
			}
		})
	}

	if err := ValidatePackageSpec("npx", strings.Repeat("a", maxPackageSpecLength+1)); err == nil {
		t.Fatal("a package identifier over the length limit must be refused")
	}
}

// TestValidatePackageSpecKeepsSpaceFreeVersionPinsWorkable documents what the
// gate deliberately still accepts beyond the live set: exact pins, dist-tags
// and space-free ranges. A stricter future rule has to argue with these forms
// rather than delete them by accident.
func TestValidatePackageSpecKeepsSpaceFreeVersionPinsWorkable(t *testing.T) {
	for _, pkg := range []string{"opencode", "opencode@latest", "opencode@^1.2.3", "opencode@~1.2.3", "opencode@1.x", "opencode@>=1.0.0"} {
		if err := ValidatePackageSpec("npx", pkg); err != nil {
			t.Fatalf("npx package %q must be accepted, got %v", pkg, err)
		}
	}
	for _, pkg := range []string{"fast-agent-acp", "fast-agent-acp[cli]==1.0.0", "fast-agent-acp>=1.0,<2.0"} {
		if err := ValidatePackageSpec("uvx", pkg); err != nil {
			t.Fatalf("uvx package %q must be accepted, got %v", pkg, err)
		}
	}
}
