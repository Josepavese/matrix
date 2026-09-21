package telegram

import (
	"net/url"
	"strings"
	"testing"

	"github.com/Josepavese/matrix/internal/middleware"
)

// ----------------------------------------------------------------------------
// Fuzz targets for the chat callback decoder
// ----------------------------------------------------------------------------
//
// Callback data is attacker-controlled: anyone in the chat can craft a payload,
// and the decoder decides which pending answer (if any) a tap resolves.

// FuzzDecodeCallbackData asserts the decoder is total and that anything it
// accepts survives re-encoding into an equivalent payload.
func FuzzDecodeCallbackData(f *testing.F) {
	seeds := []string{
		"el:tok:pick:0:0",
		"el:tok:confirm:0:0",
		"el:tok:decline:0:0",
		"el:tok:cancel:0:0",
		"el:tok:url-accept:0:0",
		"el:tok:pick:-1:-1",
		"el:tok:pick:99999999999999999999:0",
		"el::pick:0:0",
		"el:tok:pick:0",
		"el:tok:pick:0:0:0",
		"",
		"x",
		"el:tok:pick:a:b",
		"el:tok:pick: 1: 1",
		"el:tok:p\x00ick:0:0",
		strings.Repeat("a", 4096),
	}
	for _, seed := range seeds {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, data string) {
		payload, ok := decodeCallbackData(data)
		if !ok {
			return
		}
		// An accepted payload must carry a non-empty token: an empty token can
		// never identify a pending request.
		if strings.TrimSpace(payload.token) == "" {
			t.Fatalf("accepted a callback payload with an empty token: %q", data)
		}
		// A decoded payload must re-encode to something that decodes back to the
		// same routing fields. optionValue is not transported (it is looked up
		// from the schema by index), so it is not part of the round trip.
		reEncoded := payload.encode()
		again, ok := decodeCallbackData(reEncoded)
		if !ok {
			t.Fatalf("re-encoded payload %q does not decode", reEncoded)
		}
		if again.token != payload.token || again.action != payload.action ||
			again.fieldIndex != payload.fieldIndex || again.optionIndex != payload.optionIndex {
			t.Fatalf("payload %q round-tripped to %q (%+v vs %+v)", data, reEncoded, again, payload)
		}
		// The 64-byte Telegram limit is a property of the generator, not of the
		// decoder: arbitrary input may carry any token length, and rejecting a
		// long token here would be wrong. It is asserted over real tokens in
		// TestGeneratedCallbackDataFitsTelegramLimit.
	})
}

// TestGeneratedCallbackDataFitsTelegramLimit checks the invariant where it
// belongs: every payload this frontend can generate must fit the 64-byte
// callback data limit Telegram enforces.
func TestGeneratedCallbackDataFitsTelegramLimit(t *testing.T) {
	tokens := []string{"e1", "e42", "e999999"}
	actions := []string{"pick", "confirm", "decline", "cancel", "url-accept"}
	indices := []int{0, 1, 9, 100, 9999}
	for _, token := range tokens {
		for _, action := range actions {
			for _, fieldIndex := range indices {
				for _, optionIndex := range indices {
					encoded := callbackPayload{
						token: token, action: action, fieldIndex: fieldIndex, optionIndex: optionIndex,
					}.encode()
					if len(encoded) > 64 {
						t.Fatalf("generated callback exceeds 64 bytes: %q (%d)", encoded, len(encoded))
					}
				}
			}
		}
	}
}

// FuzzCoerceElicitationValue asserts that a parsed answer always has the JSON
// type the schema declares — the invariant that broke a Telegram boolean field.
func FuzzCoerceElicitationValue(f *testing.F) {
	for _, seed := range []string{"42", "-1", "1e999", "NaN", "Inf", "true", "false", "", " 7 ", "0x10", "1,5"} {
		f.Add(seed)
	}
	fields := []middleware.ElicitationField{
		{Name: "s", Type: "string"},
		{Name: "n", Type: "number"},
		{Name: "b", Type: "boolean"},
	}
	f.Fuzz(func(t *testing.T, raw string) {
		for _, field := range fields {
			value, err := coerceElicitationValue(field, raw)
			if err != nil {
				continue
			}
			// The guarantee is end-to-end: whatever the coercion produces must
			// either have the declared type or be refused by the shared
			// validator. A wrong-typed value that the validator accepts would
			// reach the agent.
			request := middleware.ElicitationRequest{
				Mode:   middleware.ElicitationModeForm,
				Fields: []middleware.ElicitationField{field},
			}
			if err := middleware.ValidateElicitationValues(request, map[string]interface{}{field.Name: value}); err != nil {
				continue
			}
			switch field.Type {
			case "number":
				number, ok := value.(float64)
				if !ok {
					t.Fatalf("a validated number field produced %T", value)
				}
				if number != number || number > 1e308 || number < -1e308 {
					t.Fatalf("number field produced a non-encodable value %v", number)
				}
			case "boolean":
				if _, ok := value.(bool); !ok {
					t.Fatalf("a validated boolean field produced %T", value)
				}
			case "string":
				if _, ok := value.(string); !ok {
					t.Fatalf("a validated string field produced %T", value)
				}
			}
		}
	})
}

// FuzzRedactURLUserinfo asserts the display path never leaks a password, for any
// URL an agent can put in a request.
func FuzzRedactURLUserinfo(f *testing.F) {
	seeds := []string{
		"https://user:secret@example.com/path",
		"https://example.com/path",
		"https://user@example.com",
		"http://a:b@c.d/e?f=g#h",
		"javascript:alert(1)",
		"data:text/html,<script>",
		"not a url",
		"",
		"https://:pass@example.com",
		"https://user:pass@example.com:8443/p?q=1",
	}
	for _, seed := range seeds {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, raw string) {
		redacted := redactURLUserinfo(raw)
		parsed, err := url.Parse(redacted)
		if err != nil {
			// Unparseable input is passed through unchanged: it must still be the
			// original, never a half-rewritten string.
			if redacted != raw {
				t.Fatalf("unparseable url was altered: %q -> %q", raw, redacted)
			}
			return
		}
		if parsed.User != nil {
			if password, hasPassword := parsed.User.Password(); hasPassword && password != "redacted" && password != "" {
				t.Fatalf("credentials leaked in display url: %q", redacted)
			}
		}
	})
}
