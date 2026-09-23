package a2a

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
)

// specJSONRPCMethodNames maps the method names the A2A specification defines for the
// JSON-RPC binding onto the names the protocol SDK dispatches.
//
// Why this exists: the SDK advertises these as "JSON-RPC method names per A2A spec
// §7" but dispatches PascalCase identifiers (SendMessage, GetTask, ...), while the
// specification's JSON-RPC binding - the one the agent card publishes as
// protocolVersion 1.0 with protocolBinding JSONRPC - uses slash-separated names
// (message/send, tasks/get, ...). A spec-conformant client therefore received
// "-32601 method not found" for every method it could send, which made the advertised
// interface unusable even though the handler behind it was wired correctly. The names
// are translated on the way in; responses need no translation because they carry no
// method name.
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

// withSpecJSONRPCMethodNames rewrites an incoming JSON-RPC method name to the one the
// SDK dispatches, leaving everything else about the request untouched. A body that is
// not a single JSON-RPC request, or that already carries a name the SDK knows, is
// passed through unchanged so nothing is lost on the way to the handler.
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
