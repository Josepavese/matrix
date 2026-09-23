package a2a

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
)

// specJSONRPCMethodNames maps the method names of the A2A 0.3 JSON-RPC binding onto
// the names the protocol SDK dispatches.
//
// A note on which generation is which, because the two are easy to confuse:
//
//   - The A2A 1.0 specification - the version this card publishes on every interface -
//     names the JSON-RPC methods in PascalCase: SendMessage, SendStreamingMessage,
//     GetTask, ListTasks, CancelTask, SubscribeToTask, CreateTaskPushNotificationConfig,
//     GetTaskPushNotificationConfig, ListTaskPushNotificationConfigs,
//     DeleteTaskPushNotificationConfig, GetExtendedAgentCard (specification §5.3
//     "Method Mapping Reference", §9.4 "Core Methods"). Those are exactly the
//     identifiers the SDK dispatches, so a 1.0 client needs no translation at all.
//   - A2A 0.3 named them "category/action": message/send, tasks/get,
//     tasks/pushNotificationConfig/set, agent/getAuthenticatedExtendedCard, ...
//
// The slash-separated names are accepted for callers written against that older
// generation: a real client that talked to this daemon sent message/send and received
// "-32601 method not found" for every method it could send, because nothing translated
// the older name onto the one the handler dispatches. The names are translated on the
// way in; responses need no translation because they carry no method name.
var specJSONRPCMethodNames = map[string]string{
	"message/send":                        "SendMessage",
	"message/stream":                      "SendStreamingMessage",
	"tasks/get":                           "GetTask",
	"tasks/list":                          "ListTasks",
	"tasks/cancel":                        "CancelTask",
	"tasks/resubscribe":                   "SubscribeToTask",
	"tasks/pushNotificationConfig/get":    "GetTaskPushNotificationConfig",
	"tasks/pushNotificationConfig/set":    "CreateTaskPushNotificationConfig",
	"tasks/pushNotificationConfig/list":   "ListTaskPushNotificationConfigs",
	"tasks/pushNotificationConfig/delete": "DeleteTaskPushNotificationConfig",
	"agent/getAuthenticatedExtendedCard":  "GetExtendedAgentCard",
	"agent/authenticatedExtendedCard":     "GetExtendedAgentCard",
}

// withSpecJSONRPCMethodNames rewrites an incoming legacy (0.3-generation) JSON-RPC
// method name to the one the SDK dispatches, leaving everything else about the request
// untouched. A 1.0 name, a body that is not a single JSON-RPC request, or a name no
// table entry matches is passed through unchanged so nothing is lost on the way to the
// handler.
func withSpecJSONRPCMethodNames(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Body == nil {
			next.ServeHTTP(w, r)
			return
		}
		body, err := io.ReadAll(r.Body)
		_ = r.Body.Close()
		if err != nil {
			next.ServeHTTP(w, r)
			return
		}
		rewritten := rewriteSpecMethodName(body)
		r.Body = io.NopCloser(bytes.NewReader(rewritten))
		r.ContentLength = int64(len(rewritten))
		next.ServeHTTP(w, r)
	})
}

// rewriteSpecMethodName returns the body with a spec method name replaced by the SDK's
// name. It works on the decoded object rather than on the text so that a "method" key
// inside params cannot be mistaken for the request's own method.
func rewriteSpecMethodName(body []byte) []byte {
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(body, &payload); err != nil {
		return body
	}
	rawMethod, ok := payload["method"]
	if !ok {
		return body
	}
	var method string
	if err := json.Unmarshal(rawMethod, &method); err != nil {
		return body
	}
	replacement, ok := specJSONRPCMethodNames[method]
	if !ok {
		return body
	}
	encoded, err := json.Marshal(replacement)
	if err != nil {
		return body
	}
	payload["method"] = encoded
	rewritten, err := json.Marshal(payload)
	if err != nil {
		return body
	}
	return rewritten
}
