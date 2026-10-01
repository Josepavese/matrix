package deliverycontract

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func digestOf(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}

func write(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return path
}

// TestNotDeclaredIsNotAcceptance is the honesty rule at the base of the whole
// contract: a run with no declared contract produces no positive judgement. If
// this ever reported "accepted", every run in the archive would silently become
// a delivered one.
func TestNotDeclaredIsNotAcceptance(t *testing.T) {
	verdict := Evaluate(context.Background(), t.TempDir(), Contract{})
	if verdict.Status != StatusNotDeclared {
		t.Fatalf("acceptance = %q, want %q", verdict.Status, StatusNotDeclared)
	}
	if verdict.Status == StatusAccepted {
		t.Fatal("silence was upgraded into acceptance")
	}
	if len(verdict.Checks) != 0 {
		t.Fatalf("a contract that declares nothing produced checks: %#v", verdict.Checks)
	}
}

// TestCompletedRunWithoutTheDeclaredArtifactIsIncompleteDelivery is the PM's own
// criterion: the run finished, and the thing the caller asked for is not there.
func TestCompletedRunWithoutTheDeclaredArtifactIsIncompleteDelivery(t *testing.T) {
	workspace := t.TempDir()
	verdict := Evaluate(context.Background(), workspace, Contract{
		Artifacts: []Artifact{{Path: "REPORT.md", SHA256: digestOf("# report")}},
	})
	if verdict.Status != StatusIncomplete {
		t.Fatalf("acceptance = %q, want %q: %#v", verdict.Status, StatusIncomplete, verdict.Checks)
	}
	check := verdict.Checks[0]
	if check.Status != CheckMissing || check.Target != "REPORT.md" {
		t.Fatalf("check = %#v, want the declared artifact reported missing", check)
	}
	if verdict.Reason == "" {
		t.Fatal("an incomplete delivery must say why it is incomplete")
	}
}

// TestPresentArtifactWithTheWrongContentIsNotAccepted covers the case a pure
// existence check would wave through: the file is there and it is not the work
// that was asked for.
func TestPresentArtifactWithTheWrongContentIsNotAccepted(t *testing.T) {
	workspace := t.TempDir()
	write(t, workspace, "REPORT.md", "# something else")
	verdict := Evaluate(context.Background(), workspace, Contract{
		Artifacts: []Artifact{{Path: "REPORT.md", SHA256: digestOf("# report")}},
	})
	if verdict.Status != StatusIncomplete || verdict.Checks[0].Status != CheckMismatch {
		t.Fatalf("acceptance = %q %#v, want a mismatch", verdict.Status, verdict.Checks)
	}
}

// TestArtifactWithoutDigestSaysOnlyExistenceWasChecked keeps the verdict from
// claiming more than it checked. "Present, contents unverified" is a weaker
// claim than "accepted", and the detail has to say which one it is.
func TestArtifactWithoutDigestSaysOnlyExistenceWasChecked(t *testing.T) {
	workspace := t.TempDir()
	write(t, workspace, "REPORT.md", "anything at all")
	verdict := Evaluate(context.Background(), workspace, Contract{
		Artifacts: []Artifact{{Path: "REPORT.md"}},
	})
	if verdict.Status != StatusAccepted {
		t.Fatalf("acceptance = %q, want %q", verdict.Status, StatusAccepted)
	}
	detail := verdict.Checks[0].Detail
	if !strings.Contains(detail, "no digest") || !strings.Contains(detail, "existence") {
		t.Fatalf("the check hides what it did not verify: %q", detail)
	}
}

func TestAcceptedWhenEveryDeclaredRequirementPasses(t *testing.T) {
	workspace := t.TempDir()
	write(t, workspace, "out/REPORT.md", "# report")
	verdict := Evaluate(context.Background(), workspace, Contract{
		Artifacts: []Artifact{{Path: "out/REPORT.md", SHA256: digestOf("# report")}},
	})
	if verdict.Status != StatusAccepted || verdict.Checks[0].Status != CheckPassed {
		t.Fatalf("acceptance = %q %#v, want accepted", verdict.Status, verdict.Checks)
	}
}

// TestNoWorkspaceIsUnverifiableNotIncomplete separates "the work is missing" from
// "Matrix could not look". Reporting the second as the first would be an
// accusation, and reporting it as accepted would be a fabrication.
func TestNoWorkspaceIsUnverifiableNotIncomplete(t *testing.T) {
	verdict := Evaluate(context.Background(), "  ", Contract{Artifacts: []Artifact{{Path: "REPORT.md"}}})
	if verdict.Status != StatusUnverifiable {
		t.Fatalf("acceptance = %q, want %q", verdict.Status, StatusUnverifiable)
	}
}

