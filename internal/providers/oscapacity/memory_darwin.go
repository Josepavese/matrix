//go:build darwin

package oscapacity

import (
	"context"
	"golang.org/x/sys/unix"
	"os/exec"
	"time"
)

func observeMemory() (*uint64, *uint64, string) {
	total, err := unix.SysctlUint64("hw.memsize")
	if err != nil {
		return nil, nil, "unavailable"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "/usr/bin/vm_stat")
	cmd.Env = []string{"LC_ALL=C"}
	output, err := cmd.Output()
	if err != nil {
		return &total, nil, "darwin:hw.memsize_free_observation_failed"
	}
	free, err := parseVMStat(output)
	if err != nil {
		return &total, nil, "darwin:hw.memsize_free_observation_failed"
	}
	return &total, &free, "darwin:hw.memsize_vm_stat_free_excludes_cache_and_speculative"
}
