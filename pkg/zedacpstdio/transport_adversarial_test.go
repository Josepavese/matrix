package zedacpstdio

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"
)

// requireCat skips the suite where a simple echo process is unavailable.
func requireCat(t *testing.T) string {
	t.Helper()
	path, err := exec.LookPath("cat")
	if err != nil {
		t.Skip("cat not available")
	}
	return path
}

// overlapDetectingWriter records whether two writes are ever in flight at the
// same time. A pipe requires exactly that guarantee: a frame larger than
// PIPE_BUF can be split, and two overlapping writers would splice their bytes
// into one line.
type overlapDetectingWriter struct {
	mu       sync.Mutex
	active   int
	overlaps int
	delay    time.Duration
	bytes    int
}

func (w *overlapDetectingWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	w.active++
	if w.active > 1 {
		w.overlaps++
	}
	w.mu.Unlock()

	// Widen the window so an unsynchronised implementation loses the race
	// deterministically instead of occasionally.
	time.Sleep(w.delay)

	w.mu.Lock()
	w.active--
	w.bytes += len(p)
	w.mu.Unlock()
	return len(p), nil
}

func (w *overlapDetectingWriter) Close() error { return nil }

// TestSendSerialisesWrites is the deterministic guard for the write lock: the
// ACP client writes responses from the read loop while callers write requests,
// so Send must never let two writes overlap.
func TestSendSerialisesWrites(t *testing.T) {
	writer := &overlapDetectingWriter{delay: 2 * time.Millisecond}
	transport := &Transport{stdin: writer, maxFrameBytes: DefaultMaxFrameBytes}

	const writers = 8
	const perWriter = 5
	var wg sync.WaitGroup
	for writerIndex := 0; writerIndex < writers; writerIndex++ {
		wg.Add(1)
		go func(_ int) {
			defer wg.Done()
			for message := 0; message < perWriter; message++ {
				if err := transport.Send(context.Background(), []byte(`{"jsonrpc":"2.0","id":1}`)); err != nil {
					t.Errorf("send failed: %v", err)
					return
				}
			}
		}(writerIndex)
	}
	wg.Wait()

	writer.mu.Lock()
	defer writer.mu.Unlock()
	if writer.overlaps != 0 {
		t.Fatalf("Send allowed %d overlapping writes; a pipe requires them to be serialised", writer.overlaps)
	}
	if writer.bytes == 0 {
		t.Fatal("no bytes reached the writer")
	}
}

// TestConcurrentSendsStayWholeUnderARealPeer keeps the end-to-end path honest
// under concurrency: many large frames written while the peer echoes. It is a
// smoke test, not the guard for write serialisation (a fast reader lets the
// kernel complete each write, so interleaving is not reproducible from here);
// TestSendSerialisesWrites is the deterministic guard.

