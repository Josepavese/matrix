package zedacp

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

// ----------------------------------------------------------------------------
// Adversarial JSON-RPC / wire suite
// ----------------------------------------------------------------------------

// fakeTransport is a scriptable in-memory transport for adversarial wire tests.
type fakeTransport struct {
	mu       sync.Mutex
	sent     [][]byte
	received chan []byte
	closed   bool
	sendErr  error
}

func newFakeTransport() *fakeTransport {
	return &fakeTransport{received: make(chan []byte, 64)}
}

func (t *fakeTransport) Send(_ context.Context, data []byte) error {
	t.mu.Lock()
	if t.closed {
		t.mu.Unlock()
		return context.Canceled
	}
	t.sent = append(t.sent, append([]byte(nil), data...))
	err := t.sendErr
	t.mu.Unlock()
	return err
}

func (t *fakeTransport) Receive(ctx context.Context) ([]byte, error) {
	select {
	case data := <-t.received:
		return data, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (t *fakeTransport) Close() error {
	t.mu.Lock()
	t.closed = true
	t.mu.Unlock()
	return nil
}

func (t *fakeTransport) sentPayloads() []string {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]string, 0, len(t.sent))
	for _, payload := range t.sent {
		out = append(out, string(payload))
	}
	return out
}

func (t *fakeTransport) push(testing *testing.T, payload string) {
	testing.Helper()
	select {
	case t.received <- []byte(payload):
	case <-time.After(time.Second):
		testing.Fatal("transport receive buffer full")
	}
}

// waitForSent polls until the client wrote something matching the predicate.
func waitForSent(t *testing.T, transport *fakeTransport, predicate func(string) bool, what string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		for _, payload := range transport.sentPayloads() {
			if predicate(payload) {
				return
			}
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s; sent=%v", what, transport.sentPayloads())
}

// TestClientIgnoresUncorrelatedAndMalformedResponses keeps the read loop alive
// when a peer sends garbage or answers an id nobody is waiting for.
func TestClientIgnoresUncorrelatedAndMalformedResponses(t *testing.T) {
	transport := newFakeTransport()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	client := NewClient(ctx, transport)
	defer client.Close()

	for _, payload := range []string{
		`not json at all`,
		`{}`,
		`{"jsonrpc":"2.0"}`,
		// Ids a peer could not legitimately produce for this client: the
		// client's own sequence must not be guessed, or the test would be
		// answering its own request rather than exercising the ignore paths.
		`{"jsonrpc":"2.0","id":987654,"result":{}}`,
		`{"jsonrpc":"2.0","id":"string-id","result":{}}`,
		`{"jsonrpc":"2.0","id":null,"result":{}}`,
		`{"jsonrpc":"2.0","id":987655}`,
		`{"jsonrpc":"2.0","id":987656,"error":{"code":-32000,"message":"boom"}}`,
		`[]`,
		`null`,
	} {
		transport.push(t, payload)
	}

	// The client must still be usable afterwards.
	done := make(chan error, 1)
	go func() {
		_, err := client.Initialize(ctx, InitializeRequest{ProtocolVersion: 1})
		done <- err
	}()
	waitForSent(t, transport, func(payload string) bool {
		return strings.Contains(payload, `"method":"initialize"`)
	}, "initialize request")

	// Answer with the id the client actually allocated.
	var requestID string
	for _, payload := range transport.sentPayloads() {
		var request struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
		}
		if json.Unmarshal([]byte(payload), &request) == nil && request.Method == "initialize" {
			requestID = string(request.ID)
			break
		}
	}
	if requestID == "" {
		t.Fatal("initialize request carried no id")
	}
	transport.push(t, `{"jsonrpc":"2.0","id":`+requestID+`,"result":{"protocolVersion":1}}`)
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("initialize failed after malformed traffic: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("initialize never completed")
	}
}

// TestClientSurvivesTransportSendFailure keeps an outbound failure from
// wedging the caller and leaking the pending entry.
func TestClientSurvivesTransportSendFailure(t *testing.T) {
	transport := newFakeTransport()
	transport.sendErr = context.DeadlineExceeded
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	client := NewClient(ctx, transport)
	defer client.Close()

	if _, err := client.Initialize(ctx, InitializeRequest{ProtocolVersion: 1}); err == nil {
		t.Fatal("a transport send failure must surface as an error")
	}
}

// TestClientRequestHonoursContextCancellation proves a hung peer cannot pin a
// caller forever.
func TestClientRequestHonoursContextCancellation(t *testing.T) {
	transport := newFakeTransport()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	client := NewClient(ctx, transport)
	defer client.Close()

	callCtx, callCancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer callCancel()
	start := time.Now()
	if _, err := client.Initialize(callCtx, InitializeRequest{ProtocolVersion: 1}); err == nil {
		t.Fatal("expected a context error when the peer never answers")
	}
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Fatalf("request ignored its context deadline: %s", elapsed)
	}
}

