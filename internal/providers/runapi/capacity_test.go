package runapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Josepavese/matrix/internal/logic/admission"
)

func TestCapacityRefusalIsDurableAndRequestReachesSharedRouter(t *testing.T) {
	router := &runTestRouter{routeErr: &admission.Refusal{Code: "capacity_disk_exhausted"}}
	server := NewServer(router)
	w := httptest.NewRecorder()
	server.HandleRuns(w, newJSONRequest(http.MethodPost, RunPathV1, strings.NewReader(`{"channel_id":"capacity-proof","input":"work","capacity":{"min_disk_free_bytes":100,"reserve_disk_bytes":50}}`)))
	if w.Code != http.StatusTooManyRequests {
		t.Fatal("capacity refusal lost", w.Code, w.Body.String())
	}
	if router.lastConversation.Capacity.MinDiskFreeBytes != 100 || router.lastConversation.Capacity.ReserveDiskBytes != 50 {
		t.Fatal("requested capacity ignored")
	}
	var response struct {
		RunID string `json:"run_id"`
		Code  string `json:"code"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	run, found, err := server.runStore.LoadRun(response.RunID)
	if err != nil || !found || run.Status != "failed" {
		t.Fatal("refused run not durable", err)
	}
	if response.Code != "capacity_disk_exhausted" {
		t.Fatal("typed refusal code lost", response.Code)
	}
	trace, found, err := server.runStore.Trace(run.ID)
	if err != nil || !found {
		t.Fatal(err)
	}
	code := ""
	for _, event := range trace.Events {
		if event.Kind == "run.failed" {
			code, _ = event.Metadata["failure_code"].(string)
		}
	}
	if code != "capacity_disk_exhausted" {
		t.Fatal("durable refusal code lost", code)
	}
}

func TestAbsentCapacityKeepsHistoricalIdempotencyPayload(t *testing.T) {
	var req runRequest
	if err := json.Unmarshal([]byte(`{"channel_id":"c","input":"work"}`), &req); err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(req)
	if err != nil || req.Capacity != nil || strings.Contains(string(encoded), `"capacity"`) {
		t.Fatal("default changed persisted idempotency payload")
	}
	var explicit runRequest
	if err := json.Unmarshal([]byte(`{"channel_id":"c","input":"work","capacity":{}}`), &explicit); err != nil {
		t.Fatal(err)
	}
	value, err := json.Marshal(explicit)
	if err != nil || explicit.Capacity == nil || !strings.Contains(string(value), `"capacity"`) {
		t.Fatal("explicit capacity lost its request identity")
	}
}
