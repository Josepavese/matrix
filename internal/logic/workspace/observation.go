package workspace

import (
	"fmt"
	"path/filepath"
	"strings"
)

// Observation is one side of the workspace comparison a run publishes: what the
// caller asked for, and what the run resolved to.
//
// RealChildCwd is the directory Matrix starts the agent process in. It is filled
// by the transport that starts the child, because that is the only layer that
// knows the child exists; a caller cannot request it.
//
// GitCommonDir and Branch are not derived by Matrix. Reading them means
// interrogating the workspace's repository, and Matrix does not run git against
// a caller's checkout to describe a run. They stay empty here, and the
// comparison says so with a reason, instead of reporting a value that depends on
// whichever working copy happened to be interrogated.
type Observation struct {
	WorkspaceID  string `json:"workspace_id,omitempty"`
	Path         string `json:"path,omitempty"`
	RealChildCwd string `json:"real_child_cwd,omitempty"`
	GitCommonDir string `json:"git_common_dir,omitempty"`
	Branch       string `json:"branch,omitempty"`
}

// The fields of the tuple that Matrix cannot fill in-process, and why. The reason
// travels with the artifact so a reader does not mistake an absent value for an
// agreement.
const (
	// GitCommonDirNotDerived answers why the artifact carries no git common dir.
	GitCommonDirNotDerived = "Matrix does not interrogate the workspace repository, so git_common_dir is not derived in-process"
	// BranchNotDerived answers why the artifact carries no branch.
	BranchNotDerived = "Matrix does not interrogate the workspace repository, so branch is not derived in-process"
	// RealChildCwdNotDerived answers why a pre-dispatch artifact has no child cwd.
	RealChildCwdNotDerived = "the child directory is published by the transport that starts the child, after this point"
)

// Comparison is what the artifact reports for one run: both sides of the
// workspace contract, the fields that were not derived and why, and the
// disagreements that failed the run.
type Comparison struct {
	Requested  Observation
	Resolved   Observation
	NotDerived map[string]string
}

// ComparisonError reports the fields on which the requested and the resolved
// workspace disagree. The run is refused: a run that executes in a directory
// other than the one the caller asked for is a different run, and reporting the
// disagreement without failing it would make the artifact a witness instead of a
// guard.
type ComparisonError struct {
	Fields []string
}

func (e *ComparisonError) Error() string {
	return fmt.Sprintf("workspace_observation_mismatch: requested and resolved workspaces disagree on %s",
		strings.Join(e.Fields, ", "))
}

// CompareObservations is the guard of the workspace contract. A field an
// observation does not carry is unknown, not a disagreement — except for the
// child directory, which is a fact about where the process runs: a run whose
// resolved workspace and child directory disagree is refused rather than
// reported.
func CompareObservations(requested, resolved Observation) (Comparison, error) {
	comparison := Comparison{
		Requested:  requested,
		Resolved:   resolved,
		NotDerived: map[string]string{},
	}
	if strings.TrimSpace(resolved.GitCommonDir) == "" {
		comparison.NotDerived["git_common_dir"] = GitCommonDirNotDerived
	}
	if strings.TrimSpace(resolved.Branch) == "" {
		comparison.NotDerived["branch"] = BranchNotDerived
	}
	if strings.TrimSpace(resolved.RealChildCwd) == "" {
		comparison.NotDerived["real_child_cwd"] = RealChildCwdNotDerived
	}

	var mismatched []string
	if differs(requested.WorkspaceID, resolved.WorkspaceID, false) {
		mismatched = append(mismatched, "workspace_id")
	}
	if differs(requested.Path, resolved.Path, true) {
		mismatched = append(mismatched, "path")
	}
	if differs(resolved.Path, resolved.RealChildCwd, true) {
		mismatched = append(mismatched, "real_child_cwd")
	}
	if differs(requested.Path, resolved.RealChildCwd, true) {
		mismatched = append(mismatched, "real_child_cwd")
	}
	for _, pair := range []struct{ name, requested, resolved string }{
		{name: "git_common_dir", requested: requested.GitCommonDir, resolved: resolved.GitCommonDir},
		{name: "branch", requested: requested.Branch, resolved: resolved.Branch},
	} {
		if differs(pair.requested, pair.resolved, false) {
			mismatched = append(mismatched, pair.name)
		}
	}
	if len(mismatched) > 0 {
		return comparison, &ComparisonError{Fields: dedupe(mismatched)}
	}
	return comparison, nil
}

// differs reports whether two recorded hints disagree. An empty hint is unknown:
// the caller who did not name a field is not in conflict with the run that
// resolved it. Paths are compared cleaned, because two spellings of one directory
// are one directory.
func differs(left, right string, asPath bool) bool {
	left = strings.TrimSpace(left)
	right = strings.TrimSpace(right)
	if left == "" || right == "" {
		return false
	}
	if asPath {
		left = filepath.Clean(left)
		right = filepath.Clean(right)
	}
	return left != right
}

func dedupe(fields []string) []string {
	seen := make(map[string]struct{}, len(fields))
	unique := make([]string, 0, len(fields))
	for _, field := range fields {
		if _, found := seen[field]; found {
			continue
		}
		seen[field] = struct{}{}
		unique = append(unique, field)
	}
	return unique
}
