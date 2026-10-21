package agents

import (
	"encoding/json"
	"github.com/Josepavese/matrix/internal/middleware"
	"reflect"
	"testing"
)

func decodePresentation(t *testing.T, raw string) acpSessionUpdate {
	t.Helper()
	var u acpSessionUpdate
	if err := json.Unmarshal([]byte(raw), &u); err != nil {
		t.Fatal(err)
	}
	return u
}
func TestCompactionReplaysPatchAndPreserveTimeline(t *testing.T) {
	p := &acpPresentation{version: 2}
	send := func(raw string) {
		p.OnUpdate(acpSessionNotification{SessionID: "s", Update: decodePresentation(t, raw)})
	}
	send(`{"sessionUpdate":"compaction_summary_chunk","compactionId":"first","content":{"type":"text","text":"early"}}`)
	send(`{"sessionUpdate":"compaction_update","compactionId":"second","status":"in_progress"}`)
	send(`{"sessionUpdate":"compaction_update","compactionId":"first","status":"completed","_meta":{"keep":1,"drop":2,"nested":{"old":1}}}`)
	send(`{"sessionUpdate":"compaction_update","compactionId":"first","status":"completed","_meta":{"drop":null,"nested":{"new":2}}}`)
	items := p.snapshot("s")
	if len(items) != 2 || items[0].ID != "first" || items[0].Summary[0].Text != "early" {
		t.Fatalf("replay/timeline lost: %+v", items)
	}
	want := map[string]interface{}{"keep": float64(1), "nested": map[string]interface{}{"new": float64(2)}}
	if !reflect.DeepEqual(items[0].Meta, want) {
		t.Fatalf("metadata patch is not top-level replace/remove: %#v", items[0].Meta)
	}
	items[0].Summary[0].Text = "tamper"
	items[0].Meta["keep"] = "tamper"
	if p.snapshot("s")[0].Summary[0].Text != "early" || p.snapshot("s")[0].Meta["keep"] != float64(1) {
		t.Fatal("snapshot borrows mutable owner state")
	}
	send(`{"sessionUpdate":"compaction_update","compactionId":"first","status":"completed","summary":null,"error":null,"_meta":null}`)
	cleared := p.snapshot("s")[0]
	if len(cleared.Summary) != 0 || len(cleared.Meta) != 0 || cleared.Error != "" {
		t.Fatalf("explicit clear ignored: %+v", cleared)
	}
}
func TestStableCompactionMetadataReplacesAndNoticeDoesNotPersist(t *testing.T) {
	p := &acpPresentation{version: 1}
	n := &observerTestNotifier{}
	unbind := p.bind("s", n)
	for _, raw := range []string{
		`{"sessionUpdate":"compaction_update","compactionId":"c","status":"in_progress","_meta":{"old":true}}`,
		`{"sessionUpdate":"compaction_update","compactionId":"c","status":"completed","_meta":{"new":true}}`,
		`{"sessionUpdate":"notice","severity":"warning","title":"Be aware","description":"live only"}`,
	} {
		p.OnUpdate(acpSessionNotification{SessionID: "s", Update: decodePresentation(t, raw)})
	}
	if len(p.snapshot("s")) != 1 || len(p.snapshot("s")[0].Meta) != 1 || p.snapshot("s")[0].Meta["new"] != true {
		t.Fatal("stable metadata must replace object")
	}
	if len(n.updates) != 3 || n.updates[2].Type != middleware.ThoughtTypeNotice {
		t.Fatalf("notice not projected neutrally: %+v", n.updates)
	}
	unbind()
	p.OnUpdate(acpSessionNotification{SessionID: "s", Update: decodePresentation(t, `{"sessionUpdate":"notice","severity":"info","title":"late"}`)})
	if len(n.updates) != 3 {
		t.Fatal("notice leaked to old run notifier")
	}
}
