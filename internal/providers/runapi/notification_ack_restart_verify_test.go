package runapi

import (
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Josepavese/matrix/internal/providers/bolt"
)

// Independent verification of the acknowledgement restart claim, on a real
// durable store: the author's proof restarts the Server over the same
// in-memory storage, which shows a new instance answers from the store but not
// that the claim is on disk. Here the bbolt file is closed and reopened, and the
// replayed key, the payload conflict, the hashed storage key and the absence of
// the client key in the durable record are all asserted after that. It also
// covers the key trimming the contract relies on (" supervisor-1 " ==
// "supervisor-1") and a reordered JSON body, which must replay rather than
// conflict because the digest is computed over the decoded values.
func TestVerifyAckSurvivesARealRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "vault.db")
	db1, err := bolt.NewProvider(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	server1 := NewServer(&runTestRouter{}).WithTraceStorage(db1)
	// La chiave e' scritta con spazi attorno: il trim e' parte del contratto.
	first := postNotificationAck(t, server1.HandleLocalNotificationAck, http.MethodPost, " supervisor-1 ", `{"run_id":"run-acked","sequence":7}`)
	if first.status != http.StatusOK || first.replayed != "" {
		t.Fatalf("prima ack: status=%d replayed=%q body=%s", first.status, first.replayed, first.body)
	}
	if err := db1.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	db2, err := bolt.NewProvider(path)
	if err != nil {
		t.Fatalf("riapertura: %v", err)
	}
	t.Cleanup(func() { _ = db2.Close() })
	server2 := NewServer(&runTestRouter{}).WithTraceStorage(db2)

	replay := postNotificationAck(t, server2.HandleLocalNotificationAck, http.MethodPost, "supervisor-1", `{"run_id":"run-acked","sequence":7}`)
	if replay.status != http.StatusOK || replay.replayed != "true" || replay.body != first.body {
		t.Fatalf("replay dopo restart vero: status=%d replayed=%q body=%s", replay.status, replay.replayed, replay.body)
	}
	// stessa claim, JSON riordinato: il digest e' sui valori, non sui byte
	reordered := postNotificationAck(t, server2.HandleLocalNotificationAck, http.MethodPost, "supervisor-1", `{"sequence":7,"run_id":"run-acked"}`)
	if reordered.status != http.StatusOK || reordered.replayed != "true" {
		t.Fatalf("JSON riordinato: status=%d replayed=%q body=%s", reordered.status, reordered.replayed, reordered.body)
	}
	// il caso che quasi sempre manca, dopo un restart vero
	conflict := postNotificationAck(t, server2.HandleLocalNotificationAck, http.MethodPost, "supervisor-1", `{"run_id":"other-run","sequence":7}`)
	if conflict.status != http.StatusConflict || !strings.Contains(conflict.body, "idempotency_payload_conflict") {
		t.Fatalf("conflitto dopo restart: status=%d body=%s", conflict.status, conflict.body)
	}

	keys, err := db2.List("notification_ack.")
	if err != nil || len(keys) != 1 {
		t.Fatalf("record su disco: %v err=%v", keys, err)
	}
	if strings.Contains(keys[0], "supervisor") {
		t.Fatalf("chiave del client diventata chiave di storage: %s", keys[0])
	}
	blob, err := db2.Get(keys[0])
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if strings.Contains(string(blob), "supervisor-1") {
		t.Fatalf("la chiave in chiaro e' finita nel record durevole: %s", blob)
	}
	rec, found, err := server2.loadNotificationAck("supervisor-1")
	if err != nil || !found || rec.RunID != "run-acked" || rec.Sequence != 7 {
		t.Fatalf("record decodificato dopo restart: %+v found=%v err=%v", rec, found, err)
	}
}
