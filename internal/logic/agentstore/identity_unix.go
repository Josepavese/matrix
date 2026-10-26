//go:build linux || darwin

package agentstore

import (
	"os"
	"syscall"
)

// fileOwnerUID reads the owning user id of info. The filesystem is asked
// directly instead of trusting a process-global guess.
func fileOwnerUID(info os.FileInfo) (uint32, bool) {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, false
	}
	return stat.Uid, true
}

func currentEUID() uint32 {
	return uint32(os.Geteuid())
}
