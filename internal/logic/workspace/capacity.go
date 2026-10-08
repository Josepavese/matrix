package workspace

import "github.com/Josepavese/matrix/internal/middleware"

// ObserveCapacity returns live, non-persisted capacity alongside metadata.
// Unknown capacity is explicit; it never claims an inaccessible remote path
// has zero bytes available, nor looks at the runtime's root disk instead.
func ObserveCapacity(path string, observer middleware.Capacity) middleware.CapacitySnapshot {
	if observer == nil {
		return middleware.CapacitySnapshot{Reason: "observer_unavailable", Scope: "runtime_host_workspace"}
	}
	snapshot, err := observer.Observe(path)
	if err != nil && snapshot.Reason == "" {
		snapshot.Reason = "observation_failed"
	}
	return snapshot
}
