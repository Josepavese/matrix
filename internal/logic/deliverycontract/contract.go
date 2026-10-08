// Package deliverycontract evaluates the delivery contract a caller declares
// before a run: what the work is supposed to leave behind, and how the caller
// can tell, deterministically, whether it did.
//
// The package exists because "the protocol said completed" and "the work was
// delivered" are different claims. A run can finish cleanly, with a model
// attested and a turn that really ended, and still leave nothing behind: the
// real case that produced this contract was a run reported as completed whose
// report was never written and whose workspace never existed. Matrix must be
// able to report the second claim without inventing a provider error, without
// reading a transcript, and without deciding on the caller's behalf what
// "delivered" means.
//
// So the caller declares the contract, and Matrix evaluates it as literally as
// it can: a declared artifact must exist, a declared digest must match, a
// declared validator must exit zero. Everything else is reported as not
// declared, and nothing here ever upgrades silence into acceptance.
package deliverycontract

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/Josepavese/matrix/internal/middleware"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// The acceptance vocabulary. These are deliberately not the protocol statuses:
// a run is completed or failed, and its delivery is accepted, incomplete,
// unverifiable or not declared. One is what the protocol reported, the other is
// what the declared contract could actually be checked against, and conflating
// them is the confusion this package was written to remove.
const (
	// StatusNotDeclared means the caller declared no contract. Matrix says
	// nothing about delivery in this case, and in particular does not say
	// "accepted": the absence of a contract is not evidence of delivery.
	StatusNotDeclared = "not_declared"
	// StatusAccepted means every declared check passed.
	StatusAccepted = "accepted"
	// StatusIncomplete means at least one declared check failed: an artifact the
	// caller required is missing or different, or the caller's validator said no.
	StatusIncomplete = "incomplete"
	// StatusUnverifiable means the contract could not be evaluated at all — no
	// workspace, an unreadable path, a validator that could not be run. It is
	// reported as its own outcome rather than folded into "incomplete", because
	// Matrix did not prove the work was missing, only that it could not look.
	StatusUnverifiable = "unverifiable"
)

// Check outcomes.
const (
	CheckPassed   = "passed"
	CheckMissing  = "missing"
	CheckMismatch = "mismatch"
	CheckFailed   = "failed"
	CheckError    = "error"
)

// Contract is what a caller declares before a run.
type Contract struct {
	Artifacts []Artifact `json:"artifacts,omitempty"`
	Validator *Validator `json:"validator,omitempty"`
}

// Artifact is a file the caller requires the run to leave behind, relative to
// the run workspace. When SHA256 is empty only existence is checked, and the
// verdict says so rather than implying the content was verified.
type Artifact struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256,omitempty"`
}

// Validator is the caller's own deterministic check, run in the run workspace
// once the run reaches a terminal state. It exists so that "committed",
// "formatted" or "tests passed" stay the caller's definitions: Matrix runs the
// check and reports the exit code instead of learning what a commit is.
type Validator struct {
	Sandbox        *middleware.ContainerSandbox `json:"sandbox,omitempty"`
	Command        []string                     `json:"command"`
	TimeoutSeconds int                          `json:"timeout_seconds,omitempty"`
}

// Check is one evaluated requirement, kept as its own record so a reader can see
// which requirement failed instead of only that something did.
type Check struct {
	Target string `json:"target"`
	Status string `json:"status"`
	Detail string `json:"detail,omitempty"`
}

// Verdict is the evaluated contract.
type Verdict struct {
	Status string  `json:"acceptance_status"`
	Checks []Check `json:"checks,omitempty"`
	Reason string  `json:"reason,omitempty"`
}

// Declared reports whether the caller declared anything at all. A contract that
// declares nothing is not an accepted delivery.
func (c Contract) Declared() bool {
	return len(c.Artifacts) > 0 || c.Validator != nil
}

