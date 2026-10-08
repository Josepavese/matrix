package runtrace

import (
	"sync"
	"testing"

	"github.com/Josepavese/matrix/internal/logic/memstore"
)

func TestLiveMetadataCannotResurrectCancelledRun(t *testing.T) {
	s := NewStore(memstore.New())
	run, _, err := s.Start(Run{AgentID: "agent"})
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	wg.Go(func() {
		if _, err := s.Cancel(run.ID, "operator"); err != nil {
			t.Error(err)
		}
	})
	for range 20 {
		wg.Go(func() {
			if err := s.UpdateRun(run.ID, func(run *Run) { run.RemoteSessionID = "remote" }); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	stored, _, err := s.LoadRun(run.ID)
	if err != nil || stored.Status != StatusCancelled || stored.RemoteSessionID != "remote" {
		t.Fatalf("live callback overwrote terminal state: %+v %v", stored, err)
	}
	items, _, err := s.LoadNotificationsAfter(0, 100, nil)
	if err != nil || len(items) != 1 {
		t.Fatalf("cancel delivery was lost or repeated: %+v %v", items, err)
	}
}

func TestUpdateMissingRunIsRejected(t *testing.T) {
	s := NewStore(memstore.New())
	if err := s.UpdateRun("missing", func(*Run) { t.Error("missing run passed to callback") }); err == nil {
		t.Fatal("created metadata without a run")
	}
}
