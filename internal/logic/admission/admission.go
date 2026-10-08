// Package admission coordinates explicit runtime task leases. It does not
// reserve physical filesystem blocks or account for work on another runtime.
package admission

import (
	"fmt"
	"sync"

	"github.com/Josepavese/matrix/internal/middleware"
)

type Limits struct {
	MaxConcurrent    int
	MinDiskFreeBytes uint64
}

type Refusal struct{ Code string }

func (e *Refusal) Error() string { return e.Code }

func (e *Refusal) FailureCode() string { return e.Code }

type State struct {
	Scope        string            `json:"scope"`
	Active       int               `json:"active"`
	ReservedDisk map[string]uint64 `json:"reserved_disk_bytes_by_volume"`
}

type Manager struct {
	mu       sync.Mutex
	observer middleware.Capacity
	limits   func() (Limits, error)
	active   int
	reserved map[string]uint64
}

func New(observer middleware.Capacity, limits func() (Limits, error)) *Manager {
	return &Manager{observer: observer, limits: limits, reserved: map[string]uint64{}}
}

// Acquire refuses before provider dispatch. A release is returned only after
// the request owns a lease; calling it repeatedly is harmless.
func (m *Manager) Acquire(path string, request middleware.CapacityRequest) (func(), error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	limits, err := m.readLimits()
	if err != nil {
		return nil, err
	}
	if limits.MaxConcurrent > 0 && m.active >= limits.MaxConcurrent {
		return nil, &Refusal{Code: "capacity_concurrency_exhausted"}
	}
	volume, err := m.reserveDisk(path, request, limits)
	if err != nil {
		return nil, err
	}
	m.active++
	var once sync.Once
	return func() { once.Do(func() { m.release(volume, request.ReserveDiskBytes) }) }, nil
}

func (m *Manager) readLimits() (Limits, error) {
	if m.limits == nil {
		return Limits{}, nil
	}
	limits, err := m.limits()
	if err != nil || limits.MaxConcurrent < 0 {
		return Limits{}, &Refusal{Code: "capacity_configuration_invalid"}
	}
	return limits, nil
}

func (m *Manager) reserveDisk(path string, request middleware.CapacityRequest, limits Limits) (string, error) {
	minimum := max(request.MinDiskFreeBytes, limits.MinDiskFreeBytes)
	if minimum == 0 && request.ReserveDiskBytes == 0 {
		return "", nil
	}
	if m.observer == nil {
		return "", &Refusal{Code: "capacity_observation_unavailable"}
	}
	snapshot, err := m.observer.Observe(path)
	if err != nil || !snapshot.Available || snapshot.DiskFreeBytes == nil || snapshot.VolumeID == "" {
		return "", &Refusal{Code: "capacity_observation_unavailable"}
	}
	free, reserved := *snapshot.DiskFreeBytes, m.reserved[snapshot.VolumeID]
	if reserved > free || minimum > free-reserved || request.ReserveDiskBytes > free-reserved-minimum {
		return "", &Refusal{Code: "capacity_disk_exhausted"}
	}
	m.reserved[snapshot.VolumeID] += request.ReserveDiskBytes
	return snapshot.VolumeID, nil
}

func (m *Manager) release(volume string, disk uint64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.active--
	if volume != "" {
		m.reserved[volume] -= disk
		if m.reserved[volume] == 0 {
			delete(m.reserved, volume)
		}
	}
}

func (m *Manager) State() State {
	m.mu.Lock()
	defer m.mu.Unlock()
	state := State{Scope: "this_runtime_active_routes", Active: m.active, ReservedDisk: map[string]uint64{}}
	for volume, count := range m.reserved {
		state.ReservedDisk[volume] = count
	}
	return state
}

// ParseLimits rejects invalid configuration instead of silently disabling it.
func ParseLimits(concurrent int, free uint64) (Limits, error) {
	if concurrent < 0 {
		return Limits{}, fmt.Errorf("capacity.max_concurrent must be nonnegative")
	}
	return Limits{MaxConcurrent: concurrent, MinDiskFreeBytes: free}, nil
}
