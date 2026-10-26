//go:build !linux && !darwin

package agentstore

import "os"

// fileOwnerUID reports no owner on platforms without unix ownership; callers
// then treat an owner-writable store as writable (fail closed).
func fileOwnerUID(os.FileInfo) (uint32, bool) {
	return 0, false
}

func currentEUID() uint32 {
	return 0
}