// maxContractArtifacts bounds how many artifacts one contract may declare. Every
// declared artifact costs the terminal a stat and a hash, so an unbounded list is
// work a caller decides to make Matrix do for a single run. The number is far
// above a real delivery - a handful of files, and every declaration in this
// repository's own tests carries one or two - and it is not the limit on the
// request body read twice: a megabyte of json holds tens of thousands of minimal
// entries, so the size of a declaration and its number of requirements need a
// ceiling each.
const maxContractArtifacts = 32

// Validate rejects a contract that cannot be evaluated as declared, before the
// run starts. A contract that cannot be checked is worse than no contract: it
// would produce an acceptance nobody can stand behind.
//
// It is the door and nothing else: how much was declared, and whether what was
// declared has a shape that can be checked. The two questions are answered by the
// two functions below, so the door reads as a sequence instead of a tree of cases.
func (c Contract) Validate() error {
	if !c.Declared() {
		return nil
	}
	if err := c.validateQuantity(); err != nil {
		return err
	}
	return c.validateShape()
}

// validateQuantity answers how much the caller declared. A list of artifacts is
// work the terminal is asked to do, so it needs a ceiling of its own; the shape of
// each entry is not this function's business.
func (c Contract) validateQuantity() error {
	if len(c.Artifacts) > maxContractArtifacts {
		return fmt.Errorf("delivery contract: %d artifacts are declared and the limit is %d", len(c.Artifacts), maxContractArtifacts)
	}
	return nil
}

// validateShape answers whether what was declared can be checked at all: a path
// to stat, a digest that is one, a validator that is an argv array with a
// non-negative timeout.
func (c Contract) validateShape() error {
	if err := validateValidatorSandbox(c.Validator); err != nil {
		return err
	}
	for _, artifact := range c.Artifacts {
		if strings.TrimSpace(artifact.Path) == "" {
			return errors.New("delivery contract: an artifact is declared without a path")
		}
		if artifact.SHA256 != "" && !isHexDigest(artifact.SHA256) {
			return fmt.Errorf("delivery contract: artifact %q declares a sha256 that is not 64 hexadecimal characters", artifact.Path)
		}
	}
	if c.Validator != nil {
		if len(c.Validator.Command) == 0 {
			return errors.New("delivery contract: the validator declares no command")
		}
		if len(c.Validator.Command) == 1 && strings.ContainsAny(c.Validator.Command[0], " \t") {
			return errors.New("delivery contract: the validator command is an argv array, never a shell string; Matrix will not guess how to split it")
		}
		if c.Validator.TimeoutSeconds < 0 {
			return errors.New("delivery contract: the validator timeout cannot be negative")
		}
	}
	return nil
}

func isHexDigest(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, r := range strings.ToLower(value) {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return false
		}
	}
	return true
}

// Evaluate checks the declared contract against the run workspace. It never
// returns an error: a contract that cannot be evaluated is a verdict of its own,
// because "could not check" is a fact the caller needs and an error return would
// push the caller into reporting either success or failure.
func Evaluate(ctx context.Context, workspace string, contract Contract) Verdict {
	return EvaluateWithValidator(ctx, workspace, contract, nil)
}

// EvaluateWithValidator requires a supplied execution boundary when isolation
// was requested; absence of a runner never causes host execution as a fallback.
func EvaluateWithValidator(ctx context.Context, workspace string, contract Contract, runner ValidatorRunner) Verdict {
	if !contract.Declared() {
		return Verdict{Status: StatusNotDeclared, Reason: "no delivery contract was declared for this run"}
	}
	if err := contract.Validate(); err != nil {
		return Verdict{Status: StatusUnverifiable, Reason: err.Error()}
	}
	if strings.TrimSpace(workspace) == "" {
		return Verdict{Status: StatusUnverifiable, Reason: "the run has no workspace, so the declared contract could not be checked"}
	}
	verdict := Verdict{Status: StatusAccepted}
	for _, artifact := range contract.Artifacts {
		verdict.add(checkArtifact(workspace, artifact))
	}
	if contract.Validator != nil {
		verdict.add(checkValidatorWithRunner(ctx, workspace, *contract.Validator, runner))
	}
	if len(verdict.Checks) == 0 {
		return Verdict{Status: StatusNotDeclared, Reason: "no delivery contract was declared for this run"}
	}
	verdict.explain()
	return verdict
}

