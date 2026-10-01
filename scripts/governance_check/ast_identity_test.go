package main

import (
	"path/filepath"
	"strings"
	"testing"
)

// identityBudget is the shape under test in this file: a named value whose role
// is an identity, compared to a non-empty literal, with no reviewed pairs.
func identityBudget(roots ...string) patternBudget {
	if len(roots) == 0 {
		roots = []string{"internal/logic"}
	}
	return patternBudget{
		Name:            "identity_comparison_shape",
		Roots:           roots,
		ASTRule:         astRuleIdentityComparison,
		IdentityNames:   []string{"name", "agentName", "AgentName", "agentID", "provider", "binary"},
		AgentNameValues: []string{"codex", "opencode", "claude", "gemini", "mimo", "minimax", "halfpocket", "kimi", "deepseek"},
		ExcludeSuffixes: []string{"_test.go"},
		ExcludeDirs:     []string{"testdata"},
		Max:             0,
	}
}

// The gap this rule exists to close: the variable is generic and the name has
// never been enumerated anywhere, so no literal list can catch it.
func TestIdentityComparisonFiresOnAnUnseenNameOnAGenericVariable(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "internal/logic/lookup.go"),
		"package logic\n\nfunc f(name string, kind string) bool { return name == \"brandnewagent\" && kind == \"acp\" }\n")

	report := checkManifest(root, manifest{PatternBudgets: []patternBudget{identityBudget()}})
	if len(report.Failures) != 1 {
		t.Fatalf("a comparison on an identity variable must fail: %#v", report.Failures)
	}
	if !strings.Contains(report.Failures[0], `"brandnewagent"`) {
		t.Fatalf("the failure must name the literal it caught: %#v", report.Failures)
	}
}

// The check must stay true: a classifier compared to a literal is how the tree
// describes closed vocabularies, and the empty-string guard is not an identity
// test. Flagging either would block healthy work and get the check disabled.
func TestIdentityComparisonIgnoresClassifiersAndEmptyGuards(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "internal/logic/classify.go"), `package logic

func f(kind, transport, status, phase, origin, agentID string) bool {
	if kind == "acp" || transport == "stdio" || phase == "final_answer" {
		return true
	}
	if status == "pending" || origin == "resumed" {
		return true
	}
	return agentID == "" || kind != ""
}
`)

	report := checkManifest(root, manifest{PatternBudgets: []patternBudget{identityBudget()}})
	if len(report.Failures) != 0 {
		t.Fatalf("classifiers and empty guards must not trip the shape check: %#v", report.Failures)
	}
}

// A call, a dereference or a short/multi-word literal carries no identity: the
// check reasons about named values only, and says so rather than guessing.
func TestIdentityComparisonIgnoresUnnamedValuesAndNonIdentityLiterals(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "internal/logic/other.go"), `package logic

func f(name string, raw []byte, p *string, op string) bool {
	if string(raw) == "null" {
		return true
	}
	if *p == "codex" {
		return true
	}
	if op == "&&" {
		return true
	}
	if name == "a" || name == "two words" {
		return true
	}
	return false
}
`)

	report := checkManifest(root, manifest{PatternBudgets: []patternBudget{identityBudget()}})
	if len(report.Failures) != 0 {
		t.Fatalf("unnamed operands and non-identity literals must not fail: %#v", report.Failures)
	}
}

// Reviewing a PAIR must not blind the check to the same variable: clearing
// name=fork is a statement about that literal, not about `name`.
func TestIdentityComparisonReviewedPairDoesNotBlindTheName(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "internal/logic/capabilities.go")
	budget := identityBudget()
	budget.ReviewedPairs = []string{"name=fork"}

	mustWrite(t, path, "package logic\n\nfunc f(name string) bool { return name == \"fork\" }\n")
	if report := checkManifest(root, manifest{PatternBudgets: []patternBudget{budget}}); len(report.Failures) != 0 {
		t.Fatalf("a reviewed pair must pass: %#v", report.Failures)
	}

	mustWrite(t, path, "package logic\n\nfunc f(name string) bool { return name == \"fork\" || name == \"brandnewagent\" }\n")
	report := checkManifest(root, manifest{PatternBudgets: []patternBudget{budget}})
	if len(report.Failures) != 1 {
		t.Fatalf("clearing one pair must not clear the next literal: %#v", report.Failures)
	}
	if !strings.Contains(report.Failures[0], `"brandnewagent"`) {
		t.Fatalf("the failure must name the unreviewed literal: %#v", report.Failures)
	}
}

