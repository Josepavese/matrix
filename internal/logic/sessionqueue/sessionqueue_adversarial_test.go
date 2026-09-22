package sessionqueue

import (
	"errors"
	"fmt"
	"sync"
	"testing"
)

// TestOrderedMergeFlushesInSequence is the whole reason this type exists: a
// result that arrives early must wait for the gap in front of it, and the
// callback order must follow sequence numbers, never arrival order.
func TestOrderedMergeFlushesInSequence(t *testing.T) {
	var (
		mu      sync.Mutex
		flushed []int
	)
	merge := New(func(seq int, _ RouteResult) {
		mu.Lock()
		defer mu.Unlock()
		flushed = append(flushed, seq)
	})

	seq0 := merge.NextSeq()
	seq1 := merge.NextSeq()
	seq2 := merge.NextSeq()
	if seq0 != 0 || seq1 != 1 || seq2 != 2 {
		t.Fatalf("NextSeq must be monotonic from zero, got %d %d %d", seq0, seq1, seq2)
	}

	// Submit out of order: the last result first.
	merge.Submit(seq2, RouteResult{Content: "third"})
	mu.Lock()
	if len(flushed) != 0 {
		mu.Unlock()
		t.Fatalf("a result behind a gap must not be flushed, got %v", flushed)
	}
	mu.Unlock()

	merge.Submit(seq0, RouteResult{Content: "first"})
	mu.Lock()
	if len(flushed) != 1 || flushed[0] != 0 {
		mu.Unlock()
		t.Fatalf("only the contiguous prefix may flush, got %v", flushed)
	}
	mu.Unlock()

	// Closing the gap must release both the gap filler and the waiting result,
	// in order.
	merge.Submit(seq1, RouteResult{Content: "second"})
	mu.Lock()
	defer mu.Unlock()
	want := []int{0, 1, 2}
	if len(flushed) != len(want) {
		t.Fatalf("flushed %v, want %v", flushed, want)
	}
	for i := range want {
		if flushed[i] != want[i] {
			t.Fatalf("flushed %v, want %v", flushed, want)
		}
	}
}

// TestOrderedMergeCarriesTheResultThrough makes sure ordering does not cost the
// payload: the callback must receive the result submitted for that sequence.
func TestOrderedMergeCarriesTheResultThrough(t *testing.T) {
	results := map[int]RouteResult{}
	merge := New(func(seq int, result RouteResult) { results[seq] = result })

	first := merge.NextSeq()
	second := merge.NextSeq()
	merge.Submit(second, RouteResult{LogicalSessionID: "s", Content: "second", AgentSessionID: "agent-2"})
	merge.Submit(first, RouteResult{LogicalSessionID: "s", Content: "first", AgentSessionID: "agent-1", Err: errors.New("upstream")})

	if results[first].Content != "first" || results[first].AgentSessionID != "agent-1" || results[first].Err == nil {
		t.Fatalf("the first result arrived mangled: %+v", results[first])
	}
	if results[second].Content != "second" || results[second].AgentSessionID != "agent-2" {
		t.Fatalf("the second result arrived mangled: %+v", results[second])
	}
}

// TestOrderedMergeWithoutCallbackStillDrains keeps a nil callback from turning
// into a panic or a blocked merge.
func TestOrderedMergeWithoutCallbackStillDrains(t *testing.T) {
	merge := New(nil)
	seq := merge.NextSeq()
	merge.Submit(seq, RouteResult{Content: "x"})
	// Nothing to assert but the absence of a panic; a second submit must still
	// advance, which proves the first was consumed.
	next := merge.NextSeq()
	merge.Submit(next, RouteResult{Content: "y"})
}

// TestOrderedMergeConcurrentSubmitsStayOrdered submits from many goroutines at
// once: the flushed sequence must be exactly 0..n-1 in order, with no duplicate
// and no gap, under the race detector.
func TestOrderedMergeConcurrentSubmitsStayOrdered(t *testing.T) {
	const workers = 8
	const perWorker = 20

	var (
		mu      sync.Mutex
		flushed []int
	)
	merge := New(func(seq int, _ RouteResult) {
		mu.Lock()
		defer mu.Unlock()
		flushed = append(flushed, seq)
	})

	sequences := make([]int, 0, workers*perWorker)
	var seqMu sync.Mutex
	var wg sync.WaitGroup
	for worker := 0; worker < workers; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < perWorker; i++ {
				seqMu.Lock()
				seq := merge.NextSeq()
				sequences = append(sequences, seq)
				seqMu.Unlock()
				merge.Submit(seq, RouteResult{Content: fmt.Sprintf("r%d", seq)})
			}
		}()
	}
	wg.Wait()

	mu.Lock()
	defer mu.Unlock()
	if len(flushed) != workers*perWorker {
		t.Fatalf("flushed %d results, want %d (a gap or a duplicate was swallowed)", len(flushed), workers*perWorker)
	}
	for i, seq := range flushed {
		if seq != i {
			t.Fatalf("flush order broke at position %d: got sequence %d", i, seq)
		}
	}
	seen := map[int]bool{}
	for _, seq := range sequences {
		if seen[seq] {
			t.Fatalf("NextSeq handed out sequence %d twice", seq)
		}
		seen[seq] = true
	}
}
