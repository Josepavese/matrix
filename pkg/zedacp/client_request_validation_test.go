package zedacp

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"
)

func TestClientRequestsRejectMalformedActionableValues(t *testing.T) {
	path, _ := json.Marshal(filepath.Join(t.TempDir(), "file.txt"))
	for _, tc := range []struct{ method, fields string }{
		{"fs/write_text_file", `"content":null`},
		{"fs/write_text_file", `"content":42`},
		{"fs/write_text_file", `"_meta":{}`},
		{"fs/read_text_file", `"line":-1`},
		{"fs/read_text_file", `"limit":4294967296`},
		{"terminal/create", `"command":"echo","args":[null]`},
		{"terminal/create", `"command":"echo","env":[{"name":"x","value":null}]`},
		{"terminal/create", `"command":"echo","outputByteLimit":-1`},
		{"session/request_permission", `"toolCall":{"toolCallId":"t"},"options":[{"optionId":null,"name":"Allow","kind":"allow_once"}]`},
	} {
		raw := json.RawMessage(`{"sessionId":"s","path":` + string(path) + `,` + tc.fields + `}`)
		err := ValidateClientRequest(tc.method, raw, 1)
		var rpc *RPCError
		if !errors.As(err, &rpc) || rpc.Code != -32602 {
			t.Fatalf("%s malformed request acted on: %s: %v", tc.method, raw, err)
		}
	}
	if err := ValidateClientRequest("fs/write_text_file", json.RawMessage(`{"sessionId":"s","path":`+string(path)+`,"content":""}`), 1); err != nil {
		t.Fatalf("valid empty file write rejected: %v", err)
	}
	if err := ValidateClientRequest("terminal/create", json.RawMessage(`{"sessionId":"s","command":"echo"}`), 2); err == nil {
		t.Fatal("v2 must reject retired client terminal methods")
	}
}
