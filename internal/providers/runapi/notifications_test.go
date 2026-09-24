package runapi

import (
	"bufio"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Josepavese/matrix/internal/logic/runtrace"
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
