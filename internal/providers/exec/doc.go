// Package exec provides process execution capabilities for the Matrix runtime.
package exec

// Re-export NewProvider so callers use the package-level name
// regardless of which build variant is compiled.
// Each platform file (exec_unixlike.go, exec_windows.go) defines:
//   - type Provider struct{}
//   - func NewProvider() *Provider
// This file intentionally empty — nothing to add here.
