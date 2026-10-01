package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadManifestAndCheck(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "doc.md"), "Matrix is protocol-neutral and channel-neutral.")
	mustWrite(t, filepath.Join(root, "ci.yml"), "go test ./...")

	manifestPath := filepath.Join(root, "manifest.toml")
	mustWrite(t, manifestPath, `
[documents]
required = ["doc.md"]

[required_text]
"doc.md" = ["protocol-neutral", "channel-neutral"]

[ci]
file = "ci.yml"
required = ["go test ./..."]
`)

	loaded, err := loadManifest(manifestPath)
	if err != nil {
		t.Fatalf("load manifest: %v", err)
	}

	report := checkManifest(root, loaded)
	if len(report.Failures) != 0 {
		t.Fatalf("unexpected failures: %#v", report.Failures)
	}
	if report.DocumentsChecked != 1 || report.TextContracts != 1 || report.FileGates != 1 {
		t.Fatalf("unexpected counts: %#v", report)
	}
}

func TestCheckManifestReportsMissingToken(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "doc.md"), "Matrix")

	report := checkManifest(root, manifest{
		Documents: []string{"doc.md"},
		RequiredText: map[string][]string{
			"doc.md": []string{"protocol-neutral"},
		},
	})

	if len(report.Failures) != 1 {
		t.Fatalf("expected one failure, got %#v", report.Failures)
	}
}

func TestPatternBudgetAllowsExplicitBaseline(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "internal/logic/allowed.go"), `import "github.com/example/protocol"`)
	mustWrite(t, filepath.Join(root, "internal/logic/new.go"), `package logic`)

	report := checkManifest(root, manifest{
		PatternBudgets: []patternBudget{
			{
				Name:         "protocol_imports",
				Roots:        []string{"internal/logic"},
				Patterns:     []string{"github.com/example/protocol"},
				AllowedFiles: []string{"internal/logic/allowed.go"},
				Max:          0,
			},
		},
	})

	if len(report.Failures) != 0 {
		t.Fatalf("unexpected failures: %#v", report.Failures)
	}
}

func TestPatternBudgetFailsOnNewOccurrence(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "internal/logic/new.go"), `import "github.com/example/protocol"`)

	report := checkManifest(root, manifest{
		PatternBudgets: []patternBudget{
			{
				Name:     "protocol_imports",
				Roots:    []string{"internal/logic"},
				Patterns: []string{"github.com/example/protocol"},
				Max:      0,
			},
		},
	})

	if len(report.Failures) != 1 {
		t.Fatalf("expected one failure, got %#v", report.Failures)
	}
}

func mustWrite(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// A test file and a test fixture may legitimately name an agent: the budget
// guards production behaviour, not the vocabulary of a test. This is what keeps
// a real revert-proof test (which must name the agent it generalises away from)
// from turning the budget into a blocker.
func TestPatternBudgetExcludesTestFilesAndDirs(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "internal/logic/agent.go"), "package logic")
	mustWrite(t, filepath.Join(root, "internal/logic/agent_test.go"),
		"package logic\n\nfunc f(agentID string) bool { return agentID == \"opencode\" || agentID == \"mimo\" }\n")
	mustWrite(t, filepath.Join(root, "internal/logic/testdata/registry.json"), `{"agents":[{"id":"mimo"}]}`)

	report := checkManifest(root, manifest{
		PatternBudgets: []patternBudget{
			{
				Name:            "adhoc_agent_names",
				Roots:           []string{"internal/logic"},
				Patterns:        []string{`"mimo"`},
				ExcludeSuffixes: []string{"_test.go"},
				ExcludeDirs:     []string{"testdata"},
				Max:             0,
			},
			{
				Name:            "agent_literals",
				Roots:           []string{"internal/logic"},
				Patterns:        []string{`"opencode"`},
				ExcludeSuffixes: []string{"_test.go"},
				Max:             0,
			},
		},
	})

	if len(report.Failures) != 0 {
		t.Fatalf("test files and fixtures must be excluded: %#v", report.Failures)
	}
}

// The branch shape catches a branch on a name nobody has enumerated yet, and it
// must not fire on the legitimate empty-string guard that shares the same shape.
func TestPatternBudgetRegexCatchesBranchAndIgnoresEmptyGuard(t *testing.T) {
	root := t.TempDir()
	guard := filepath.Join(root, "internal/logic/agent.go")
	budget := patternBudget{
		Name:          "agent_identity_branch_shape",
		Roots:         []string{"internal/logic"},
		RegexPatterns: []string{`(agentID|agentName|AgentID|agent_id)\s*[!=]=\s*"[^"]+"`},
		Max:           0,
	}

	mustWrite(t, guard, "package logic\n\nfunc f(agentID string) bool { return agentID == \"\" }\n")
	if report := checkManifest(root, manifest{PatternBudgets: []patternBudget{budget}}); len(report.Failures) != 0 {
		t.Fatalf("an empty-string guard must not trip the budget: %#v", report.Failures)
	}

	mustWrite(t, guard, "package logic\n\nfunc f(agentID string) bool { return agentID == \"brandnewagent\" }\n")
	report := checkManifest(root, manifest{PatternBudgets: []patternBudget{budget}})
	if len(report.Failures) != 1 {
		t.Fatalf("a branch on an unseen agent name must fail: %#v", report.Failures)
	}
}

// A budget that measures nothing is decoration, and a mistyped regex must not
// pass silently: the manifest parser ignores keys it does not know, so the
// failure has to be loud.
func TestPatternBudgetRequiresAPatternKindAndAValidRegex(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "internal/logic/agent.go"), "package logic")

	empty := checkManifest(root, manifest{
		PatternBudgets: []patternBudget{
			{Name: "measures_nothing", Roots: []string{"internal/logic"}, Max: 0},
		},
	})
	if len(empty.Failures) != 1 {
		t.Fatalf("a budget without any pattern kind must fail: %#v", empty.Failures)
	}

	broken := checkManifest(root, manifest{
		PatternBudgets: []patternBudget{
			{Name: "broken_regex", Roots: []string{"internal/logic"}, RegexPatterns: []string{"(unclosed"}, Max: 0},
		},
	})
	if len(broken.Failures) != 1 {
		t.Fatalf("an invalid regex must fail loudly: %#v", broken.Failures)
	}
}
