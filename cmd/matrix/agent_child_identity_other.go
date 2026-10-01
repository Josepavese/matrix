//go:build !linux

package main

import "fmt"

// readChildIdentity reports that this platform offers no process identity to
// read. Saying so is the point: a doctor that silently omitted the child would
// let an operator read its absence as "nothing to report".
func readChildIdentity(int) (childIdentity, error) {
	return childIdentity{}, fmt.Errorf("process identity is read from /proc, which this platform does not provide")
}