// A map keyed by a string literal names its field as clearly as a struct field
// does, and a test file may name the agent it generalises away from.
func TestIdentityComparisonReadsMapKeysAndSkipsTests(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "internal/logic/config.go"),
		"package logic\n\nfunc f(state map[string]string) bool { return state[\"provider\"] == \"OpenRouter\" }\n")
	mustWrite(t, filepath.Join(root, "internal/logic/config_test.go"),
		"package logic\n\nfunc g(name string) bool { return name == \"mimo\" }\n")
	mustWrite(t, filepath.Join(root, "internal/logic/testdata/fixture.go"),
		"package logic\n\nfunc h(name string) bool { return name == \"kimi\" }\n")

	report := checkManifest(root, manifest{PatternBudgets: []patternBudget{identityBudget()}})
	if len(report.Failures) != 1 || !strings.Contains(report.Failures[0], `"OpenRouter"`) {
		t.Fatalf("a literal map key must be read as a name, tests must be skipped: %#v", report.Failures)
	}
}

// A file the checker cannot parse must fail loudly: skipping it would exempt
// exactly the code nobody can read.
func TestIdentityComparisonFailsLoudlyOnUnparseableGo(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "internal/logic/broken.go"), "package logic\n\nfunc f( {\n")

	report := checkManifest(root, manifest{PatternBudgets: []patternBudget{identityBudget()}})
	if len(report.Failures) != 1 {
		t.Fatalf("an unparseable file must fail the check: %#v", report.Failures)
	}
	if !strings.Contains(report.Failures[0], "scan failed") {
		t.Fatalf("the failure must say the scan failed: %#v", report.Failures)
	}
}

// A budget that measures nothing is decoration, and a rule the checker does not
// know must not pass silently.
func TestIdentityComparisonRequiresItsConfiguration(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "internal/logic/agent.go"), "package logic")

	unnamed := identityBudget()
	unnamed.IdentityNames = nil
	if report := checkManifest(root, manifest{PatternBudgets: []patternBudget{unnamed}}); len(report.Failures) != 1 {
		t.Fatalf("a shape check without identity_names must fail: %#v", report.Failures)
	}

	unknown := identityBudget()
	unknown.ASTRule = "something_else"
	if report := checkManifest(root, manifest{PatternBudgets: []patternBudget{unknown}}); len(report.Failures) != 1 {
		t.Fatalf("an unknown ast_rule must fail loudly: %#v", report.Failures)
	}

	// An AST-only budget measures something, so it must not be rejected as empty
	// the way a budget with no pattern kind at all is.
	if report := checkManifest(root, manifest{PatternBudgets: []patternBudget{identityBudget()}}); len(report.Failures) != 0 {
		t.Fatalf("an AST-only budget is a pattern kind: %#v", report.Failures)
	}
}

func TestManifestLoadsASTRuleKeys(t *testing.T) {
	root := t.TempDir()
	manifestPath := filepath.Join(root, "manifest.toml")
	mustWrite(t, manifestPath, `
[pattern_budget.shape]
roots = ["internal/logic"]
ast_rule = "identity_comparison"
identity_names = ["name", "agentID"]
reviewed_pairs = ["name=fork"]
exclude_suffixes = ["_test.go"]
max = 0
reason = "shape"
`)

	loaded, err := loadManifest(manifestPath)
	if err != nil {
		t.Fatalf("load manifest: %v", err)
	}
	if len(loaded.PatternBudgets) != 1 {
		t.Fatalf("expected one budget, got %#v", loaded.PatternBudgets)
	}
	budget := loaded.PatternBudgets[0]
	if budget.ASTRule != astRuleIdentityComparison {
		t.Fatalf("ast_rule not loaded: %#v", budget)
	}
	if len(budget.IdentityNames) != 2 || budget.IdentityNames[1] != "agentID" {
		t.Fatalf("identity_names not loaded: %#v", budget.IdentityNames)
	}
	if len(budget.ReviewedPairs) != 1 || budget.ReviewedPairs[0] != "name=fork" {
		t.Fatalf("reviewed_pairs not loaded: %#v", budget.ReviewedPairs)
	}
}

