package runapi

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

// TestRunRequestRefusesABodyLargerThanItAccepts pins the third decode outcome. A
// body too large to read is neither a syntax error nor a schema error - nothing
// can be said about the shape of a document that was never read to the end - and
// answering it "invalid json" would send the caller looking for a broken body it
// does not have, while the request itself is perfectly well formed.
func TestRunRequestRefusesABodyLargerThanItAccepts(t *testing.T) {
	// The declared limit is a policy, so it is pinned as data: a limit that moves
	// silently is a limit nobody agreed to.
	if runRequestMaxBytes != 1<<20 {
		t.Fatalf("the run request body limit is %d, want %d", runRequestMaxBytes, 1<<20)
	}
	router := &runTestRouter{}
	recorder := httptest.NewRecorder()

	NewServer(router).HandleRuns(recorder, newJSONRequest(http.MethodPost, RunPathV1,
		strings.NewReader(runRequestBodyOfSize(runRequestMaxBytes+1))))

	if recorder.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want %d: %s", recorder.Code, http.StatusRequestEntityTooLarge, recorder.Body.String())
	}
	answer := recorder.Body.String()
	if !strings.Contains(answer, strconv.Itoa(runRequestMaxBytes)) {
		t.Fatalf("the answer must name the limit %d, got %q", runRequestMaxBytes, answer)
	}
	if strings.Contains(answer, "invalid json") {
		t.Fatalf("an oversized body is not a syntax error, got %q", answer)
	}
	if router.lastConversation.ChannelID != "" {
		t.Fatalf("an oversized body must not reach the provider, routed %q", router.lastConversation.ChannelID)
	}
}

// TestRunRequestAcceptsABodyAtTheLimit is the other half of the same policy: the
// ceiling is a ceiling and not a silent truncation, so a request that fits is
// still executed. A limit that refuses real work would be a defect of its own.
func TestRunRequestAcceptsABodyAtTheLimit(t *testing.T) {
	router := &runTestRouter{}
	recorder := httptest.NewRecorder()

	NewServer(router).HandleRuns(recorder, newJSONRequest(http.MethodPost, RunPathV1,
		strings.NewReader(runRequestBodyOfSize(runRequestMaxBytes))))

	if recorder.Code == http.StatusRequestEntityTooLarge {
		t.Fatalf("a body exactly at the limit was refused: %s", recorder.Body.String())
	}
	if router.lastConversation.ChannelID != "c" {
		t.Fatalf("a request inside the limit did not reach the provider: status %d, routed %q",
			recorder.Code, router.lastConversation.ChannelID)
	}
}

// runRequestBodyOfSize builds a well-formed run request of exactly size bytes.
func runRequestBodyOfSize(size int) string {
	const prefix = `{"channel_id":"c","input":"`
	const suffix = `"}`
	if size < len(prefix)+len(suffix) {
		panic("run request body size is below the envelope")
	}
	return prefix + strings.Repeat("x", size-len(prefix)-len(suffix)) + suffix
}
