package admission

import (
	"math"
	"sync"
	"testing"

	"github.com/Josepavese/matrix/internal/middleware"
)

type fixedCapacity struct{ free uint64 }

func (c fixedCapacity) Observe(string) (middleware.CapacitySnapshot, error) {
	return middleware.CapacitySnapshot{Available: true, DiskFreeBytes: &c.free, VolumeID: "same-volume"}, nil
}

func TestConcurrentWorkspacesShareVolumeReservationsAndReleaseExactlyOnce(t *testing.T) {
	m := New(fixedCapacity{free: 100}, nil)
	var wg sync.WaitGroup
	releases := make(chan func(), 20)
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			release, err := m.Acquire("different-workspace", middleware.CapacityRequest{ReserveDiskBytes: 60})
			if err == nil {
				releases <- release
			}
		}()
	}
	wg.Wait()
	close(releases)
	if len(releases) != 1 || m.State().Active != 1 {
		t.Fatal("overcommitted the shared volume", m.State())
	}
	for release := range releases {
		release()
		release()
	}
	if m.State().Active != 0 || len(m.State().ReservedDisk) != 0 {
		t.Fatal("lease was not released exactly once", m.State())
	}
	release, err := m.Acquire("workspace", middleware.CapacityRequest{ReserveDiskBytes: 60})
	if err != nil {
		t.Fatal("released capacity remained unavailable", err)
	}
	release()
}

func TestLimitsUnknownCapacityAndUintOverflowFailClosed(t *testing.T) {
	m := New(fixedCapacity{free: 100}, func() (Limits, error) { return Limits{MaxConcurrent: 1, MinDiskFreeBytes: 20}, nil })
	release, err := m.Acquire("workspace", middleware.CapacityRequest{ReserveDiskBytes: 80})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.Acquire("another", middleware.CapacityRequest{}); err == nil {
		t.Fatal("concurrency limit ignored")
	}
	release()
	if _, err := m.Acquire("workspace", middleware.CapacityRequest{ReserveDiskBytes: math.MaxUint64}); err == nil {
		t.Fatal("overflow accepted")
	}
	if m.State().Active != 0 {
		t.Fatal("refusal acquired a lease")
	}
	unknown := New(nil, nil)
	if _, err := unknown.Acquire("", middleware.CapacityRequest{ReserveDiskBytes: 1}); err == nil {
		t.Fatal("unknown capacity accepted")
	}
	if _, err := New(nil, func() (Limits, error) { return Limits{MaxConcurrent: -1}, nil }).Acquire("", middleware.CapacityRequest{}); err == nil {
		t.Fatal("invalid limits silently disabled")
	}
}