// The hole this extension closes, measured before it was closed: a branch on a
// constant that denominates an agent passed every budget, because no budget read
// the constant's value.
func TestIdentityComparisonFiresOnAConstantThatDenominatesAnAgent(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "internal/logic/launch.go"), `package logic

const agentCodex = "codex"

func pick(agentID string) string {
	if agentID == agentCodex {
		return "codex-path"
	}
	return "generic-path"
}
`)

	report := checkManifest(root, manifest{PatternBudgets: []patternBudget{identityBudget()}})
	if len(report.Failures) != 1 {
		t.Fatalf("a comparison against a constant naming an agent must fail: %#v", report.Failures)
	}
	if !strings.Contains(report.Failures[0], "const agentCodex") || !strings.Contains(report.Failures[0], `"codex"`) {
		t.Fatalf("the failure must name the constant and its value: %#v", report.Failures)
	}
}

// A constant that lives in another package is the shape the tree actually uses:
// agentidentity declares the canonical id, agentlaunch and the wizard read it.
func TestIdentityComparisonResolvesAQualifiedConstantFromAnotherPackage(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "go.mod"), "module example.com/tree\n\ngo 1.24\n")
	mustWrite(t, filepath.Join(root, "internal/logic/agentidentity/codex.go"),
		"package agentidentity\n\nconst CanonicalCodexAgentID = \"codex\"\n")
	mustWrite(t, filepath.Join(root, "internal/logic/launch.go"), `package logic

import "example.com/tree/internal/logic/agentidentity"

func pick(agentID string) string {
	if agentID == agentidentity.CanonicalCodexAgentID {
		return "codex-path"
	}
	return "generic-path"
}
`)

	report := checkManifest(root, manifest{PatternBudgets: []patternBudget{identityBudget()}})
	if len(report.Failures) != 1 {
		t.Fatalf("a qualified constant naming an agent must fail: %#v", report.Failures)
	}
	if !strings.Contains(report.Failures[0], "const agentidentity.CanonicalCodexAgentID") {
		t.Fatalf("the failure must name the constant as written: %#v", report.Failures)
	}
}

// The check must stay true: a classifier compared to a constant is how the tree
// describes closed vocabularies, and the unset guard is not an identity test.
// The constant rule judges the SHAPE on an identity role, not the value, so a
// classifier must be left alone by its ROLE.
func TestIdentityComparisonIgnoresClassifierConstantsAndEmptyGuards(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "internal/logic/classify.go"), `package logic

const (
	authTypeAgent = "agent"
	kindACP       = "acp"
	unsetAgent    = ""
)

func f(kind, status, agentID string) bool {
	if kind == kindACP || status == authTypeAgent {
		return true
	}
	return agentID == unsetAgent
}
`)

	report := checkManifest(root, manifest{PatternBudgets: []patternBudget{identityBudget()}})
	if len(report.Failures) != 0 {
		t.Fatalf("classifier roles and the unset guard must not trip the shape check: %#v", report.Failures)
	}
}

// A constant on an identity role whose value names no agent is still a branch
// with its value one indirection away: the shape fails it, and the message tells
// the reviewer the value names no agent, so the pair can be reviewed on its
// merits instead of by opening the constant by hand.
func TestIdentityComparisonFiresOnAConstantWhoseValueNamesNoAgent(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "internal/logic/providers.go"), `package logic

const providerOpenRouter = "OpenRouter"

func f(provider string) bool { return provider == providerOpenRouter }
`)

	report := checkManifest(root, manifest{PatternBudgets: []patternBudget{identityBudget()}})
	if len(report.Failures) != 1 {
		t.Fatalf("a constant on an identity role must fail even when its value names no agent: %#v", report.Failures)
	}
	if !strings.Contains(report.Failures[0], "names no agent in the vocabulary") {
		t.Fatalf("the failure must say the value names no agent: %#v", report.Failures)
	}
}

