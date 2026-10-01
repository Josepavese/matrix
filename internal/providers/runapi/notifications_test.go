package runapi

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Josepavese/matrix/internal/logic/memstore"
	"github.com/Josepavese/matrix/internal/logic/runtrace"
	"github.com/Josepavese/matrix/internal/middleware"
)

func TestLocalNotificationsSSEWakesOnTerminalEventWithoutContent(t *testing.T) {
	server := NewServer(&runTestRouter{})
	httpServer := httptest.NewServer(http.HandlerFunc(server.HandleLocalNotifications))
	defer httpServer.Close()
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(httpServer.URL + "?stream=sse&run_id=run-watched")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	reader := bufio.NewReader(resp.Body)
	if _, err := reader.ReadString('\n'); err != nil {
		t.Fatal(err)
	}
	if _, err := server.Store().AppendEvent(runtrace.Event{RunID: "run-other", Kind: "run.completed", Message: "private"}); err != nil {
		t.Fatal(err)
	}
	if _, err := server.Store().AppendEvent(runtrace.Event{RunID: "run-watched", Kind: "run.failed", Message: "private failure"}); err != nil {
		t.Fatal(err)
	}
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		var notification runtrace.Notification
		if err := json.Unmarshal([]byte(strings.TrimSpace(strings.TrimPrefix(line, "data: "))), &notification); err != nil {
			t.Fatal(err)
		}
		if notification.RunID != "run-watched" || notification.Kind != "run.failed" || strings.Contains(line, "private") {
			t.Fatalf("wrong wakeup: %+v raw=%s", notification, line)
		}
		break
	}
}

// ----------------------------------------------------------------------------
// EP-05.B: reconnect and restart proof for the local wakeup channel.
// ----------------------------------------------------------------------------

type sseItem struct {
	id   string
	kind string
	data string
}

type sseStream struct {
	reader *bufio.Reader
	body   io.Closer
}

func openSSEStream(t *testing.T, url string) *sseStream {
	t.Helper()
	resp, err := (&http.Client{}).Get(url)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		t.Fatalf("notification stream status %d", resp.StatusCode)
	}
	return &sseStream{reader: bufio.NewReader(resp.Body), body: resp.Body}
}

func (s *sseStream) close() { _ = s.body.Close() }

// next returns the next complete SSE item, or ok=false when none arrived within
// wait. The read runs off the test goroutine so a silent stream cannot hang the
// test: a supervisor that receives nothing must fail an assertion, not a timeout.
func (s *sseStream) next(wait time.Duration) (sseItem, bool) {
	type readResult struct {
		item sseItem
		ok   bool
	}
	results := make(chan readResult, 1)
	go func() {
		item := sseItem{}
		for {
			line, err := s.reader.ReadString('\n')
			if err != nil {
				results <- readResult{}
				return
			}
			line = strings.TrimRight(line, "\r\n")
			switch {
			case line == "" && item.data != "":
				results <- readResult{item: item, ok: true}
				return
			case strings.HasPrefix(line, "id: "):
				item.id = strings.TrimPrefix(line, "id: ")
			case strings.HasPrefix(line, "event: "):
				item.kind = strings.TrimPrefix(line, "event: ")
			case strings.HasPrefix(line, "data: "):
				item.data = strings.TrimPrefix(line, "data: ")
			}
		}
	}()
	select {
	case result := <-results:
		return result.item, result.ok
	case <-time.After(wait):
		return sseItem{}, false
	}
}

