//go:build !linux

package childidentity

import "fmt"

// readIdentity reports that this platform offers no process identity to
// read. Saying so is the point: a doctor that silently omitted the child would
// let an operator read its absence as "nothing to report".
func readIdentity(int) (Identity, error) {
	return Identity{}, fmt.Errorf("process identity is read from /proc, which this platform does not provide")
}