// The probe that chose the design: gating the constant rule on the value let a
// NEW agent name behind a NEW constant pass every budget (measured: failures 0),
// which is the same hole one indirection further out. The shape gate closes it.
func TestIdentityComparisonFiresOnANewAgentNameBehindANewConstant(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "internal/logic/newagent.go"), `package logic

const probeSomeBrandNewAgent = "somebrandnewagent"

func f(agentID string) bool { return agentID == probeSomeBrandNewAgent }
`)

	report := checkManifest(root, manifest{PatternBudgets: []patternBudget{identityBudget()}})
	if len(report.Failures) != 1 {
		t.Fatalf("a name nobody enumerated, hidden behind a constant, must still fail: %#v", report.Failures)
	}
}

// The whole-word rule cuts both ways: `codex-acp` DOES denominate the codex
// agent, because the name is a word there. A registry id is an identity, not a
// classifier, and comparing it is a branch on which agent.
func TestIdentityComparisonTreatsARegistryIDAsAnIdentity(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "internal/logic/registry.go"), `package logic

const CodexRegistryID = "codex-acp"

func f(agentID string) bool { return agentID == CodexRegistryID }
`)

	report := checkManifest(root, manifest{PatternBudgets: []patternBudget{identityBudget()}})
	if len(report.Failures) != 1 {
		t.Fatalf("a registry id names an agent and must fail: %#v", report.Failures)
	}
	if !strings.Contains(report.Failures[0], "which names an agent") {
		t.Fatalf("the failure must say the value names an agent: %#v", report.Failures)
	}
}

// The whole-word rule is what keeps the label from over-claiming: "mimosa" is not
// the MiMo agent, and the finding must say so rather than let a reviewer believe
// the branch is on an agent.
func TestIdentityComparisonWholeWordRuleIsReportedHonestly(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "internal/logic/mimosa.go"), `package logic

const providerMimosa = "mimosa"

func f(provider string) bool { return provider == providerMimosa }
`)

	report := checkManifest(root, manifest{PatternBudgets: []patternBudget{identityBudget()}})
	if len(report.Failures) != 1 {
		t.Fatalf("the shape must fail whatever the value: %#v", report.Failures)
	}
	if !strings.Contains(report.Failures[0], "names no agent in the vocabulary") {
		t.Fatalf("a word merely containing a name must not be reported as an agent: %#v", report.Failures)
	}
}

// Clearing a constant pair is a statement about that constant on that name, not
// about the name: the same constant on another identity variable must still fail,
// and so must a different constant on the cleared variable.
func TestIdentityComparisonReviewedConstantPairDoesNotBlindTheName(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "internal/logic/wizard.go"), `package logic

const (
	agentCodex    = "codex"
	agentOpencode = "opencode"
)

func f(agentName, provider string) bool {
	if agentName == agentCodex || provider == agentCodex {
		return true
	}
	return agentName == agentOpencode
}
`)

	budget := identityBudget()
	budget.ReviewedPairs = []string{"agentName=const:agentCodex"}
	report := checkManifest(root, manifest{PatternBudgets: []patternBudget{budget}})
	if len(report.Failures) != 1 {
		t.Fatalf("the two unreviewed constants must fail the budget: %#v", report.Failures)
	}
	if got := strings.Count(report.Failures[0], "\n  - "); got != 2 {
		t.Fatalf("a cleared pair must clear exactly that pair, got %d findings: %#v", got, report.Failures)
	}
	if strings.Contains(report.Failures[0], "compares agentName against const agentCodex") {
		t.Fatalf("the reviewed pair was not cleared: %#v", report.Failures)
	}
}

// Reading a constant's value is a capability that has to be configured: without
// the vocabulary the check would silently go back to ignoring constants, which is
// the hole this extension exists to close.
func TestIdentityComparisonRequiresAgentNameValues(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "internal/logic/agent.go"), "package logic")

	unconfigured := identityBudget()
	unconfigured.AgentNameValues = nil
	report := checkManifest(root, manifest{PatternBudgets: []patternBudget{unconfigured}})
	if len(report.Failures) != 1 || !strings.Contains(report.Failures[0], "agent_name_values") {
		t.Fatalf("a shape check without agent_name_values must fail loudly: %#v", report.Failures)
	}
}