// TestReconnectAfterRecipientCrashYieldsOneLogicalOutcome is the EP-05.B proof.
// A supervisor is interrupted after it received the terminal wakeup and before it
// delivered it; the daemon is restarted in between and the recipient reopens the
// stream from the cursor it had persisted. What the supervisor ends up with must
// be exactly one logical outcome: the platform may hand the same wakeup over
// twice (at-least-once) but it must never invent a second outcome, never renumber
// the one it has, and never put turn content on the wire. The stream is the only
// thing the supervisor reads: it never polls and never asks for a transcript.
func TestReconnectAfterRecipientCrashYieldsOneLogicalOutcome(t *testing.T) {
	storage := memstore.New()
	const privateContent = "contenuto privato del turno che non deve viaggiare"

	// Daemon 1 persists the run and its terminal state, then stops before the
	// wakeup is persisted: this is the crash window the reconciler exists for.
	first := runtrace.NewStore(storage)
	run, _, err := first.Start(runtrace.Run{AgentID: "opencode", ChannelID: "halfpocket.runs", Protocol: "acp"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := first.AppendEvent(runtrace.Event{RunID: run.ID, Kind: "agent.message", Message: privateContent}); err != nil {
		t.Fatal(err)
	}
	run.Status = runtrace.StatusCompleted
	run.StopReason = "end_turn"
	run.CompletedAt = time.Now().UTC()
	if err := first.SaveRun(run); err != nil {
		t.Fatal(err)
	}
	if items, _, err := first.LoadNotificationsAfter(0, 100, nil); err != nil || len(items) != 0 {
		t.Fatalf("the crash window must have no wakeup yet: items=%+v err=%v", items, err)
	}

	// Restart. The reconciler restores the missing wakeup exactly once.
	daemon := NewServer(&runTestRouter{}).WithTraceStorage(storage)
	reconciled, err := daemon.Store().ReconcileTerminalNotifications()
	if err != nil || reconciled != 1 {
		t.Fatalf("reconcile after restart: count=%d err=%v", reconciled, err)
	}
	stored, _, err := daemon.Store().LoadNotificationsAfter(0, 100, map[string]struct{}{run.ID: {}})
	if err != nil || len(stored) != 1 || stored[0].Kind != "run.completed" {
		t.Fatalf("a restarted daemon must hold exactly one outcome: %+v err=%v", stored, err)
	}
	sequence := strconv.FormatUint(stored[0].Sequence, 10)

	httpServer := httptest.NewServer(http.HandlerFunc(daemon.HandleLocalNotifications))
	defer httpServer.Close()
	watchURL := httpServer.URL + "?stream=sse&run_id=" + run.ID

	// The recipient receives the outcome, then dies before delivering it, so its
	// durable cursor still points at the wakeup it never acknowledged.
	interrupted := openSSEStream(t, watchURL)
	firstDelivery, ok := interrupted.next(3 * time.Second)
	if !ok {
		interrupted.close()
		t.Fatal("the recipient never received the wakeup it was waiting for")
	}
	interrupted.close()
	if firstDelivery.id != sequence || firstDelivery.kind != "run.completed" {
		t.Fatalf("wakeup identity changed: id=%q kind=%q want id=%q kind=run.completed", firstDelivery.id, firstDelivery.kind, sequence)
	}
	if strings.Contains(firstDelivery.data, privateContent) {
		t.Fatalf("turn content reached the wakeup payload: %s", firstDelivery.data)
	}
	observed := map[string]int{run.ID + "|" + firstDelivery.kind: 1}

	// Reopening from the cursor it persisted before delivery hands the same
	// logical outcome over again, with the same identity.
	reopened := openSSEStream(t, watchURL+"&after=0")
	redelivered, ok := reopened.next(3 * time.Second)
	if !ok {
		reopened.close()
		t.Fatal("reopening from the persisted cursor lost the outcome")
	}
	reopened.close()
	if redelivered.id != firstDelivery.id || redelivered.kind != firstDelivery.kind || redelivered.data != firstDelivery.data {
		t.Fatalf("the redelivered outcome is not the same one: first=%+v second=%+v", firstDelivery, redelivered)
	}
	observed[run.ID+"|"+redelivered.kind]++

	// A recipient that did deliver persists the cursor past the outcome: it must
	// not be handed the same outcome a second time.
	caughtUp := openSSEStream(t, watchURL+"&after="+sequence)
	duplicate, ok := caughtUp.next(1500 * time.Millisecond)
	caughtUp.close()
	if ok {
		t.Fatalf("a cursor past the outcome replayed it: %+v", duplicate)
	}

	// A second restart must not turn the same terminal state into another
	// outcome, and must not renumber the one already stored.
	restarted := NewServer(&runTestRouter{}).WithTraceStorage(storage)
	if reconciled, err := restarted.Store().ReconcileTerminalNotifications(); err != nil || reconciled != 0 {
		t.Fatalf("a second restart re-announced the outcome: count=%d err=%v", reconciled, err)
	}
	afterRestart, _, err := restarted.Store().LoadNotificationsAfter(0, 100, map[string]struct{}{run.ID: {}})
	if err != nil || len(afterRestart) != 1 || strconv.FormatUint(afterRestart[0].Sequence, 10) != sequence {
		t.Fatalf("the outcome changed across a restart: %+v err=%v", afterRestart, err)
	}
	postRestartServer := httptest.NewServer(http.HandlerFunc(restarted.HandleLocalNotifications))
	defer postRestartServer.Close()
	reattached := openSSEStream(t, postRestartServer.URL+"?stream=sse&run_id="+run.ID+"&after=0")
	replayed, ok := reattached.next(3 * time.Second)
	reattached.close()
	if !ok || replayed.id != sequence {
		t.Fatalf("a pre-restart cursor must still address the same wakeup: %+v ok=%v", replayed, ok)
	}
	if strings.Contains(replayed.data, privateContent) {
		t.Fatalf("turn content reached the wakeup payload after restart: %s", replayed.data)
	}

	// One run, one terminal state, one logical outcome - however many times the
	// wire handed it over.
	if len(observed) != 1 || observed[run.ID+"|run.completed"] < 2 {
		t.Fatalf("the recipient did not see one redeliverable logical outcome: %#v", observed)
	}
}

// ----------------------------------------------------------------------------
// EP-05.A: an acknowledgement is recorded exactly once per idempotency key.
// ----------------------------------------------------------------------------

type ackResponse struct {
	status   int
	body     string
	replayed string
}

func postNotificationAck(t *testing.T, handler http.HandlerFunc, method, key, body string) ackResponse {
	t.Helper()
	req := httptest.NewRequest(method, "/v1/run-notifications/ack", strings.NewReader(body))
	if key != "" {
		req.Header.Set("Idempotency-Key", key)
	}
	recorder := httptest.NewRecorder()
	handler(recorder, req)
	return ackResponse{
		status:   recorder.Code,
		body:     recorder.Body.String(),
		replayed: recorder.Header().Get("Idempotency-Replayed"),
	}
}

func TestNotificationAckRecordsOneClaimOncePerIdempotencyKey(t *testing.T) {
	storage := memstore.New()
	server := NewServer(&runTestRouter{}).WithTraceStorage(storage)
	const claim = `{"run_id":"run-acked","sequence":7}`

	first := postNotificationAck(t, server.HandleLocalNotificationAck, http.MethodPost, "supervisor-1", claim)
	if first.status != http.StatusOK || first.replayed != "" {
		t.Fatalf("first acknowledgement: status=%d replayed=%q body=%s", first.status, first.replayed, first.body)
	}
	var acked map[string]interface{}
	if err := json.Unmarshal([]byte(first.body), &acked); err != nil {
		t.Fatal(err)
	}
	if acked["status"] != "acked" || acked["run_id"] != "run-acked" || acked["sequence"] != float64(7) {
		t.Fatalf("first acknowledgement body: %s", first.body)
	}
	recorded, found, err := server.loadNotificationAck("supervisor-1")
	if err != nil || !found || recorded.Sequence != 7 || recorded.RunID != "run-acked" {
		t.Fatalf("the claim was not recorded once: %+v found=%v err=%v", recorded, found, err)
	}

	// The retry of a claim the daemon already recorded is answered from the
	// record: same outcome, and the record itself is not written again.
	replay := postNotificationAck(t, server.HandleLocalNotificationAck, http.MethodPost, "supervisor-1", claim)
	if replay.status != http.StatusOK || replay.replayed != "true" {
		t.Fatalf("replay: status=%d replayed=%q body=%s", replay.status, replay.replayed, replay.body)
	}
	if replay.body != first.body {
		t.Fatalf("a replay must return the same outcome: first=%s replay=%s", first.body, replay.body)
	}
	after, found, err := server.loadNotificationAck("supervisor-1")
	if err != nil || !found || !after.AckedAt.Equal(recorded.AckedAt) {
		t.Fatalf("the record was written twice: before=%+v after=%+v err=%v", recorded, after, err)
	}

	// The same key carrying a different claim is a conflict, not a second
	// acknowledgement and not an overwrite of the first.
	conflict := postNotificationAck(t, server.HandleLocalNotificationAck, http.MethodPost, "supervisor-1", `{"run_id":"run-acked","sequence":8}`)
	if conflict.status != http.StatusConflict || !strings.Contains(conflict.body, "idempotency_payload_conflict") {
		t.Fatalf("a reused key with a different claim: status=%d body=%s", conflict.status, conflict.body)
	}
	still, found, err := server.loadNotificationAck("supervisor-1")
	if err != nil || !found || still.Sequence != recorded.Sequence || !still.AckedAt.Equal(recorded.AckedAt) {
		t.Fatalf("the conflict rewrote the first claim: %+v found=%v err=%v", still, found, err)
	}

	// Exactly-once is per claim key: a second supervisor acknowledging the same
	// wakeup is a second, distinct record - not a replay of the first.
	other := postNotificationAck(t, server.HandleLocalNotificationAck, http.MethodPost, "supervisor-2", claim)
	if other.status != http.StatusOK || other.replayed != "" || other.body != first.body {
		t.Fatalf("a distinct key: status=%d replayed=%q body=%s", other.status, other.replayed, other.body)
	}
	if second, found, err := server.loadNotificationAck("supervisor-2"); err != nil || !found || second.Sequence != 7 {
		t.Fatalf("the second claim was not recorded: %+v found=%v err=%v", second, found, err)
	}

	// The client-chosen key is hashed: the store holds claims, never the opaque
	// value a caller picked.
	keys, err := storage.List("notification_ack.")
	if err != nil || len(keys) != 2 {
		t.Fatalf("acknowledgement records: %v err=%v", keys, err)
	}
	for _, key := range keys {
		if strings.Contains(key, "supervisor") {
			t.Fatalf("an idempotency key became a storage key: %s", key)
		}
	}

	// A daemon restart must not turn a recorded claim into a new one: the record
	// is durable, and the replay is answered from it.
	restarted := NewServer(&runTestRouter{}).WithTraceStorage(storage)
	afterRestart := postNotificationAck(t, restarted.HandleLocalNotificationAck, http.MethodPost, "supervisor-1", claim)
	if afterRestart.status != http.StatusOK || afterRestart.replayed != "true" || afterRestart.body != first.body {
		t.Fatalf("after restart: status=%d replayed=%q body=%s", afterRestart.status, afterRestart.replayed, afterRestart.body)
	}
	durable, found, err := restarted.loadNotificationAck("supervisor-1")
	if err != nil || !found || !durable.AckedAt.Equal(recorded.AckedAt) {
		t.Fatalf("the claim did not survive the restart: %+v found=%v err=%v", durable, found, err)
	}
	if keys, err := storage.List("notification_ack."); err != nil || len(keys) != 2 {
		t.Fatalf("a restart changed the recorded claims: %v err=%v", keys, err)
	}
}

func TestNotificationAckRefusesWhatItCannotRecordOnce(t *testing.T) {
	storage := memstore.New()
	server := NewServer(&runTestRouter{}).WithTraceStorage(storage)
	const claim = `{"run_id":"run-acked","sequence":7}`

	for _, tc := range []struct {
		name   string
		method string
		key    string
		body   string
		status int
		detail string
	}{
		{name: "no key", method: http.MethodPost, body: claim, status: http.StatusBadRequest, detail: "idempotency_key_required"},
		{name: "oversized key", method: http.MethodPost, key: strings.Repeat("k", 129), body: claim, status: http.StatusBadRequest, detail: "exceeds 128 bytes"},
		{name: "not json", method: http.MethodPost, key: "sup", body: "not json", status: http.StatusBadRequest, detail: "invalid json"},
		{name: "unknown field", method: http.MethodPost, key: "sup", body: `{"run_id":"r","sequence":1,"extra":true}`, status: http.StatusBadRequest, detail: "invalid json"},
		{name: "no run", method: http.MethodPost, key: "sup", body: `{"sequence":1}`, status: http.StatusBadRequest, detail: "run_id is required"},
		{name: "zero sequence", method: http.MethodPost, key: "sup", body: `{"run_id":"r"}`, status: http.StatusBadRequest, detail: "positive notification sequence"},
		{name: "wrong method", method: http.MethodGet, key: "sup", status: http.StatusMethodNotAllowed, detail: "Method Not Allowed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := postNotificationAck(t, server.HandleLocalNotificationAck, tc.method, tc.key, tc.body)
			if got.status != tc.status || !strings.Contains(got.body, tc.detail) {
				t.Fatalf("status=%d body=%s, want %d containing %q", got.status, got.body, tc.status, tc.detail)
			}
		})
	}
	if keys, err := storage.List("notification_ack."); err != nil || len(keys) != 0 {
		t.Fatalf("a refused acknowledgement was recorded anyway: %v err=%v", keys, err)
	}
}

// ----------------------------------------------------------------------------
// The delivery contract this file proves, stated once:
//
//   * Delivery is at least once. The daemon can prove a wakeup was written, not
//     that a supervisor received it, and there is no server-side per-consumer
//     cursor: the durable cursor is the consumer's own `after` value.
//   * Identity is stable: a wakeup is (run_id, kind), numbered by a monotonic
//     sequence that a restart neither renumbers nor re-announces, because the
//     startup repair of missing terminal wakeups is idempotent.
//   * One logical outcome is therefore the consumer's dedup on (run_id, kind),
//     not a property this channel can enforce by itself.
//   * The payload is an envelope: identity, not content. Transcripts never
//     travel on this wire.
//   * An acknowledgement (POST /v1/run-notifications/ack) is the supervisor's
//     own claim about one delivery, recorded exactly once per idempotency key.
// ----------------------------------------------------------------------------

// TestNotificationAckRecordsOneClaimOnceUnderConcurrency is the property the
// exactly-once record actually needs: the read-check-write is atomic. Eight
// identical claims race for the same key, and exactly one of them may be the
// first one - a lock that is not there returns several "first" claims, which is
// the whole guarantee gone. The single-threaded test above cannot see that.
func TestNotificationAckRecordsOneClaimOnceUnderConcurrency(t *testing.T) {
	storage := memstore.New()
	server := NewServer(&runTestRouter{}).WithTraceStorage(storage)
	const claim = `{"run_id":"run-raced","sequence":3}`
	const claims = 8

	start := make(chan struct{})
	var wg sync.WaitGroup
	var mu sync.Mutex
	firstClaims, replays, refusals := 0, 0, 0
	for i := 0; i < claims; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			// The helper only builds a request and a recorder; it never fails
			// the test, so it is safe off the test goroutine.
			got := postNotificationAck(t, server.HandleLocalNotificationAck, http.MethodPost, "supervisor-race", claim)
			mu.Lock()
			defer mu.Unlock()
			switch {
			case got.status != http.StatusOK:
				refusals++
			case got.replayed == "":
				firstClaims++
			default:
				replays++
			}
		}()
	}
	close(start)
	wg.Wait()

	if refusals != 0 || firstClaims != 1 || replays != claims-1 {
		t.Fatalf("la stessa claim in parallelo ha prodotto prime=%d replay=%d rifiuti=%d, vuole prime=1 replay=%d rifiuti=0",
			firstClaims, replays, refusals, claims-1)
	}
	record, found, err := server.loadNotificationAck("supervisor-race")
	if err != nil || !found || record.Sequence != 3 {
		t.Fatalf("record dopo la corsa: %+v found=%v err=%v", record, found, err)
	}
	if keys, err := storage.List("notification_ack."); err != nil || len(keys) != 1 {
		t.Fatalf("la corsa ha lasciato %d record: %v err=%v", len(keys), keys, err)
	}
}

