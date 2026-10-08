package middleware

import "time"

// CapacitySnapshot reports an observation of the runtime host, never a remote
// provider account balance or a promise that future allocations will succeed.
type CapacitySnapshot struct {
	Available        bool      `json:"available"`
	Reason           string    `json:"reason,omitempty"`
	Path             string    `json:"path,omitempty"`
	VolumeID         string    `json:"volume_id,omitempty"`
	DiskFreeBytes    *uint64   `json:"disk_free_bytes,omitempty"`
	DiskTotalBytes   *uint64   `json:"disk_total_bytes,omitempty"`
	LogicalCPUs      int       `json:"logical_cpus"`
	MemoryTotalBytes *uint64   `json:"memory_total_bytes,omitempty"`
	MemoryFreeBytes  *uint64   `json:"memory_free_bytes,omitempty"`
	MemorySource     string    `json:"memory_source,omitempty"`
	Source           string    `json:"source"`
	MeasuredAt       time.Time `json:"measured_at"`
	Scope            string    `json:"scope"`
}

// Capacity is the PAL seam for workspace filesystem and host observations.
type Capacity interface {
	Observe(path string) (CapacitySnapshot, error)
}

// CapacityRequest is an explicit admission requirement, not an allocation.
type CapacityRequest struct {
	MinDiskFreeBytes uint64 `json:"min_disk_free_bytes,omitempty"`
	ReserveDiskBytes uint64 `json:"reserve_disk_bytes,omitempty"`
}
