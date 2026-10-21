package runapi

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Josepavese/matrix/internal/middleware"
	runresponse "github.com/Josepavese/matrix/internal/providers/runapi/response"
	"github.com/Josepavese/matrix/internal/providers/runpayload"
)

type richInputRouter struct {
	runTestRouter
	imageSupported  bool
	capabilityErr   error
	capabilityCalls int
}

func (r *richInputRouter) AgentCapabilities(context.Context, string) (middleware.ProviderCapabilityReport, error) {
	r.capabilityCalls++
	return middleware.ProviderCapabilityReport{Content: map[string]middleware.CapabilityDescriptor{"image": {Supported: r.imageSupported}}}, r.capabilityErr
}

func (r *richInputRouter) HandleSessionActionTyped(ctx context.Context, req middleware.SessionActionRequest) (middleware.SessionActionResult, error) {
	if req.Action == "capabilities" {
		report, err := r.AgentCapabilities(ctx, req.AgentID)
		return middleware.SessionActionResult{Capabilities: &report}, err
	}
	return r.runTestRouter.HandleSessionActionTyped(ctx, req)
}

func imageInputBody(t *testing.T, c color.RGBA) string {
	t.Helper()
	im := image.NewRGBA(image.Rect(0, 0, 8, 8))
	im.Set(2, 4, c)
	var b bytes.Buffer
	if err := png.Encode(&b, im); err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(map[string]any{"channel_id": "rich", "model_id": "selected-model", "input": map[string]any{"text": "inspect", "content": []middleware.Content{{Type: "image", MimeType: "image/png", Data: base64.StdEncoding.EncodeToString(b.Bytes()), Meta: map[string]any{"page": 1, "revision": "r1"}}}}})
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func submitRich(server *Server, body, key string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	r := newJSONRequest(http.MethodPost, RunPathV1, strings.NewReader(body))
	r.Header.Set("Idempotency-Key", key)
	server.HandleRuns(w, r)
	return w
}

func TestRichRunPreservesImageLifecycleAndWholePayloadIdempotency(t *testing.T) {
	router := &richInputRouter{imageSupported: true}
	server := NewServer(router)
	body := imageInputBody(t, color.RGBA{R: 255, A: 255})
	first := submitRich(server, body, "image-key")
	if first.Code != http.StatusCreated {
		t.Fatal(first.Code, first.Body.String())
	}
	if len(router.lastConversation.ContentBlocks) != 1 || router.lastConversation.ModelID != "selected-model" {
		t.Fatal("rich turn or pin lost")
	}
	data := router.lastConversation.ContentBlocks[0].Data
	if router.lastConversation.ContentBlocks[0].Meta["revision"] != "r1" {
		t.Fatal("provenance lost")
	}
	var created runresponse.Success
	if err := json.Unmarshal(first.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	trace, found, err := server.Store().Trace(created.RunID)
	if err != nil || !found {
		t.Fatal(err)
	}
	var input runpayload.Input
	var request runRequest
	if err = json.Unmarshal([]byte(body), &request); err != nil {
		t.Fatal(err)
	}
	input = request.Input
	stored, _, _ := server.Store().LoadRun(created.RunID)
	if stored.InputDigest != input.Digest() || trace.Run.Status != "completed" {
		t.Fatal("payload identity or terminal lost")
	}
	encoded, _ := json.Marshal(trace)
	if bytes.Contains(encoded, []byte(data)) || !hasEventKind(trace.Events, inputAcceptedEvent) {
		t.Fatal("raw image leaked or manifest missing")
	}
	calls := router.capabilityCalls
	replay := submitRich(server, body, "image-key")
	if router.capabilityCalls != calls {
		t.Fatal("idempotent replay started a new capability probe")
	}
	if replay.Code != http.StatusAccepted || replay.Header().Get("Idempotency-Replayed") != "true" {
		t.Fatal(replay.Code, replay.Body.String())
	}
	for _, changed := range []string{imageInputBody(t, color.RGBA{B: 255, A: 255}), strings.Replace(body, `"revision":"r1"`, `"revision":"r2"`, 1)} {
		conflict := submitRich(server, changed, "image-key")
		if conflict.Code != http.StatusConflict {
			t.Fatal("different rich payload replayed", conflict.Code)
		}
	}
}

func TestRichRunRejectsMissingCapabilityBeforeDispatch(t *testing.T) {
	body := imageInputBody(t, color.RGBA{R: 255, A: 255})
	cases := []struct {
		name   string
		router Router
		body   string
		status int
	}{
		{"unsupported", &richInputRouter{}, body, 409},
		{"unavailable", &richInputRouter{capabilityErr: errors.New("private error")}, body, 503},
		{"reporter absent", &runTestRouter{}, body, 409},
		{"model absent", &richInputRouter{imageSupported: true}, strings.Replace(body, `"model_id":"selected-model"`, `"model_id":""`, 1), 400},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := submitRich(NewServer(tc.router), tc.body, "")
			if w.Code != tc.status || strings.Contains(w.Body.String(), "private error") {
				t.Fatal(w.Code, w.Body.String())
			}
		})
	}
}