// failingSetStorage keeps a working store for reads and refuses writes, which is
// what a store outage looks like from the handler.
type failingSetStorage struct {
	middleware.Storage
	err error
}

func (f failingSetStorage) Set(string, []byte) error { return f.err }

// TestNotificationAckNeverClaimsWhatItCouldNotRecord covers the write path a
// happy test cannot: if the record cannot be made durable, answering
// "acked" would claim an exactly-once guarantee Matrix does not have. A record
// written before the outage keeps answering replays, because reading it needs no
// write.
func TestNotificationAckNeverClaimsWhatItCouldNotRecord(t *testing.T) {
	const claim = `{"run_id":"run-unwritten","sequence":5}`
	broken := NewServer(&runTestRouter{}).WithTraceStorage(failingSetStorage{
		Storage: memstore.New(),
		err:     errors.New("vault write unavailable"),
	})

	got := postNotificationAck(t, broken.HandleLocalNotificationAck, http.MethodPost, "supervisor-1", claim)
	if got.status != http.StatusInternalServerError || strings.Contains(got.body, "acked") {
		t.Fatalf("una scrittura fallita è stata annunciata: status=%d body=%s", got.status, got.body)
	}
	if _, found, err := broken.loadNotificationAck("supervisor-1"); err != nil || found {
		t.Fatalf("un record mai scritto risulta presente: found=%v err=%v", found, err)
	}

	// The same claim, with a record already durable from before the outage: the
	// replay is answered from storage, so the outage does not lose a claim that
	// was already made.
	durable := memstore.New()
	seeded := NewServer(&runTestRouter{}).WithTraceStorage(durable)
	if seed := postNotificationAck(t, seeded.HandleLocalNotificationAck, http.MethodPost, "supervisor-2", claim); seed.status != http.StatusOK {
		t.Fatalf("preparazione del record: status=%d body=%s", seed.status, seed.body)
	}
	replay := postNotificationAck(t, NewServer(&runTestRouter{}).WithTraceStorage(failingSetStorage{Storage: durable, err: errors.New("vault write unavailable")}).HandleLocalNotificationAck, http.MethodPost, "supervisor-2", claim)
	if replay.status != http.StatusOK || replay.replayed != "true" {
		t.Fatalf("un record già durabile deve rispondere in replay anche a store non scrivibile: status=%d replayed=%q", replay.status, replay.replayed)
	}
}
