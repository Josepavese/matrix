package runapi

import (
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/Josepavese/matrix/internal/logic/memstore"
)

// TestTheAcknowledgementRefusesABodyLargerThanItAccepts pins the third decode
// outcome on this endpoint too. An oversized body is not a syntax error: nothing
// can be said about the shape of a document that was never read to the end, and
// one error must not get two different answers on two endpoints of the same API.
func TestTheAcknowledgementRefusesABodyLargerThanItAccepts(t *testing.T) {
	// The declared limit is a policy, so it is pinned as data.
	if notificationAckBodyMaxBytes != 4<<10 {
		t.Fatalf("the acknowledgement body limit is %d, want %d", notificationAckBodyMaxBytes, 4<<10)
	}
	server := NewServer(&runTestRouter{}).WithTraceStorage(memstore.New())

	answer := postNotificationAck(t, server.HandleLocalNotificationAck, http.MethodPost,
		"supervisor-limit", ackRequestBodyOfSize(notificationAckBodyMaxBytes+1))

	if answer.status != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want %d: %s", answer.status, http.StatusRequestEntityTooLarge, answer.body)
	}
	if !strings.Contains(answer.body, strconv.Itoa(notificationAckBodyMaxBytes)) {
		t.Fatalf("the answer must name the limit %d, got %q", notificationAckBodyMaxBytes, answer.body)
	}
	if strings.Contains(answer.body, "invalid json") {
		t.Fatalf("an oversized body is not a syntax error, got %q", answer.body)
	}
}

// TestTheAcknowledgementAcceptsABodyAtTheLimit is the other half: the ceiling is
// a ceiling, not a silent truncation, so an acknowledgement that fits is still
// recorded.
func TestTheAcknowledgementAcceptsABodyAtTheLimit(t *testing.T) {
	server := NewServer(&runTestRouter{}).WithTraceStorage(memstore.New())

	answer := postNotificationAck(t, server.HandleLocalNotificationAck, http.MethodPost,
		"supervisor-limit", ackRequestBodyOfSize(notificationAckBodyMaxBytes))

	if answer.status != http.StatusOK {
		t.Fatalf("a body exactly at the limit was refused: status = %d: %s", answer.status, answer.body)
	}
	if !strings.Contains(answer.body, `"status":"acked"`) {
		t.Fatalf("an acknowledgement inside the limit was not recorded: %s", answer.body)
	}
}

// ackRequestBodyOfSize builds a well-formed acknowledgement of exactly size
// bytes. The padding goes into run_id because the decoder refuses unknown fields,
// so there is no spare field to park it in.
func ackRequestBodyOfSize(size int) string {
	const prefix = `{"run_id":"`
	const suffix = `","sequence":1}`
	if size < len(prefix)+len(suffix) {
		panic("acknowledgement body size is below the envelope")
	}
	return prefix + strings.Repeat("r", size-len(prefix)-len(suffix)) + suffix
}