func TestRunRejectsAmbiguousIngressBeforeDispatch(t *testing.T) {
	for _, body := range []string{
		`{"channel_id":"c","input":"a","unknown":"private"}`,
		`{"channel_id":"c","input":{"text":"a","unknown":"private"}}`,
		`{"channel_id":"c","input":"a","input":"b"}`,
		`{"channel_id":"c","input":{"text":"a","text":"b"}}`,
		`{"channel_id":"c","input":"a","Input":"b"}`,
		`{"channel_id":"c","input":"a"} {}`,
		`{"channel_id":"c","input":{"content":[{"type":"image","uri":"http://localhost/private"}]}}`,
	} {
		router := &richInputRouter{imageSupported: true}
		w := submitRich(NewServer(router), body, "")
		if w.Code != 400 || router.lastConversation.ChannelID != "" || strings.Contains(w.Body.String(), "private") {
			t.Fatal(w.Code, w.Body.String())
		}
	}
}

func TestRichRunEphemeralCleanupStillOwnsItsSession(t *testing.T) {
	router := &richInputRouter{imageSupported: true}
	server := NewServer(router)
	body := imageInputBody(t, color.RGBA{G: 255, A: 255})
	body = strings.TrimSuffix(body, "}") + `,"session_policy":"new_ephemeral_delete_after_run","cleanup_policy":"delete_remote_or_forget_local"}`
	w := submitRich(server, body, "")
	if w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	if len(router.lastConversation.ContentBlocks) != 1 || lastSessionAction(router.sessionActions, "cleanup") == nil {
		t.Fatal("rich payload changed session lifetime")
	}
}

func TestRichRunAcceptsImageOnlyAndTextBlocksInOrder(t *testing.T) {
	router := &richInputRouter{imageSupported: true}
	body := strings.Replace(imageInputBody(t, color.RGBA{G: 255, A: 255}), `"text":"inspect"`, `"text":""`, 1)
	w := submitRich(NewServer(router), body, "")
	if w.Code != 201 || router.lastConversation.Input != "" || len(router.lastConversation.ContentBlocks) != 1 {
		t.Fatal("image-only turn lost", w.Code)
	}
	w = submitRich(NewServer(router), `{"channel_id":"text-blocks","input":{"content":[{"type":"text","text":"first"},{"type":"text","text":"second"}]}}`, "")
	if w.Code != 201 || router.lastConversation.ContentBlocks[0].Text != "first" || router.lastConversation.ContentBlocks[1].Text != "second" {
		t.Fatal("text block order lost", w.Code)
	}
}

func TestRichRunCapabilityRefusalHasTerminalCodeAndCleanup(t *testing.T) {
	router := &richInputRouter{}
	server := NewServer(router)
	body := strings.TrimSuffix(imageInputBody(t, color.RGBA{R: 255, A: 255}), "}") + `,"session_policy":"new_ephemeral_delete_after_run","cleanup_policy":"delete_remote_or_forget_local"}`
	w := submitRich(server, body, "")
	if w.Code != 409 {
		t.Fatal(w.Code, w.Body.String())
	}
	var result map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result["code"] != inputUnsupported || result["status"] != "failed" {
		t.Fatal("typed failure lost", result)
	}
	cleanup, ok := result["cleanup"].(map[string]any)
	if !ok || cleanup["clean"] != true {
		t.Fatal("admission client escaped run cleanup", result)
	}
	runID, _ := result["run_id"].(string)
	events, err := server.Store().LoadEvents(runID, 100)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, event := range events {
		if event.Kind == "run.failed" && event.Metadata["failure_code"] == inputUnsupported {
			found = true
		}
	}
	if !found {
		t.Fatal("durable refusal code absent")
	}
}