// TestUnverifiableOutranksIncomplete: when one requirement cannot be evaluated,
// the verdict as a whole is not trustworthy, and a neighbouring failure must not
// make it look like a clean "incomplete".
func TestUnverifiableOutranksIncomplete(t *testing.T) {
	workspace := t.TempDir()
	verdict := Evaluate(context.Background(), workspace, Contract{
		Artifacts: []Artifact{{Path: "missing.md"}, {Path: "../escape.md"}},
	})
	if verdict.Status != StatusUnverifiable {
		t.Fatalf("acceptance = %q, want %q: %#v", verdict.Status, StatusUnverifiable, verdict.Checks)
	}
}

func TestArtifactPathCannotLeaveTheRunWorkspace(t *testing.T) {
	workspace := t.TempDir()
	write(t, workspace, "../outside.md", "secret")
	cases := []string{"../outside.md", "/etc/passwd", "a/../../outside.md"}
	for _, path := range cases {
		verdict := Evaluate(context.Background(), workspace, Contract{Artifacts: []Artifact{{Path: path}}})
		if verdict.Status != StatusUnverifiable {
			t.Fatalf("declared path %q produced %q, want %q", path, verdict.Status, StatusUnverifiable)
		}
		if verdict.Checks[0].Status != CheckError {
			t.Fatalf("declared path %q was not refused: %#v", path, verdict.Checks[0])
		}
	}
}

// TestArtifactSymlinkOutOfTheWorkspaceIsRefused covers the path check that a
// lexical containment test alone would miss.
func TestArtifactSymlinkOutOfTheWorkspaceIsRefused(t *testing.T) {
	workspace := t.TempDir()
	outside := t.TempDir()
	target := write(t, outside, "secret.txt", "secret")
	if err := os.Symlink(target, filepath.Join(workspace, "link.txt")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	verdict := Evaluate(context.Background(), workspace, Contract{Artifacts: []Artifact{{Path: "link.txt"}}})
	if verdict.Status != StatusUnverifiable || verdict.Checks[0].Status != CheckError {
		t.Fatalf("a link out of the workspace was accepted: %q %#v", verdict.Status, verdict.Checks)
	}
}

// TestValidatorExitCodeDecidesAcceptance is the reason the validator exists: the
// files are present and the caller's own check says the work was not delivered.
func TestValidatorExitCodeDecidesAcceptance(t *testing.T) {
	workspace := t.TempDir()
	write(t, workspace, "REPORT.md", "# report")
	restore := stubValidator(func(context.Context, string, []string) (int, error) { return 3, nil })
	defer restore()

	verdict := Evaluate(context.Background(), workspace, Contract{
		Artifacts: []Artifact{{Path: "REPORT.md"}},
		Validator: &Validator{Command: []string{"check-delivery"}},
	})
	if verdict.Status != StatusIncomplete {
		t.Fatalf("acceptance = %q, want %q", verdict.Status, StatusIncomplete)
	}
	last := verdict.Checks[len(verdict.Checks)-1]
	if last.Status != CheckFailed || !strings.Contains(last.Detail, "3") {
		t.Fatalf("validator check = %#v, want the exit code reported", last)
	}
}

// TestValidatorOutputNeverReachesTheVerdict enforces the Lead's fourth constraint
// with teeth: a validator that prints something sensitive must not be able to
// put it into the record Matrix keeps.
func TestValidatorOutputNeverReachesTheVerdict(t *testing.T) {
	workspace := t.TempDir()
	restore := stubValidator(func(context.Context, string, []string) (int, error) {
		// Standing in for a command whose output must not be imported: whatever
		// it would have printed is simply not part of what this function returns.
		return 1, nil
	})
	defer restore()

	verdict := Evaluate(context.Background(), workspace, Contract{
		Validator: &Validator{Command: []string{"noisy-validator"}},
	})
	encoded, err := json.Marshal(verdict)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if strings.Contains(string(encoded), "noisy-validator") {
		t.Fatalf("the verdict carries the validator's command: %s", encoded)
	}
	if !strings.Contains(string(encoded), "exited with code 1") {
		t.Fatalf("the verdict does not carry the exit code: %s", encoded)
	}
}

// TestValidatorRunsInTheRunWorkspace pins the third constraint: the working
// directory is the run's, so the caller's check sees the work and not the
// server's directory.
func TestValidatorRunsInTheRunWorkspace(t *testing.T) {
	workspace := t.TempDir()
	var seen string
	restore := stubValidator(func(_ context.Context, dir string, argv []string) (int, error) {
		seen = dir
		if len(argv) != 2 || argv[0] != "git" || argv[1] != "status" {
			t.Fatalf("argv was rewritten: %#v", argv)
		}
		return 0, nil
	})
	defer restore()

	verdict := Evaluate(context.Background(), workspace, Contract{Validator: &Validator{Command: []string{"git", "status"}}})
	if seen != workspace {
		t.Fatalf("validator ran in %q, want the run workspace %q", seen, workspace)
	}
	if verdict.Status != StatusAccepted {
		t.Fatalf("acceptance = %q, want %q", verdict.Status, StatusAccepted)
	}
}

// TestValidatorIsARealProcess proves the seam above is wired to a real command
// and not only to a stub: the argv reaches a process, and the exit code comes
// back through the real path.
func TestValidatorIsARealProcess(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skipf("no shell available: %v", err)
	}
	cases := []struct {
		argv []string
		want string
	}{
		{[]string{"sh", "-c", "exit 0"}, StatusAccepted},
		{[]string{"sh", "-c", "exit 7"}, StatusIncomplete},
	}
	for _, tc := range cases {
		verdict := Evaluate(context.Background(), t.TempDir(), Contract{Validator: &Validator{Command: tc.argv}})
		if verdict.Status != tc.want {
			t.Fatalf("%v produced %q, want %q", tc.argv, verdict.Status, tc.want)
		}
	}
}

