// Package exec provides process execution capabilities for the Matrix runtime.
//
// # Two environment policies, one per trust
//
// The commands this package runs inherit the daemon's environment
// (os.Environ() plus the spec's own entries), and that is a decision, not an
// omission. An environment allowlist belongs where the child is NOT chosen by
// the operator or is not inside their trust: the agent process, which is driven
// by a model and executes actions it generated, and the caller's validator, which
// is code handed in through a request. A command the operator wrote themselves,
// of short duration - an installer, a toolchain - lives in the trust domain of
// whoever wrote it and expects their environment: taking it away would change
// what a command runner is for.
//
// So the two policies are deliberately different, and the difference is which
// side chooses the program: internal/logic/childenv holds the allowlist and the
// rationale for the children Matrix does not trust.
package exec

// Re-export NewProvider so callers use the package-level name
// regardless of which build variant is compiled.
// Each platform file (exec_unixlike.go, exec_windows.go) defines:
//   - type Provider struct{}
//   - func NewProvider() *Provider
// This file intentionally empty — nothing to add here.
