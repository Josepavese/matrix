package a2a

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	a2asdk "github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2asrv/push"
)

func fetchCard(t *testing.T, url string) a2asdk.AgentCard {
	t.Helper()
	resp, err := http.Get(url + "/.well-known/agent-card.json")
	if err != nil {
		t.Fatalf("GET agent card: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	var card a2asdk.AgentCard
	if err := json.NewDecoder(resp.Body).Decode(&card); err != nil {
		t.Fatalf("decode agent card: %v", err)
	}
	return card
}

// TestMessageSendAnswersUnderTheNameTheSpecificationDefines drives message/send with
// both the name the v1.0 JSON-RPC binding defines (SendMessage, §5.3 and §9.4.1) and
// the slash-separated name of the 0.3 generation, which the ingress also accepts, and
// asserts the response body: the terminal state, the artifact carrying the routed
// answer, and the request message in the history.
func TestMessageSendAnswersUnderTheNameTheSpecificationDefines(t *testing.T) {
	router := &stubSessionRouter{}
	server := newA2ATestServer(t, NewServer(router, "http://127.0.0.1:0", "opencode"))

	for _, method := range []string{"SendMessage", "message/send"} {
		t.Run(method, func(t *testing.T) {
			envelope := callRPC(t, server.URL, method, sendMessageParams("m-send", "count the files"))
			task := envelope.requireTask(t, method)
			if task.Status.State != stateCompleted {
				t.Fatalf("state = %q (%s), want %s", task.Status.State, task.statusMessageText(), stateCompleted)
			}
			if answer := task.artifactText(); answer != "matrix:count the files" {
				t.Fatalf("artifact = %q, want the routed answer", answer)
			}
			if len(task.History) != 1 || task.History[0].MessageID != "m-send" {
				t.Fatalf("history = %#v, want the request message once", task.History)
			}
			if router.channelID == "" || router.agentID != "opencode" || router.input != "count the files" {
				t.Fatalf("router saw channel=%q agent=%q input=%q", router.channelID, router.agentID, router.input)
			}
		})
	}
}

// TestMessageStreamEmitsTheSpecifiedEventSequence asserts the ordered stream the
// streaming capability promises: the Task object first, then status and artifact
// updates, and a close that coincides with the terminal state (§3.1.2, §3.5.2).
func TestMessageStreamEmitsTheSpecifiedEventSequence(t *testing.T) {
	router := &stubSessionRouter{}
	adapter := NewServer(router, "http://127.0.0.1:0", "opencode")
	server := newA2ATestServer(t, adapter)

	if card := fetchCard(t, server.URL); !card.Capabilities.Streaming {
		t.Fatalf("the card does not advertise the streaming capability this test exercises")
	}

	frames := openStream(t, server.URL, "SendStreamingMessage", sendMessageParams("m-stream", "stream me"))

	first := nextFrame(t, frames, "the initial task").result(t)
	if first.Task == nil {
		t.Fatalf("the stream did not begin with a Task object")
	}
	if first.Task.ID == "" || first.Task.Status.State != stateSubmitted {
		t.Fatalf("initial task = %+v, want a submitted task with an id", first.Task)
	}
	taskID := first.Task.ID

	var (
		states   []string
		answer   string
		artifact string
	)
	for {
		frame := nextFrame(t, frames, "the next stream event")
		result := frame.result(t)
		switch {
		case result.StatusUpdate != nil:
			if result.StatusUpdate.TaskID != taskID {
				t.Fatalf("status update for task %q, want %q", result.StatusUpdate.TaskID, taskID)
			}
			states = append(states, result.StatusUpdate.Status.State)
		case result.ArtifactUpdate != nil:
			if result.ArtifactUpdate.TaskID != taskID {
				t.Fatalf("artifact update for task %q, want %q", result.ArtifactUpdate.TaskID, taskID)
			}
			for _, part := range result.ArtifactUpdate.Artifact.Parts {
				answer += part.Text
			}
			artifact = result.ArtifactUpdate.Artifact.Parts[0].Text
		default:
			t.Fatalf("stream frame carries neither a status nor an artifact update: %s", frame.Result)
		}
		if states[len(states)-1] == stateCompleted {
			break
		}
	}

	if len(states) < 2 || states[0] != stateWorking {
		t.Fatalf("status sequence = %v, want it to open with %q", states, stateWorking)
	}
	if artifact == "" {
		t.Fatalf("the stream carried no artifact")
	}
	if answer != "matrix:stream me" {
		t.Fatalf("artifact text = %q, want the routed answer", answer)
	}
	awaitStreamClosed(t, frames)
}

// TestTasksGetAndListServeTheStoredTask covers the two read operations: the task by id
// with its artifacts and history, the error code for an unknown id, the listing with
// cursor fields, the artifact projection switch, and the context filter (§3.1.3, §3.1.4).
func TestTasksGetAndListServeTheStoredTask(t *testing.T) {
	server := newA2ATestServer(t, NewServer(&stubSessionRouter{}, "http://127.0.0.1:0", "opencode"))

	first := callRPC(t, server.URL, "SendMessage", sendMessageParams("m-first", "first")).requireTask(t, "SendMessage")
	second := callRPC(t, server.URL, "SendMessage", sendMessageParams("m-second", "second")).requireTask(t, "SendMessage")

	fetched := callRPC(t, server.URL, "GetTask", fmt.Sprintf(`{"id":%q}`, first.ID)).requireTask(t, "GetTask")
	if fetched.ID != first.ID || fetched.Status.State != stateCompleted {
		t.Fatalf("GetTask returned %+v, want the completed task %s", fetched, first.ID)
	}
	if answer := fetched.artifactText(); answer != "matrix:first" {
		t.Fatalf("GetTask artifact = %q, want the routed answer", answer)
	}
	if len(fetched.History) != 1 || fetched.History[0].MessageID != "m-first" {
		t.Fatalf("GetTask history = %#v, want the request message", fetched.History)
	}

	callRPC(t, server.URL, "GetTask", `{"id":"no-such-task"}`).requireError(t, "GetTask", codeTaskNotFound)

	var listing struct {
		Tasks []taskBody `json:"tasks"`
		Next  string     `json:"nextPageToken"`
		Size  int        `json:"pageSize"`
		Total int        `json:"totalSize"`
	}
	list := callRPC(t, server.URL, "ListTasks", `{}`)
	if list.Error != nil {
		t.Fatalf("ListTasks: error %d: %s", list.Error.Code, list.Error.Message)
	}
	if err := json.Unmarshal(list.Result, &listing); err != nil {
		t.Fatalf("decode ListTasks: %v", err)
	}
	if listing.Total != 2 || len(listing.Tasks) != 2 {
		t.Fatalf("ListTasks returned %d of %d tasks, want both", len(listing.Tasks), listing.Total)
	}
	if listing.Size == 0 {
		t.Fatalf("ListTasks did not report a page size")
	}
	for _, task := range listing.Tasks {
		if len(task.Artifacts) != 0 {
			t.Fatalf("ListTasks included artifacts without includeArtifacts: %#v", task.Artifacts)
		}
	}

	withArtifacts := callRPC(t, server.URL, "ListTasks", fmt.Sprintf(`{"contextId":%q,"includeArtifacts":true}`, first.ContextID))
	if err := json.Unmarshal(withArtifacts.Result, &listing); err != nil {
		t.Fatalf("decode filtered ListTasks: %v", err)
	}
	if listing.Total != 1 || len(listing.Tasks) != 1 || listing.Tasks[0].ID != first.ID {
		t.Fatalf("context filter returned %+v, want only %s", listing.Tasks, first.ID)
	}
	if answer := listing.Tasks[0].artifactText(); answer != "matrix:first" {
		t.Fatalf("includeArtifacts returned %q, want the artifact content", answer)
	}
	if second.ContextID == first.ContextID {
		t.Fatalf("the two messages shared a context, so the filter proves nothing")
	}
}

// TestTasksCancelStopsTheTurnAndAnswersCanceled covers cancelation end to end: the
// request answers the canceled task, the turn the agent is running is actually
// canceled, repeating the request is idempotent, and the error codes for a task that
// cannot be canceled and for one that does not exist match the specification (§3.1.5,
// §3.3.1, §5.4).
func TestTasksCancelStopsTheTurnAndAnswersCanceled(t *testing.T) {
	router := newBlockingTurnRouter()
	server := newA2ATestServer(t, NewServer(router, "http://127.0.0.1:0", "opencode"))

	responses := make(chan rpcEnvelope, 1)
	go func() {
		envelope, _ := postRPC(server.URL, "SendMessage",
			`{"message":{"role":"user","messageId":"m-cancel","parts":[{"text":"long job"}]},"configuration":{"returnImmediately":true}}`)
		responses <- envelope
	}()
	select {
	case <-router.started:
	case <-time.After(5 * time.Second):
		t.Fatalf("the turn never reached the router")
	}
	running := (<-responses).requireTask(t, "SendMessage")
	if running.Status.State == stateCompleted {
		t.Fatalf("the send waited for the turn to finish: %q", running.Status.State)
	}

	canceled := callRPC(t, server.URL, "CancelTask", fmt.Sprintf(`{"id":%q}`, running.ID)).requireTask(t, "CancelTask")
	if canceled.Status.State != stateCanceled {
		t.Fatalf("CancelTask answered %q, want %q", canceled.Status.State, stateCanceled)
	}

	select {
	case err := <-router.turnDone:
		if err == nil {
			t.Fatalf("the agent turn kept running after the task was canceled")
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("the agent turn was not canceled")
	}

	again := callRPC(t, server.URL, "CancelTask", fmt.Sprintf(`{"id":%q}`, running.ID)).requireTask(t, "CancelTask")
	if again.Status.State != stateCanceled {
		t.Fatalf("canceling twice answered %q, want the canceled task (§3.3.1)", again.Status.State)
	}
}

// TestTasksCancelRefusesTerminalAndUnknownTasks pins the two error codes cancelation
// must answer: a task that already finished cannot be canceled and an unknown id does
// not exist (§3.1.5, §5.4).
func TestTasksCancelRefusesTerminalAndUnknownTasks(t *testing.T) {
	server := newA2ATestServer(t, NewServer(&stubSessionRouter{}, "http://127.0.0.1:0", "opencode"))

	done := callRPC(t, server.URL, "SendMessage", sendMessageParams("m-done", "short job")).requireTask(t, "SendMessage")
	if done.Status.State != stateCompleted {
		t.Fatalf("the turn did not finish: %q", done.Status.State)
	}
	callRPC(t, server.URL, "CancelTask", fmt.Sprintf(`{"id":%q}`, done.ID)).requireError(t, "CancelTask", codeTaskNotCancelable)
	callRPC(t, server.URL, "CancelTask", `{"id":"no-such-task"}`).requireError(t, "CancelTask", codeTaskNotFound)
}

// TestTasksResubscribeStreamsTheTaskAndItsTerminalUpdate covers the subscription
// operation: the stream starts with the Task object in its current state and ends when
// the task reaches a terminal state (§3.1.6).
func TestTasksResubscribeStreamsTheTaskAndItsTerminalUpdate(t *testing.T) {
	router := newBlockingTurnRouter()
	server := newA2ATestServer(t, NewServer(router, "http://127.0.0.1:0", "opencode"))

	responses := make(chan rpcEnvelope, 1)
	go func() {
		envelope, _ := postRPC(server.URL, "SendMessage",
			`{"message":{"role":"user","messageId":"m-resub","parts":[{"text":"long job"}]},"configuration":{"returnImmediately":true}}`)
		responses <- envelope
	}()
	select {
	case <-router.started:
	case <-time.After(5 * time.Second):
		t.Fatalf("the turn never reached the router")
	}
	running := (<-responses).requireTask(t, "SendMessage")

	frames := openStream(t, server.URL, "SubscribeToTask", fmt.Sprintf(`{"id":%q}`, running.ID))
	first := nextFrame(t, frames, "the task the subscription starts with").result(t)
	if first.Task == nil || first.Task.ID != running.ID {
		t.Fatalf("the subscription did not start with the current task: %+v", first)
	}
	if first.Task.Status.State == stateCompleted || first.Task.Status.State == stateCanceled {
		t.Fatalf("the subscription opened on a terminal task: %q", first.Task.Status.State)
	}

	callRPC(t, server.URL, "CancelTask", fmt.Sprintf(`{"id":%q}`, running.ID))

	terminal := nextFrame(t, frames, "the terminal status update").result(t)
	if terminal.StatusUpdate == nil || terminal.StatusUpdate.Status.State != stateCanceled {
		t.Fatalf("the subscription did not deliver the terminal status: %+v", terminal)
	}
	awaitStreamClosed(t, frames)
}

// TestPushNotificationMethodsFollowTheAdvertisedCapability covers all four push
// notification operations against both capability states. The specification is
// explicit in both directions: with the capability off every one of them MUST answer
// PushNotificationNotSupportedError, and with it on they must work (§3.3.4).
func TestPushNotificationMethodsFollowTheAdvertisedCapability(t *testing.T) {
	methods := []string{
		"CreateTaskPushNotificationConfig",
		"GetTaskPushNotificationConfig",
		"ListTaskPushNotificationConfigs",
		"DeleteTaskPushNotificationConfig",
	}

	t.Run("capability off", func(t *testing.T) {
		server := newA2ATestServer(t, NewServer(&stubSessionRouter{}, "http://127.0.0.1:0", "opencode"))
		if card := fetchCard(t, server.URL); card.Capabilities.PushNotifications {
			t.Fatalf("the card advertises push notifications the server has no store for")
		}
		for _, method := range methods {
			callRPC(t, server.URL, method, `{"taskId":"t-1","id":"c-1"}`).requireError(t, method, codePushNotSupported)
		}
	})

	t.Run("capability on", func(t *testing.T) {
		adapter := NewServer(&stubSessionRouter{}, "http://127.0.0.1:0", "opencode").
			WithPushNotifications(push.NewInMemoryStore(), &recordingPushSender{})
		server := newA2ATestServer(t, adapter)
		if card := fetchCard(t, server.URL); !card.Capabilities.PushNotifications {
			t.Fatalf("the card does not advertise the push notifications the server is configured for")
		}

		task := callRPC(t, server.URL, "SendMessage", sendMessageParams("m-push", "notify me")).requireTask(t, "SendMessage")

		created := callRPC(t, server.URL, "CreateTaskPushNotificationConfig",
			fmt.Sprintf(`{"taskId":%q,"url":"https://callback.example/a2a"}`, task.ID))
		if created.Error != nil {
			t.Fatalf("create: error %d: %s", created.Error.Code, created.Error.Message)
		}
		var config struct {
			ID     string `json:"id"`
			TaskID string `json:"taskId"`
			URL    string `json:"url"`
		}
		if err := json.Unmarshal(created.Result, &config); err != nil {
			t.Fatalf("decode created config: %v", err)
		}
		if config.ID == "" || config.TaskID != task.ID || config.URL != "https://callback.example/a2a" {
			t.Fatalf("created config = %+v, want it stored for %s", config, task.ID)
		}

		fetched := callRPC(t, server.URL, "GetTaskPushNotificationConfig",
			fmt.Sprintf(`{"taskId":%q,"id":%q}`, task.ID, config.ID))
		if fetched.Error != nil {
			t.Fatalf("get: error %d: %s", fetched.Error.Code, fetched.Error.Message)
		}
		var stored struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(fetched.Result, &stored); err != nil || stored.ID != config.ID {
			t.Fatalf("get returned %s (err %v), want config %s", fetched.Result, err, config.ID)
		}

		listed := callRPC(t, server.URL, "ListTaskPushNotificationConfigs", fmt.Sprintf(`{"taskId":%q}`, task.ID))
		if listed.Error != nil {
			t.Fatalf("list: error %d: %s", listed.Error.Code, listed.Error.Message)
		}
		var listing struct {
			Configs []struct {
				ID string `json:"id"`
			} `json:"configs"`
		}
		if err := json.Unmarshal(listed.Result, &listing); err != nil || len(listing.Configs) != 1 {
			t.Fatalf("list returned %s (err %v), want one config", listed.Result, err)
		}

		deleted := callRPC(t, server.URL, "DeleteTaskPushNotificationConfig",
			fmt.Sprintf(`{"taskId":%q,"id":%q}`, task.ID, config.ID))
		if deleted.Error != nil {
			t.Fatalf("delete: error %d: %s", deleted.Error.Code, deleted.Error.Message)
		}
		gone := callRPC(t, server.URL, "ListTaskPushNotificationConfigs", fmt.Sprintf(`{"taskId":%q}`, task.ID))
		if gone.Error != nil {
			t.Fatalf("list after delete: error %d: %s", gone.Error.Code, gone.Error.Message)
		}
		if err := json.Unmarshal(gone.Result, &listing); err != nil || len(listing.Configs) != 0 {
			t.Fatalf("list after delete returned %s, want no configs", gone.Result)
		}
	})
}

// TestExtendedAgentCardFollowsTheAdvertisedCapability covers the eleventh operation in
// both directions: unadvertised it MUST answer UnsupportedOperationError, advertised it
// must return the extended card (§3.1.11, §3.3.4).
func TestExtendedAgentCardFollowsTheAdvertisedCapability(t *testing.T) {
	t.Run("capability off", func(t *testing.T) {
		server := newA2ATestServer(t, NewServer(&stubSessionRouter{}, "http://127.0.0.1:0", "opencode"))
		if card := fetchCard(t, server.URL); card.Capabilities.ExtendedAgentCard {
			t.Fatalf("the card advertises an extended card the server has none of")
		}
		callRPC(t, server.URL, "GetExtendedAgentCard", `{}`).requireError(t, "GetExtendedAgentCard", codeUnsupportedOp)
	})

	t.Run("capability on", func(t *testing.T) {
		extended := &a2asdk.AgentCard{
			Name:        "Matrix",
			Description: "authenticated profile",
			Version:     "2",
			Capabilities: a2asdk.AgentCapabilities{
				Streaming: true,
			},
			SupportedInterfaces: []*a2asdk.AgentInterface{
				a2asdk.NewAgentInterface("http://127.0.0.1:0/a2a", a2asdk.TransportProtocolJSONRPC),
			},
		}
		adapter := NewServer(&stubSessionRouter{}, "http://127.0.0.1:0", "opencode").WithExtendedAgentCard(extended)
		server := newA2ATestServer(t, adapter)
		if card := fetchCard(t, server.URL); !card.Capabilities.ExtendedAgentCard {
			t.Fatalf("the card does not advertise the extended card the server serves")
		}

		answer := callRPC(t, server.URL, "GetExtendedAgentCard", `{}`)
		if answer.Error != nil {
			t.Fatalf("GetExtendedAgentCard: error %d: %s", answer.Error.Code, answer.Error.Message)
		}
		var card a2asdk.AgentCard
		if err := json.Unmarshal(answer.Result, &card); err != nil {
			t.Fatalf("decode extended card: %v", err)
		}
		if card.Description != "authenticated profile" {
			t.Fatalf("extended card = %+v, want the configured card", card)
		}
	})
}

// recordingPushSender is a push.Sender that records what would have been delivered, so
// the push configuration operations can be driven without an outbound webhook.
type recordingPushSender struct {
	sent []*a2asdk.PushConfig
}

func (s *recordingPushSender) SendPush(_ context.Context, config *a2asdk.PushConfig, _ a2asdk.Event) error {
	s.sent = append(s.sent, config)
	return nil
}

// TestTheAdvertisedRESTBindingServesTheSameOperations checks the second interface the
// card publishes (HTTP+JSON at /a2a/rest) against the same operations, because §5.1
// requires every advertised binding to provide the same set of operations and to map
// errors consistently. Where the JSON-RPC binding answers an A2A error code, this
// binding answers the HTTP status §5.4 fixes for that error type.
func TestTheAdvertisedRESTBindingServesTheSameOperations(t *testing.T) {
	server := newA2ATestServer(t, NewServer(&stubSessionRouter{}, "http://127.0.0.1:0", "opencode"))

	posted := restRequest(t, http.MethodPost, server.URL+"/a2a/rest/message:send",
		`{"message":{"role":"user","messageId":"m-rest","parts":[{"text":"rest please"}]}}`)
	if posted.status != http.StatusOK {
		t.Fatalf("REST message:send status = %d, body %s", posted.status, posted.body)
	}
	var sent struct {
		Task taskBody `json:"task"`
	}
	if err := json.Unmarshal(posted.body, &sent); err != nil {
		t.Fatalf("decode REST task: %v", err)
	}
	if sent.Task.Status.State != stateCompleted || sent.Task.artifactText() != "matrix:rest please" {
		t.Fatalf("REST message:send answered %+v, want a completed task with the answer", sent.Task)
	}

	fetched := restRequest(t, http.MethodGet, server.URL+"/a2a/rest/tasks/"+sent.Task.ID, "")
	if fetched.status != http.StatusOK {
		t.Fatalf("REST tasks/{id} status = %d, body %s", fetched.status, fetched.body)
	}
	var one taskBody
	if err := json.Unmarshal(fetched.body, &one); err != nil {
		t.Fatalf("decode REST get: %v", err)
	}
	if one.Status.State != stateCompleted || one.artifactText() != "matrix:rest please" {
		t.Fatalf("REST tasks/{id} answered %+v, want the stored task", one)
	}

	listed := restRequest(t, http.MethodGet, server.URL+"/a2a/rest/tasks", "")
	if listed.status != http.StatusOK {
		t.Fatalf("REST tasks status = %d, body %s", listed.status, listed.body)
	}
	var listing struct {
		Tasks []taskBody `json:"tasks"`
	}
	if err := json.Unmarshal(listed.body, &listing); err != nil || len(listing.Tasks) != 1 {
		t.Fatalf("REST tasks answered %s (err %v), want the one task", listed.body, err)
	}

	// §5.4: TaskNotCancelable, PushNotificationNotSupported and UnsupportedOperation all
	// map to HTTP 400 on this binding.
	notCancelable := restRequest(t, http.MethodPost, server.URL+"/a2a/rest/tasks/"+sent.Task.ID+":cancel", "")
	if notCancelable.status != http.StatusBadRequest {
		t.Fatalf("REST cancel of a completed task status = %d, want 400", notCancelable.status)
	}
	pushRefused := restRequest(t, http.MethodPost, server.URL+"/a2a/rest/tasks/"+sent.Task.ID+"/pushNotificationConfigs",
		`{"taskId":"`+sent.Task.ID+`","url":"https://callback.example/a2a"}`)
	if pushRefused.status != http.StatusBadRequest {
		t.Fatalf("REST push config status = %d, want 400 for the unsupported capability", pushRefused.status)
	}
	extendedRefused := restRequest(t, http.MethodGet, server.URL+"/a2a/rest/extendedAgentCard", "")
	if extendedRefused.status != http.StatusBadRequest {
		t.Fatalf("REST extendedAgentCard status = %d, want 400 for the unsupported capability", extendedRefused.status)
	}
}

// TestTerminalStateErrorsAreTheProtocolSDKsOwn records two places where the error a
// caller receives does not match the specification's letter, so that a future upgrade of
// the protocol SDK which changes them is noticed rather than silently absorbed:
//
//   - Sending a message to a task in a terminal state answers -32602 (invalid params)
//     where §3.1.1 specifies UnsupportedOperationError (-32004).
//   - Subscribing to a task in a terminal state answers -32001 (task not found) where
//     §9.4.6 specifies UnsupportedOperationError (-32004).
//
// Both decisions are made inside the SDK's request handler before Matrix's executor is
// reached: a2asrv.factory.loadExecutionContext rejects the terminal task with
// a2a.ErrInvalidParams, and taskexec.localManager.Resubscribe has no execution left to
// attach to, which the handler wraps in a2a.ErrTaskNotFound. Remapping them would mean
// replacing the SDK's RequestHandler for both transports - a much larger change than
// this fix - and the operation is refused either way, so no caller is told the request
// succeeded.
func TestTerminalStateErrorsAreTheProtocolSDKsOwn(t *testing.T) {
	server := newA2ATestServer(t, NewServer(&stubSessionRouter{}, "http://127.0.0.1:0", "opencode"))

	done := callRPC(t, server.URL, "SendMessage", sendMessageParams("m-terminal", "done")).requireTask(t, "SendMessage")
	if done.Status.State != stateCompleted {
		t.Fatalf("the turn did not finish: %q", done.Status.State)
	}

	followUp := fmt.Sprintf(`{"message":{"role":"user","messageId":"m-follow-up","taskId":%q,"parts":[{"text":"more"}]}}`, done.ID)
	callRPC(t, server.URL, "SendMessage", followUp).requireError(t, "SendMessage", codeInvalidParams)

	frames := openStream(t, server.URL, "SubscribeToTask", fmt.Sprintf(`{"id":%q}`, done.ID))
	frame := nextFrame(t, frames, "the subscription answer")
	if frame.Error == nil || frame.Error.Code != codeTaskNotFound {
		t.Fatalf("SubscribeToTask on a terminal task answered %+v, want the SDK's task-not-found error", frame)
	}
}

type restAnswer struct {
	status int
	body   []byte
}

func restRequest(t *testing.T, method, url, body string) restAnswer {
	t.Helper()
	var payload io.Reader
	if body != "" {
		payload = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, url, payload)
	if err != nil {
		t.Fatalf("build %s %s: %v", method, url, err)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	answer, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read %s %s: %v", method, url, err)
	}
	return restAnswer{status: resp.StatusCode, body: answer}
}

// TestNoAdvertisedMethodNameAnswersMethodNotFound walks every name in the translation
// table and asserts that not one of them is rejected as an unknown method. That is the
// defect the issue records: the route was wired, the card advertised the interface, and
// every method a client could send answered -32601, so the surface was unusable while
// looking healthy. The behaviour of each method is asserted by the tests above; this one
// guards the dispatch table itself, including the entries the per-method tests do not
// drive by their legacy name.
func TestNoAdvertisedMethodNameAnswersMethodNotFound(t *testing.T) {
	server := newA2ATestServer(t, NewServer(&stubSessionRouter{}, "http://127.0.0.1:0", "opencode"))

	for name := range specJSONRPCMethodNames {
		t.Run(name, func(t *testing.T) {
			body := fmt.Sprintf(`{"jsonrpc":"2.0","id":"table-check","method":%q,"params":{}}`, name)
			resp, err := http.Post(server.URL+"/a2a", "application/json", strings.NewReader(body))
			if err != nil {
				t.Fatalf("post %s: %v", name, err)
			}
			defer func() { _ = resp.Body.Close() }()
			payload, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatalf("read %s: %v", name, err)
			}
			if len(payload) == 0 {
				t.Fatalf("%s answered an empty body", name)
			}
			if strings.Contains(string(payload), `"code":-32601`) {
				t.Fatalf("%s was rejected as an unknown method: %s", name, payload)
			}
		})
	}
}
