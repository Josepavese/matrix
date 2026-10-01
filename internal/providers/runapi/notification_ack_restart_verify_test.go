package runapi

import (
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Josepavese/matrix/internal/providers/bolt"
)

// installTestVaultKey generates the vault master key for this test and hands it
// to the storage the way an operator hands it to a deployment: through the
// configuration the provider reads, never through a key that happens to exist on
// the machine running the test.
//
// The durable store is encrypted, so bolt.Set refuses to write without a key and
// this test fails wherever no vault key is configured - which is exactly where a
// real restart most needs to be proven. Both variables are pinned, so a key file
// left in the environment cannot decide the outcome either.
func installTestVaultKey(t *testing.T) {
	t.Helper()
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("generate vault master key: %v", err)
	}
	t.Setenv("MATRIX_VAULT_MASTER_KEY_FILE", "")
	t.Setenv("MATRIX_VAULT_MASTER_KEY", base64.StdEncoding.EncodeToString(key))
}

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
	installTestVaultKey(t)
	path := filepath.Join(t.TempDir(), "vault.db")
	db1, err := bolt.NewProvider(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	server1 := NewServer(&runTestRouter{}).WithTraceStorage(db1)
	// The key is written with surrounding spaces: trimming is part of the contract.
	first := postNotificationAck(t, server1.HandleLocalNotificationAck, http.MethodPost, " supervisor-1 ", `{"run_id":"run-acked","sequence":7}`)
	if first.status != http.StatusOK || first.replayed != "" {
		t.Fatalf("first ack: status=%d replayed=%q body=%s", first.status, first.replayed, first.body)
	}
	if err := db1.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	db2, err := bolt.NewProvider(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	t.Cleanup(func() { _ = db2.Close() })
	server2 := NewServer(&runTestRouter{}).WithTraceStorage(db2)

	replay := postNotificationAck(t, server2.HandleLocalNotificationAck, http.MethodPost, "supervisor-1", `{"run_id":"run-acked","sequence":7}`)
	if replay.status != http.StatusOK || replay.replayed != "true" || replay.body != first.body {
		t.Fatalf("replay after a real restart: status=%d replayed=%q body=%s", replay.status, replay.replayed, replay.body)
	}
	// Same claim, reordered JSON: the digest covers the decoded values, not the bytes.
	reordered := postNotificationAck(t, server2.HandleLocalNotificationAck, http.MethodPost, "supervisor-1", `{"sequence":7,"run_id":"run-acked"}`)
	if reordered.status != http.StatusOK || reordered.replayed != "true" {
		t.Fatalf("reordered JSON: status=%d replayed=%q body=%s", reordered.status, reordered.replayed, reordered.body)
	}
	// The case that is almost always missing after a real restart.
	conflict := postNotificationAck(t, server2.HandleLocalNotificationAck, http.MethodPost, "supervisor-1", `{"run_id":"other-run","sequence":7}`)
	if conflict.status != http.StatusConflict || !strings.Contains(conflict.body, "idempotency_payload_conflict") {
		t.Fatalf("conflict after restart: status=%d body=%s", conflict.status, conflict.body)
	}

	keys, err := db2.List("notification_ack.")
	if err != nil || len(keys) != 1 {
		t.Fatalf("records on disk: %v err=%v", keys, err)
	}
	if strings.Contains(keys[0], "supervisor") {
		t.Fatalf("client key became the storage key: %s", keys[0])
	}
	blob, err := db2.Get(keys[0])
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if strings.Contains(string(blob), "supervisor-1") {
		t.Fatalf("the plaintext key reached the durable record: %s", blob)
	}
	// The value on disk is encrypted under the key this test procured, so a store
	// that silently wrote plaintext would not pass here either, and the key the
	// test installs is provably the one doing the work.
	if encrypted, plaintext, err := db2.InspectRawEncryption(); err != nil || encrypted != 1 || plaintext != 0 {
		t.Fatalf("value at rest: encrypted=%d plaintext=%d err=%v", encrypted, plaintext, err)
	}
	rec, found, err := server2.loadNotificationAck("supervisor-1")
	if err != nil || !found || rec.RunID != "run-acked" || rec.Sequence != 7 {
		t.Fatalf("record decoded after restart: %+v found=%v err=%v", rec, found, err)
	}
}