// TestClientNotificationDispatchIsSafeUnderConcurrency pushes notifications
// while observers are being registered and removed.
func TestClientNotificationDispatchIsSafeUnderConcurrency(t *testing.T) {
	transport := newFakeTransport()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	client := NewClient(ctx, transport)
	defer client.Close()

	observer := &countingObserver{}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			unsubscribe := client.registerObserver("session-1", observer)
			time.Sleep(time.Millisecond)
			unsubscribe()
		}()
	}
	for i := 0; i < 50; i++ {
		transport.push(t, `{"jsonrpc":"2.0","method":"session/update","params":{"sessionId":"session-1","update":{"sessionUpdate":"agent_message_chunk","content":{"type":"text","text":"x"}}}}`)
	}
	wg.Wait()
}

type countingObserver struct {
	mu      sync.Mutex
	updates int
}

func (o *countingObserver) OnUpdate(SessionNotification) {
	o.mu.Lock()
	o.updates++
	o.mu.Unlock()
}

// TestClientSessionNotificationWithoutSessionIDIsIgnored keeps a malformed
// notification from reaching observers with an empty key.
func TestClientSessionNotificationWithoutSessionIDIsIgnored(t *testing.T) {
	transport := newFakeTransport()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	client := NewClient(ctx, transport)
	defer client.Close()

	observer := &countingObserver{}
	client.registerObserver("", observer)
	transport.push(t, `{"jsonrpc":"2.0","method":"session/update","params":{"update":{"sessionUpdate":"agent_message_chunk"}}}`)
	transport.push(t, `{"jsonrpc":"2.0","method":"session/update","params":{}}`)
	transport.push(t, `{"jsonrpc":"2.0","method":"session/update"}`)
	time.Sleep(50 * time.Millisecond)
	observer.mu.Lock()
	defer observer.mu.Unlock()
	if observer.updates != 0 {
		t.Fatalf("notifications without a session must not be dispatched, got %d", observer.updates)
	}
}

// TestClientHandlesConcurrentRequestsWithoutIDCollisions keeps the id
// allocator honest under load.
func TestClientHandlesConcurrentRequestsWithoutIDCollisions(t *testing.T) {
	transport := newFakeTransport()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	client := NewClient(ctx, transport)
	defer client.Close()

	const callers = 25
	var wg sync.WaitGroup
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			callCtx, callCancel := context.WithTimeout(ctx, 300*time.Millisecond)
			defer callCancel()
			_, _ = client.NewSession(callCtx, NewSessionRequest{Cwd: "/tmp"})
		}()
	}

	// Answer every request the client emits, matching ids as they arrive.
	answered := map[string]bool{}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		for _, payload := range transport.sentPayloads() {
			var request struct {
				ID     json.RawMessage `json:"id"`
				Method string          `json:"method"`
			}
			if err := json.Unmarshal([]byte(payload), &request); err != nil || request.Method != "session/new" {
				continue
			}
			key := string(request.ID)
			if answered[key] {
				continue
			}
			answered[key] = true
			transport.push(t, `{"jsonrpc":"2.0","id":`+key+`,"result":{"sessionId":"s-`+key+`"}}`)
		}
		if len(answered) == callers {
			break
		}
		time.Sleep(time.Millisecond)
	}
	wg.Wait()
	if len(answered) != callers {
		t.Fatalf("expected %d distinct request ids, saw %d", callers, len(answered))
	}
}

// TestClientCloseIsIdempotentAndUnblocksRequests keeps shutdown from hanging.
func TestClientCloseIsIdempotentAndUnblocksRequests(t *testing.T) {
	transport := newFakeTransport()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	client := NewClient(ctx, transport)

	done := make(chan error, 1)
	go func() {
		callCtx, callCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer callCancel()
		_, err := client.Initialize(callCtx, InitializeRequest{ProtocolVersion: 1})
		done <- err
	}()
	waitForSent(t, transport, func(payload string) bool {
		return strings.Contains(payload, `"method":"initialize"`)
	}, "initialize request")

	if err := client.Close(); err != nil {
		t.Fatalf("close failed: %v", err)
	}
	_ = client.Close()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Close did not unblock an in-flight request")
	}
}

// TestRPCErrorMapping pins the error contract handlers rely on.
func TestRPCErrorMapping(t *testing.T) {
	var notFound *RPCError
	if err := NewMethodNotFoundError("elicitation/complete"); !errors.As(err, &notFound) {
		t.Fatalf("method-not-found must be an RPCError, got %T", err)
	}
	if notFound.Code != ErrCodeMethodNotFound {
		t.Fatalf("method-not-found must map to %d, got %d", ErrCodeMethodNotFound, notFound.Code)
	}
	custom := &RPCError{Code: -32602, Message: "invalid params"}
	if !strings.Contains(custom.Error(), "invalid params") {
		t.Fatalf("RPCError must expose its message: %q", custom.Error())
	}
}