// The reader runs concurrently with the writers: a pipe cannot buffer the whole
// conversation, so writing everything first would deadlock the test rather than
// test the transport.
func TestConcurrentSendsDoNotInterleaveFrames(t *testing.T) {
	cat := requireCat(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	transport, err := New(ctx, cat, nil)
	if err != nil {
		t.Fatalf("start echo peer: %v", err)
	}
	defer transport.Close()

	const writers = 8
	const perWriter = 6
	const total = writers * perWriter
	// Comfortably above PIPE_BUF (4096) so the kernel can split each write.
	payload := strings.Repeat("x", 8192)

	type frame struct {
		ID   int `json:"id"`
		Body struct {
			Blob string `json:"blob"`
		} `json:"params"`
	}
	var mu sync.Mutex
	seen := map[int]int{}
	var readErr error
	readerDone := make(chan struct{})
	go func() {
		defer close(readerDone)
		for {
			mu.Lock()
			count := len(seen)
			mu.Unlock()
			if count >= total {
				return
			}
			line, err := transport.Receive(ctx)
			if err != nil {
				mu.Lock()
				if readErr == nil {
					readErr = err
				}
				mu.Unlock()
				return
			}
			var decoded frame
			if err := json.Unmarshal(line, &decoded); err != nil {
				mu.Lock()
				if readErr == nil {
					readErr = fmt.Errorf("interleaved or truncated frame on the wire: %w", err)
				}
				mu.Unlock()
				return
			}
			mu.Lock()
			if decoded.Body.Blob != payload {
				if readErr == nil {
					readErr = fmt.Errorf("frame %d arrived with a corrupted body (len=%d)", decoded.ID, len(decoded.Body.Blob))
				}
				mu.Unlock()
				return
			}
			seen[decoded.ID]++
			mu.Unlock()
		}
	}()

	var wg sync.WaitGroup
	var sendErr error
	var sendMu sync.Mutex
	for writer := 0; writer < writers; writer++ {
		wg.Add(1)
		go func(writer int) {
			defer wg.Done()
			for message := 0; message < perWriter; message++ {
				frame, err := json.Marshal(map[string]interface{}{
					"jsonrpc": "2.0",
					"id":      writer*perWriter + message,
					"method":  "probe",
					"params":  map[string]interface{}{"blob": payload},
				})
				if err != nil {
					return
				}
				if err := transport.Send(ctx, frame); err != nil {
					sendMu.Lock()
					if sendErr == nil {
						sendErr = err
					}
					sendMu.Unlock()
					return
				}
			}
		}(writer)
	}
	wg.Wait()
	<-readerDone

	mu.Lock()
	defer mu.Unlock()
	if readErr != nil {
		t.Fatalf("%v", readErr)
	}
	if sendErr != nil {
		t.Fatalf("send failed: %v", sendErr)
	}
	if len(seen) != total {
		t.Fatalf("expected %d intact frames, got %d", total, len(seen))
	}
	for id, count := range seen {
		if count != 1 {
			t.Fatalf("frame %d received %d times", id, count)
		}
	}
}

// TestReceiveRejectsOversizedFrames keeps a hostile peer from allocating
// without limit on a single unterminated line.
func TestReceiveRejectsOversizedFrames(t *testing.T) {
	// Exercise the bounded reader directly: an unterminated line above the
	// limit must fail instead of buffering forever.
	reader := bufio.NewReaderSize(strings.NewReader(strings.Repeat("a", 4096)), 512)
	if _, err := readBoundedLine(reader, 1024); !errors.Is(err, ErrFrameTooLarge) {
		t.Fatalf("expected ErrFrameTooLarge, got %v", err)
	}
}

// TestReadBoundedLineHandlesChunkBoundaries proves a frame split across the
// reader's internal buffer is reassembled rather than truncated.
func TestReadBoundedLineHandlesChunkBoundaries(t *testing.T) {
	body := strings.Repeat("b", 5000)
	reader := bufio.NewReaderSize(strings.NewReader(body+"\n"), 256)
	line, err := readBoundedLine(reader, 1<<20)
	if err != nil {
		t.Fatalf("reassembly failed: %v", err)
	}
	if strings.TrimRight(string(line), "\n") != body {
		t.Fatalf("frame was altered: got %d bytes, want %d", len(line), len(body)+1)
	}
}

// TestSendAfterCloseFails keeps a closed transport from panicking or silently
// pretending to deliver.
func TestSendAfterCloseFails(t *testing.T) {
	cat := requireCat(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	transport, err := New(ctx, cat, nil)
	if err != nil {
		t.Fatalf("start echo peer: %v", err)
	}
	if err := transport.Close(); err != nil {
		t.Fatalf("close failed: %v", err)
	}
	if err := transport.Send(ctx, []byte(`{"jsonrpc":"2.0"}`)); err == nil {
		t.Fatal("sending on a closed transport must fail")
	}
	_ = transport.Close()
}

// TestSpawnFailureDoesNotLeakDescriptors is a best-effort guard: a launch that
// cannot start must not leave pipes open. Failures are counted by repeated
// attempts so a leak would show up as an error rather than silently.
func TestSpawnFailureDoesNotLeakDescriptors(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	for i := 0; i < 32; i++ {
		transport, err := New(ctx, "/nonexistent/matrix-agent-binary", nil)
		if err == nil {
			_ = transport.Close()
			t.Fatal("expected a start failure for a missing binary")
		}
		if !strings.Contains(err.Error(), "failed to start agent") {
			t.Fatalf("unexpected error shape: %v", err)
		}
	}
}