// explain gives every non-accepted verdict a reason. A caller reading
// "incomplete" without knowing which requirement failed has to go back to the
// workspace and guess, which is the work the contract exists to do for them.
func (v *Verdict) explain() {
	switch v.Status {
	case StatusAccepted:
		v.Reason = "every declared requirement was checked and passed"
	case StatusIncomplete:
		v.Reason = fmt.Sprintf("%d of %d declared requirements were not satisfied", failedChecks(v.Checks), len(v.Checks))
	case StatusUnverifiable:
		v.Reason = "the declared delivery contract could not be evaluated"
	}
}

func failedChecks(checks []Check) int {
	failed := 0
	for _, check := range checks {
		if check.Status != CheckPassed {
			failed++
		}
	}
	return failed
}

// add folds one check into the verdict. A failed check can only be downgraded by
// another failed check: once something is missing, a later pass must not restore
// acceptance, and an unevaluable check outranks a failed one because it means the
// verdict itself is not trustworthy.
func (v *Verdict) add(check Check) {
	v.Checks = append(v.Checks, check)
	switch check.Status {
	case CheckPassed:
		return
	case CheckError:
		v.Status = StatusUnverifiable
	case CheckMissing, CheckMismatch, CheckFailed:
		if v.Status != StatusUnverifiable {
			v.Status = StatusIncomplete
		}
	}
}

func checkArtifact(workspace string, artifact Artifact) Check {
	check := Check{Target: artifact.Path}
	path, err := resolveArtifact(workspace, artifact.Path)
	if err != nil {
		return check.as(CheckError, err.Error())
	}
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return check.as(CheckMissing, "the declared artifact was not produced")
		}
		return check.as(CheckError, "the declared artifact could not be read: "+err.Error())
	}
	if info.IsDir() {
		return check.as(CheckError, "the declared artifact is a directory, not a file")
	}
	if artifact.SHA256 == "" {
		return check.as(CheckPassed, "present; the contract declared no digest, so only existence was checked")
	}
	digest, err := digestFile(path)
	if err != nil {
		return check.as(CheckError, "the declared artifact could not be hashed: "+err.Error())
	}
	if !strings.EqualFold(digest, artifact.SHA256) {
		return check.as(CheckMismatch, "sha256 "+digest+" does not match the declared "+strings.ToLower(artifact.SHA256))
	}
	return check.as(CheckPassed, "sha256 "+digest)
}

func digestFile(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	digest := sha256.New()
	if _, err := io.Copy(digest, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(digest.Sum(nil)), nil
}

// resolveArtifact keeps a caller-declared path inside the run workspace. The
// check is done twice on purpose: once on the path as written, and once after
// resolving symlinks, so a link planted inside the workspace cannot point the
// check at a file the run was never entitled to have verified.
func resolveArtifact(workspace, target string) (string, error) {
	if filepath.IsAbs(target) {
		return "", fmt.Errorf("declared path %q is absolute; contract paths are relative to the run workspace", target)
	}
	root := filepath.Clean(workspace)
	path := filepath.Clean(filepath.Join(root, target))
	if !within(root, path) {
		return "", fmt.Errorf("declared path %q resolves outside the run workspace", target)
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		// A path that does not exist yet is not a resolution failure: the check
		// that follows reports it as missing, which is the honest outcome. Any
		// other failure — a link loop, a directory that cannot be traversed — is
		// a real inability to look, and it must not be flattened into "missing".
		if errors.Is(err, fs.ErrNotExist) {
			return path, nil
		}
		return "", fmt.Errorf("declared path %q could not be resolved: %w", target, err)
	}
	if !within(root, filepath.Clean(resolved)) {
		return "", fmt.Errorf("declared path %q is a link pointing outside the run workspace", target)
	}
	return resolved, nil
}

func within(root, path string) bool {
	return path != root && strings.HasPrefix(path, root+string(filepath.Separator))
}

func (c Check) as(status, detail string) Check {
	c.Status = status
	c.Detail = detail
	return c
}
