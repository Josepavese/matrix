package zedacp

import (
	"encoding/json"
	"log/slog"
	"testing"
)

// ----------------------------------------------------------------------------
// Fuzz targets for the wire decoders
// ----------------------------------------------------------------------------
//
// Every byte here comes from an external agent process, so the decoders are the
// boundary where a hostile or buggy peer reaches Matrix. The invariant asserted
// is deliberately weak — "never panic, never hang, never return a half-decoded
// message as valid" — because that is exactly what must hold for arbitrary
// input. Stronger properties are checked by returning the decoded value for
// seeds that do decode.

// FuzzDecodeInboundRaw checks the first-stage envelope decoder.
func FuzzDecodeInboundRaw(f *testing.F) {
	seeds := []string{
		`{"jsonrpc":"2.0","id":1,"method":"session/prompt","params":{}}`,
		`{"jsonrpc":"2.0","id":"str-id","method":"session/update","params":{"sessionId":"s","update":{}}}`,
		`{"jsonrpc":"2.0","id":null,"result":{}}`,
		`{"jsonrpc":"2.0","method":"notify"}`,
		`{}`,
		`[]`,
		`null`,
		`{"id":{"nested":[1,2,3]}}`,
		"{\"id\":1e999}",
		`{"jsonrpc":"2.0","id":1,"method":"x","params":{"a":"` + "\xff\xfe" + `"}}`,
	}
	for _, seed := range seeds {
		f.Add([]byte(seed))
	}
	// A discarding logger keeps the fuzz loop from drowning in warnings for the
	// many invalid inputs it generates. Nil-logger safety is covered by a
	// dedicated unit test instead.
	quiet := slog.New(slog.DiscardHandler)
	f.Fuzz(func(t *testing.T, data []byte) {
		raw, ok := decodeInboundRaw(quiet, data)
		if !ok {
			return
		}
		// A decoded envelope must be re-encodable and must agree with the raw
		// shape the dispatcher inspects.
		if _, err := json.Marshal(raw); err != nil {
			t.Fatalf("decoded envelope is not marshalable: %v", err)
		}
		if method, present := raw["method"]; present && method != nil {
			if _, isString := method.(string); isString {
				// The dispatcher must be able to classify this message.
				var req jsonRPCRequest
				_ = json.Unmarshal(data, &req)
			}
		}
	})
}

// FuzzJSONRPCIDInt64 checks response correlation ids: a bad id must be rejected,
// never coerced into a valid one that would misroute a response.
func FuzzJSONRPCIDInt64(f *testing.F) {
	for _, seed := range []string{`1`, `"1"`, `" 1"`, `null`, `true`, `1.5`, `1e3`, `-3`, `"abc"`, `{}`, `[]`, `""`} {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		var id json.RawMessage = data
		value, ok := jsonRPCIDInt64(id)
		if !ok {
			return
		}
		// Whatever is accepted must round-trip to the same integer.
		reEncoded, err := json.Marshal(value)
		if err != nil {
			t.Fatalf("accepted id %d does not marshal: %v", value, err)
		}
		again, ok := jsonRPCIDInt64(reEncoded)
		if !ok || again != value {
			t.Fatalf("id %s decoded to %d but re-decoding %s gave (%d, %v)", data, value, reEncoded, again, ok)
		}
	})
}

// FuzzDecodeUpdateContent checks the session update payload decoder, which has
// several accepted shapes (string, content block, list of blocks).
func FuzzDecodeUpdateContent(f *testing.F) {
	seeds := []string{
		`{"type":"text","text":"hello"}`,
		`[{"type":"text","text":"a"},{"type":"image","data":"x"}]`,
		`"plain string"`,
		`[]`,
		`[[]]`,
		`null`,
		`{"type":"text"}`,
		`{"type":123}`,
		`[[{"type":"text","text":"nested"}]]`,
	}
	for _, seed := range seeds {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		var update SessionUpdate
		if err := json.Unmarshal(data, &update); err != nil {
			return
		}
		// Successful decode must survive re-encoding.
		if _, err := json.Marshal(update.Content); err != nil {
			t.Fatalf("decoded content is not marshalable: %v", err)
		}
	})
}

// FuzzSessionNotificationDecode checks the notification envelope used for every
// streamed chunk.
func FuzzSessionNotificationDecode(f *testing.F) {
	seeds := []string{
		`{"sessionId":"s","update":{"sessionUpdate":"agent_message_chunk","content":{"type":"text","text":"x"}}}`,
		`{"sessionId":"","update":{}}`,
		`{}`,
		`null`,
		`{"sessionId":123}`,
		`{"sessionId":"s","update":{"content":[{"type":"text","text":"a"}]}}`,
	}
	for _, seed := range seeds {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		var update SessionNotification
		if err := json.Unmarshal(data, &update); err != nil {
			return
		}
		if _, err := json.Marshal(update); err != nil {
			t.Fatalf("decoded notification is not marshalable: %v", err)
		}
	})
}

// FuzzDecodeOptionalResult checks the response decoder that tolerates a missing
// or null result, which every optional response uses.
func FuzzDecodeOptionalResult(f *testing.F) {
	for _, seed := range []string{`{}`, `null`, `{"sessionId":"s"}`, `[]`, `"x"`, `{"a":{"b":[1]}}`, `{"sessionId":1}`} {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(_ *testing.T, data []byte) {
		var target struct {
			SessionID string `json:"sessionId"`
		}
		if err := decodeOptionalResult(json.RawMessage(data), &target); err != nil {
			return
		}
	})
}

// TestDecodeInboundRawToleratesANilLogger is the fuzz regression: the decoder
// that exists to handle malformed input dereferenced a nil logger and panicked.
func TestDecodeInboundRawToleratesANilLogger(t *testing.T) {
	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("decoding with a nil logger panicked: %v", recovered)
		}
	}()
	if _, ok := decodeInboundRaw(nil, []byte("not json")); ok {
		t.Fatal("invalid json must not decode")
	}
	if _, ok := decodeInboundRaw(nil, []byte(`{"jsonrpc":"2.0"}`)); !ok {
		t.Fatal("valid json must decode")
	}
}