func TestValidatorThatCannotBeRunIsUnverifiable(t *testing.T) {
	restore := stubValidator(func(context.Context, string, []string) (int, error) {
		return 0, context.DeadlineExceeded
	})
	defer restore()
	verdict := Evaluate(context.Background(), t.TempDir(), Contract{
		Validator: &Validator{Command: []string{"slow"}, TimeoutSeconds: 1},
	})
	if verdict.Status != StatusUnverifiable {
		t.Fatalf("acceptance = %q, want %q: %#v", verdict.Status, StatusUnverifiable, verdict.Checks)
	}
	if !strings.Contains(verdict.Checks[0].Detail, "1s") {
		t.Fatalf("the timeout is not reported: %#v", verdict.Checks[0])
	}
}

// TestContractRejectsAShellString covers the first constraint: a command is an
// argv array. A caller who writes one string gets a refusal, not a command line
// Matrix guessed how to split.
func TestContractRejectsAShellString(t *testing.T) {
	cases := []Contract{
		{Validator: &Validator{Command: []string{"git diff --quiet"}}},
		{Validator: &Validator{Command: []string{}}},
		{Artifacts: []Artifact{{Path: "  "}}},
		{Artifacts: []Artifact{{Path: "a.md", SHA256: "deadbeef"}}},
		{Validator: &Validator{Command: []string{"true"}, TimeoutSeconds: -1}},
	}
	for _, contract := range cases {
		if err := contract.Validate(); err == nil {
			t.Fatalf("contract %#v was accepted", contract)
		}
	}
	// A shell is allowed when the caller writes one explicitly: it is then a
	// declared choice, not something Matrix introduced.
	explicit := Contract{Validator: &Validator{Command: []string{"sh", "-c", "git diff --quiet"}}}
	if err := explicit.Validate(); err != nil {
		t.Fatalf("an explicit shell was refused: %v", err)
	}
}

// TestDeclaredContractWithAValidatorIsStillCheckedWhenNothingElseIs: the
// protocol status is irrelevant to acceptance, and acceptance is irrelevant to
// the protocol status. They are two answers to two questions.
func TestAcceptanceAndProtocolStatusAreSeparate(t *testing.T) {
	workspace := t.TempDir()
	verdict := Evaluate(context.Background(), workspace, Contract{
		Artifacts: []Artifact{{Path: "REPORT.md", SHA256: digestOf("x")}},
	})
	if verdict.Status == "" || verdict.Status == "completed" || verdict.Status == "failed" {
		t.Fatalf("acceptance borrowed the protocol vocabulary: %q", verdict.Status)
	}
}

func stubValidator(fn func(context.Context, string, []string) (int, error)) func() {
	previous := runValidatorCommand
	runValidatorCommand = fn
	return func() { runValidatorCommand = previous }
}

// TestArtifactThatCannotBeResolvedIsUnverifiable: a link loop is not a missing
// artifact. Folding it into "missing" would accuse the run of not delivering,
// when what actually happened is that Matrix could not look.
func TestArtifactThatCannotBeResolvedIsUnverifiable(t *testing.T) {
	workspace := t.TempDir()
	loop := filepath.Join(workspace, "loop.md")
	if err := os.Symlink(loop, loop); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	verdict := Evaluate(context.Background(), workspace, Contract{Artifacts: []Artifact{{Path: "loop.md"}}})
	if verdict.Status != StatusUnverifiable || verdict.Checks[0].Status != CheckError {
		t.Fatalf("a link loop produced %q: %#v", verdict.Status, verdict.Checks)
	}
}