// panickingHandler panics on every request, standing in for a buggy adapter.
type panickingHandler struct{}

func (panickingHandler) HandleRequest(context.Context, string, json.RawMessage) (interface{}, error) {
	panic("adapter bug")
}

// TestInboundRequestPanicIsContained keeps a panicking request handler from
// killing the process: the peer must receive an internal error and the client
// must stay usable.
func TestInboundRequestPanicIsContained(t *testing.T) {
	transport := newFakeTransport()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	client := NewClient(ctx, transport)
	client.SetRequestHandler(panickingHandler{})
	defer client.Close()

	transport.push(t, `{"jsonrpc":"2.0","id":11,"method":"fs/read_text_file","params":{}}`)
	waitForSent(t, transport, func(payload string) bool {
		return strings.Contains(payload, `"id":11`) && strings.Contains(payload, `"code":-32603`)
	}, "internal error response")

	// The client must still work afterwards.
	done := make(chan error, 1)
	go func() {
		_, err := client.Initialize(ctx, InitializeRequest{ProtocolVersion: 1})
		done <- err
	}()
	waitForSent(t, transport, func(payload string) bool {
		return strings.Contains(payload, `"method":"initialize"`)
	}, "initialize request")
}

// blockingObserver stays inside OnUpdate until released, standing in for a
// channel frontend that performs a network call (the Telegram notifier does).
type blockingObserver struct {
	entered chan struct{}
	release chan struct{}
}

func (o *blockingObserver) OnUpdate(SessionNotification) {
	select {
	case o.entered <- struct{}{}:
	default:
	}
	<-o.release
}

// TestSlowObserverDoesNotBlockResponseCorrelation is the regression guard for
// observer head-of-line blocking: while a frontend is busy, JSON-RPC responses
// must still be correlated, or every request stalls behind a chat API call.
func TestSlowObserverDoesNotBlockResponseCorrelation(t *testing.T) {
	transport := newFakeTransport()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	client := NewClient(ctx, transport)
	defer client.Close()

	observer := &blockingObserver{entered: make(chan struct{}, 1), release: make(chan struct{})}
	client.registerObserver("session-1", observer)

	transport.push(t, `{"jsonrpc":"2.0","method":"session/update","params":{"sessionId":"session-1","update":{"sessionUpdate":"agent_message_chunk","content":{"type":"text","text":"x"}}}}`)

	select {
	case <-observer.entered:
	case <-time.After(2 * time.Second):
		t.Fatal("the observer was never called")
	}

	// The observer is now blocked. A request must still complete.
	done := make(chan error, 1)
	go func() {
		_, err := client.Initialize(ctx, InitializeRequest{ProtocolVersion: 1})
		done <- err
	}()
	waitForSent(t, transport, func(payload string) bool {
		return strings.Contains(payload, `"method":"initialize"`)
	}, "initialize request")

	var requestID string
	for _, payload := range transport.sentPayloads() {
		var request struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
		}
		if json.Unmarshal([]byte(payload), &request) == nil && request.Method == "initialize" {
			requestID = string(request.ID)
			break
		}
	}
	transport.push(t, `{"jsonrpc":"2.0","id":`+requestID+`,"result":{"protocolVersion":1}}`)

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("initialize failed while an observer was blocked: %v", err)
		}
	case <-time.After(3 * time.Second):
		close(observer.release)
		t.Fatal("a blocked observer stalled JSON-RPC response correlation")
	}
	close(observer.release)
}

// TestObserverPanicDoesNotBreakDelivery keeps a broken frontend from silencing
// the session's updates or killing the process.
func TestObserverPanicDoesNotBreakDelivery(t *testing.T) {
	transport := newFakeTransport()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	client := NewClient(ctx, transport)
	defer client.Close()

	panicking := &panickingObserver{}
	recorder := &countingObserver{}
	client.registerObserver("session-1", panicking)
	client.registerObserver("session-1", recorder)

	for i := 0; i < 3; i++ {
		transport.push(t, `{"jsonrpc":"2.0","method":"session/update","params":{"sessionId":"session-1","update":{"sessionUpdate":"agent_message_chunk","content":{"type":"text","text":"x"}}}}`)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		recorder.mu.Lock()
		count := recorder.updates
		recorder.mu.Unlock()
		if count >= 3 {
			return
		}
		time.Sleep(time.Millisecond)
	}
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	t.Fatalf("a panicking observer stopped delivery, got %d updates", recorder.updates)
}

type panickingObserver struct{}

func (panickingObserver) OnUpdate(SessionNotification) { panic("frontend bug") }
