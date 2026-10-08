package oscapacity

import (
	"fmt"
	"strconv"
	"strings"
)

// Apple's vm_stat prints the native kernel page size and Pages free excluding
// speculative pages. Do not assume 4 KiB on ARM or call cached pages "free".
func parseVMStat(raw []byte) (uint64, error) {
	if len(raw) > 8192 {
		return 0, fmt.Errorf("vm_stat output exceeds limit")
	}
	var pageSize, freePages uint64
	var foundFree bool
	for _, line := range strings.Split(string(raw), "\n") {
		if _, tail, ok := strings.Cut(line, "(page size of "); ok {
			fields := strings.Fields(tail)
			if len(fields) < 2 {
				return 0, fmt.Errorf("invalid vm_stat page size")
			}
			pageSize, _ = strconv.ParseUint(fields[0], 10, 64)
		}
		if tail, ok := strings.CutPrefix(line, "Pages free:"); ok {
			var err error
			freePages, err = strconv.ParseUint(strings.TrimSuffix(strings.TrimSpace(tail), "."), 10, 64)
			if err != nil {
				return 0, err
			}
			foundFree = true
		}
	}
	if !foundFree || pageSize == 0 || pageSize > 65536 {
		return 0, fmt.Errorf("vm_stat free memory unavailable")
	}
	if freePages > ^uint64(0)/pageSize {
		return 0, fmt.Errorf("vm_stat quantity overflow")
	}
	return freePages * pageSize, nil
}
