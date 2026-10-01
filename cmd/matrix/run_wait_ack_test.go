//go:build linux || darwin

package main

import (
	"bytes"
	"context"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Josepavese/matrix/internal/logic/runtrace"
	"github.com/Josepavese/matrix/internal/providers/runapi"
	"github.com/spf13/cobra"
)

// serveNotificationSocket serves the daemon's own local notification handlers
// on a socket the test owns. Using the real handlers is the point: a fake would
// only prove the client agrees with the fake.
func serveNotificationHandlersOnSocket(t *testing.T, server *runapi.Server, socketPath string) {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc(runNotificationsPath, server.HandleLocalNotifications)
	mux.HandleFunc(runNotificationsAckPath, server.HandleLocalNotificationAck)
	if err := os.MkdirAll(filepath.Dir(socketPath), 0o700); err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	httpServer := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() { _ = httpServer.Serve(listener) }()
	t.Cleanup(func() { _ = httpServer.Close() })
}

// newNotificationTestServer returns a daemon-side notification API and the
// socket path a client of it resolves from a Matrix home.
func newNotificationTestServer(t *testing.T) (*runapi.Server, string) {
	t.Helper()
	server := runapi.NewServer(nil)
	socketPath := notificationSocketPath(t.TempDir())
	serveNotificationHandlersOnSocket(t, server, socketPath)
	return server, socketPath
}

// testCommand captures what a command prints without running a process.
func testCommand() (*cobra.Command, *bytes.Buffer) {
	buffer := &bytes.Buffer{}
	command := &cobra.Command{}
	command.SetContext(context.Background())
	command.SetOut(buffer)
	command.SetErr(buffer)
	return command, buffer
}

func withWaitFlags(t *testing.T, after uint64, timeout time.Duration) {
	t.Helper()
	previousAfter, previousTimeout := runWaitAfter, runWaitTimeout
	runWaitAfter, runWaitTimeout = after, timeout
	t.Cleanup(func() { runWaitAfter, runWaitTimeout = previousAfter, previousTimeout })
}

// TestRunWaitReportsTheTerminalOutcomeAndNeverTheTurn is the acceptance
// evidence for the wait primitive: a terminal event is delivered with the run
// id and its outcome, the cursor is reported so a restart resumes, and the
// content the run carried never crosses this surface.
func TestRunWaitReportsTheTerminalOutcomeAndNeverTheTurn(t *testing.T) {
	const privateContent = "contenuto privato del turno che non deve viaggiare"
	server, socketPath := newNotificationTestServer(t)

	if _, err := server.Store().AppendEvent(runtrace.Event{RunID: "run-watched", Kind: "agent.message", Message: privateContent}); err != nil {
		t.Fatal(err)
	}
	if _, err := server.Store().AppendEvent(runtrace.Event{
		RunID:    "run-other",
		Kind:     "run.completed",
		Metadata: map[string]interface{}{"failure_code": "should-not-be-reported"},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := server.Store().AppendEvent(runtrace.Event{
		RunID:    "run-watched",
		Kind:     "run.failed",
		Metadata: map[string]interface{}{"failure_code": "agent_timeout"},
	}); err != nil {
		t.Fatal(err)
	}

	withWaitFlags(t, 0, 5*time.Second)
	command, output := testCommand()
	if err := runWaitAt(command, socketPath, "run-watched"); err != nil {
		t.Fatalf("wait failed: %v", err)
	}
	printed := output.String()
	for _, want := range []string{"run_id=run-watched", "outcome=failed", "failure_code=agent_timeout", "cursor="} {
		if !strings.Contains(printed, want) {
			t.Fatalf("wait output %q is missing %q", printed, want)
		}
	}
	if strings.Contains(printed, privateContent) {
		t.Fatalf("the turn content crossed the wait surface: %q", printed)
	}
	if strings.Contains(printed, "should-not-be-reported") {
		t.Fatalf("another run's outcome was reported for this run: %q", printed)
	}
}

// TestRunWaitResumesFromTheCursorWithoutReplaying pins the durability half: a
// supervisor that already consumed a wakeup does not receive it again, and is
// told the cursor to continue from.
func TestRunWaitResumesFromTheCursorWithoutReplaying(t *testing.T) {
	server, socketPath := newNotificationTestServer(t)
	if _, err := server.Store().AppendEvent(runtrace.Event{RunID: "run-watched", Kind: "run.cancelled"}); err != nil {
		t.Fatal(err)
	}

	withWaitFlags(t, 0, 5*time.Second)
	command, output := testCommand()
	if err := runWaitAt(command, socketPath, "run-watched"); err != nil {
		t.Fatalf("first wait failed: %v", err)
	}
	cursor := cursorFromOutput(t, output.String())
	if cursor == 0 {
		t.Fatal("a delivered wakeup must advance the cursor")
	}

	// Resuming from the delivered cursor must find nothing new: the wakeup was
	// delivered once, and the short timeout turns "nothing new" into an error
	// that names the resume point.
	withWaitFlags(t, cursor, 300*time.Millisecond)
	command, output = testCommand()
	err := runWaitAt(command, socketPath, "run-watched")
	if err == nil {
		t.Fatalf("a consumed wakeup was delivered again: %q", output.String())
	}
	if !strings.Contains(err.Error(), "resume with --after") {
		t.Fatalf("the timeout must tell the caller how to resume, got %v", err)
	}
	if !strings.Contains(output.String(), "outcome=timeout") {
		t.Fatalf("a timeout must still report the cursor, got %q", output.String())
	}
	if !strings.Contains(output.String(), "cursor="+strconv.FormatUint(cursor, 10)) {
		t.Fatalf("a timeout must report the cursor it stopped at, got %q", output.String())
	}
}

// TestRunAckIsRecordedExactlyOncePerKey is the acceptance evidence for the ack
// primitive: the same key and claim is one recorded acknowledgement reported as
// a replay, a different claim under the same key is refused, and a missing key
// is refused before anything is sent.
func TestRunAckIsRecordedExactlyOncePerKey(t *testing.T) {
	_, socketPath := newNotificationTestServer(t)
	client := newLocalNotificationClient(socketPath, "")
	ctx := context.Background()

	replayed, err := client.ack(ctx, ackRequest{RunID: "run-acked", Sequence: 7, IdempotencyKey: "supervisor-1"})
	if err != nil || replayed {
		t.Fatalf("first acknowledgement = (replayed %t, err %v), want a fresh success", replayed, err)
	}
	replayed, err = client.ack(ctx, ackRequest{RunID: "run-acked", Sequence: 7, IdempotencyKey: "supervisor-1"})
	if err != nil || !replayed {
		t.Fatalf("replay = (replayed %t, err %v), want a reported replay", replayed, err)
	}
	if _, err := client.ack(ctx, ackRequest{RunID: "run-acked", Sequence: 8, IdempotencyKey: "supervisor-1"}); err == nil {
		t.Fatal("the same key with a different claim must be refused")
	}
	if _, err := client.ack(ctx, ackRequest{RunID: "run-acked", Sequence: 7, IdempotencyKey: "  "}); err == nil {
		t.Fatal("an acknowledgement without a stable key must be refused")
	}
}

func cursorFromOutput(t *testing.T, printed string) uint64 {
	t.Helper()
	for _, field := range strings.Fields(printed) {
		if value, found := strings.CutPrefix(field, "cursor="); found {
			cursor, err := strconv.ParseUint(value, 10, 64)
			if err != nil {
				t.Fatalf("unreadable cursor %q: %v", value, err)
			}
			return cursor
		}
	}
	t.Fatalf("no cursor in %q", printed)
	return 0
}
