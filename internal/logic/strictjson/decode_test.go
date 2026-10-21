package strictjson

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

type document struct {
	Text   string `json:"text"`
	Nested []struct {
		Text string `json:"text"`
	} `json:"nested"`
	Meta map[string]any `json:"meta"`
}

func TestDecodeRejectsAmbiguousDocuments(t *testing.T) {
	for _, raw := range []string{
		`{"text":"a","text":"b"}`, `{"text":"a","Text":"b"}`, `{"Text":"a"}`,
		`{"nested":[{"text":"a","text":"b"}]}`, `{"meta":{"x":1,"x":2}}`,
		`{"nested":[{"unknown":1}]}`, `{"unknown":true}`, `{"text":"a"}{}`, `{"text":"a"}null`,
		strings.Repeat("[", MaxDepth+2) + strings.Repeat("]", MaxDepth+2),
	} {
		var result document
		if err := Decode(strings.NewReader(raw), &result); err == nil {
			t.Fatalf("accepted %s", raw)
		}
	}
}

func TestDecodePreservesTypedErrorsAndFreeMapKeys(t *testing.T) {
	var result document
	if err := Decode(strings.NewReader(`{"text":"ok","nested":[{"text":"child"}],"meta":{"A":1,"a":2}}`), &result); err != nil {
		t.Fatal(err)
	}
	if result.Text != "ok" || len(result.Meta) != 2 {
		t.Fatal(result)
	}
	var target any
	if err := Decode(strings.NewReader(`{} {}`), &target); !errors.Is(err, ErrTrailing) {
		t.Fatal(err)
	}
	var wrong document
	err := Decode(strings.NewReader(`{"text":7}`), &wrong)
	var mismatch *json.UnmarshalTypeError
	if !errors.As(err, &mismatch) {
		t.Fatalf("typed error lost: %v", err)
	}
}

func FuzzDecode(f *testing.F) {
	for _, seed := range []string{`{}`, `{"text":"x"}`, `{"text":"x","text":"y"}`, `{"meta":[{}]}`} {
		f.Add(seed)
	}
	f.Fuzz(func(_ *testing.T, raw string) { var result document; _ = Decode(strings.NewReader(raw), &result) })
}
