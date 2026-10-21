package zedacp

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestPresentationUpdatesPreservePatchPresence(t *testing.T) {
	for _, tc := range []struct {
		raw        string
		set, clear bool
	}{
		{`{"sessionUpdate":"compaction_update","compactionId":"c","status":"in_progress"}`, false, false},
		{`{"sessionUpdate":"compaction_update","compactionId":"c","status":"completed","summary":null,"_meta":null}`, true, true},
		{`{"sessionUpdate":"compaction_update","compactionId":"c","status":"completed","summary":[],"_meta":{}}`, true, false},
	} {
		var u SessionUpdate
		if err := json.Unmarshal([]byte(tc.raw), &u); err != nil {
			t.Fatal(err)
		}
		if u.SummarySet != tc.set || u.SummaryCleared != tc.clear || u.MetaSet != tc.set || u.MetaCleared != tc.clear {
			t.Fatalf("patch presence lost: %+v", u)
		}
		b, err := json.Marshal(u)
		if err != nil {
			t.Fatal(err)
		}
		var round SessionUpdate
		if err = json.Unmarshal(b, &round); err != nil {
			t.Fatal(err)
		}
		if round.SummarySet != tc.set || round.SummaryCleared != tc.clear || round.MetaSet != tc.set || round.MetaCleared != tc.clear {
			t.Fatalf("round trip lost clearing operations: %s", b)
		}
	}
}
func TestNoticeAndCompactionRejectMissingRequiredValues(t *testing.T) {
	for _, raw := range []string{
		`{"sessionUpdate":"notice","severity":"info"}`,
		`{"sessionUpdate":"notice","title":"hi"}`,
		`{"sessionUpdate":"compaction_update","status":"completed"}`,
		`{"sessionUpdate":"compaction_summary_chunk","compactionId":"c"}`,
	} {
		var u SessionUpdate
		if json.Unmarshal([]byte(raw), &u) == nil {
			t.Fatalf("malformed update accepted: %s", raw)
		}
	}
	var notice SessionUpdate
	if err := json.Unmarshal([]byte(`{"sessionUpdate":"notice","title":"hi","severity":"future","description":42,"_meta":[]}`), &notice); err != nil {
		t.Fatal(err)
	}
	if notice.Description != "" || notice.MetaSet {
		t.Fatalf("invalid optional display values must be absent: %+v", notice)
	}
}
func TestGlobalSessionWatcherReceivesBetweenPromptsAndStops(t *testing.T) {
	tr := newFakeTransport()
	c := NewClient(context.Background(), tr)
	defer func() { _ = c.Close() }()
	observer := &recordingObserver{}
	stop := c.WatchSessionUpdates(observer)
	c.handleNotification(sessionUpdateResponse(t, "a", "one"))
	c.handleNotification(sessionUpdateResponse(t, "b", "two"))
	if err := c.DrainSessionUpdates(context.Background(), "a"); err != nil {
		t.Fatal(err)
	}
	if err := c.DrainSessionUpdates(context.Background(), "b"); err != nil {
		t.Fatal(err)
	}
	if text := observer.joined(); text != "onetwo" && text != "twoone" {
		t.Fatalf("cross-session updates lost: %q", text)
	}
	stop()
	c.handleNotification(sessionUpdateResponse(t, "a", "late"))
	if err := c.DrainSessionUpdates(context.Background(), "a"); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(observer.joined(), "late") {
		t.Fatal("unregistered observer received new update")
	}
}
func TestV2PromptRejectsMissingInsertionAcknowledgement(t *testing.T) {
	tr := newFakeTransport()
	c := NewClient(context.Background(), tr)
	defer func() { _ = c.Close() }()
	c.protocolVersion = ProtocolVersionV2
	done := make(chan error, 1)
	go func() { _, err := c.Prompt(context.Background(), PromptRequest{SessionID: "s"}, nil); done <- err }()
	waitForSent(t, tr, func(s string) bool { return strings.Contains(s, `"session/prompt"`) }, "prompt")
	tr.push(t, `{"jsonrpc":"2.0","id":1,"result":{"messageId":null}}`)
	if err := <-done; err == nil {
		t.Fatal("missing messageId acknowledged insertion")
	}
}

func TestCloseSessionWaitsForFinalNotificationDelivery(t *testing.T) {
	tr := newFakeTransport()
	c := NewClient(context.Background(), tr)
	defer func() { _ = c.Close() }()
	observer := &heldObserver{recordingObserver: &recordingObserver{}, entered: make(chan struct{}), release: make(chan struct{})}
	defer close(observer.release)
	stop := c.WatchSessionUpdates(observer)
	defer stop()
	c.handleNotification(sessionUpdateResponse(t, "s", "hold"))
	<-observer.entered
	done := make(chan error, 1)
	go func() { done <- c.CloseSession(context.Background(), "s") }()
	waitForSent(t, tr, func(s string) bool { return strings.Contains(s, `"session/close"`) }, "close")
	tr.push(t, `{"jsonrpc":"2.0","id":1,"result":{}}`)
	select {
	case err := <-done:
		t.Fatalf("close returned before final delivery: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	observer.release <- struct{}{}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if observer.joined() != "hold" {
		t.Fatal("final notification lost before close returned")
	}
}

func TestSessionInfoStringPatchRoundTripsEmptyNullAndMalformed(t *testing.T) {
	for _, raw := range []string{`{"sessionUpdate":"session_info_update","title":""}`, `{"sessionUpdate":"session_info_update","title":null,"updatedAt":null}`} {
		var u SessionUpdate
		if err := json.Unmarshal([]byte(raw), &u); err != nil {
			t.Fatal(err)
		}
		b, err := json.Marshal(u)
		if err != nil {
			t.Fatal(err)
		}
		var round SessionUpdate
		if err = json.Unmarshal(b, &round); err != nil {
			t.Fatal(err)
		}
		if !round.TitleSet || round.TitleCleared != u.TitleCleared || round.UpdatedAtSet != u.UpdatedAtSet {
			t.Fatalf("session clear lost: %s", b)
		}
	}
	var u SessionUpdate
	if err := json.Unmarshal([]byte(`{"sessionUpdate":"session_info_update","title":42,"updatedAt":[]}`), &u); err != nil {
		t.Fatal(err)
	}
	if u.TitleSet || u.UpdatedAtSet {
		t.Fatal("malformed optional strings act as explicit clears")
	}
}
