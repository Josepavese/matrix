package main

import (
	"bufio"
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"
)

// captureStdout runs fn with os.Stdout redirected into a pipe and returns what
// was written. The mock agent answers the client by writing notifications to
// stdout, so this is the only way to assert the peer-visible behaviour.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	original := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = writer
	done := make(chan string, 1)
	go func() {
		data, _ := io.ReadAll(reader)
		done <- string(data)
	}()
	fn()
	_ = writer.Close()
	os.Stdout = original
	return <-done
}

func scannerOver(text string) *bufio.Scanner {
	return bufio.NewScanner(strings.NewReader(text))
}

// TestHandleRequestAnswersTheSessionHandshake pins the protocol surface the
// interop tests rely on: if the mock stops answering initialize or session/new,
// every end-to-end test would fail for the wrong reason.
func TestHandleRequestAnswersTheSessionHandshake(t *testing.T) {
	resp, ok := handleRequest(jsonRPCRequest{ID: 1, Method: "initialize"}, scannerOver(""))
	if !ok {
		t.Fatal("initialize must be answered")
	}
	var initResult map[string]interface{}
	if err := json.Unmarshal(resp.Result, &initResult); err != nil {
		t.Fatalf("initialize result: %v", err)
	}
	if initResult["protocolVersion"] != float64(1) {
		t.Fatalf("the mock must advertise protocol version 1, got %v", initResult["protocolVersion"])
	}

	resp, ok = handleRequest(jsonRPCRequest{ID: 2, Method: "session/new"}, scannerOver(""))
	if !ok {
		t.Fatal("session/new must be answered")
	}
	var newResult map[string]interface{}
	if err := json.Unmarshal(resp.Result, &newResult); err != nil {
		t.Fatalf("session/new result: %v", err)
	}
	if newResult["sessionId"] != "mock-session-id" {
		t.Fatalf("unexpected session id: %v", newResult["sessionId"])
	}
}

// TestHandleRequestLeavesTheIdEchoed keeps request correlation intact: a response
// without the caller's id cannot be matched to a request.
func TestHandleRequestLeavesTheIdEchoed(t *testing.T) {
	resp, _ := handleRequest(jsonRPCRequest{ID: "abc-123", Method: "session/new"}, scannerOver(""))
	if resp.ID != "abc-123" || resp.JSONRPC != "2.0" {
		t.Fatalf("the response must echo the id and the version: %+v", resp)
	}
	// An unknown method must not be answered with a result, which would look
	// like success to a caller probing an unsupported surface.
	resp, ok := handleRequest(jsonRPCRequest{ID: 3, Method: "session/unknown"}, scannerOver(""))
	if !ok {
		t.Fatal("an unknown method is not a transport failure")
	}
	if len(resp.Result) != 0 {
		t.Fatalf("an unknown method must not return a result, got %s", resp.Result)
	}
}

// TestHandlePromptRejectsMalformedParams keeps a broken request from being
// treated as a prompt with no text.
func TestHandlePromptRejectsMalformedParams(t *testing.T) {
	if _, ok := handlePrompt(jsonRPCRequest{Params: json.RawMessage(`{"prompt":`)}, scannerOver("")); ok {
		t.Fatal("malformed params must not be answered as a prompt")
	}
}

// TestHandlePromptEmitsAMessageForAnOrdinaryTurn covers the default path an
// interop test asserts: the peer sends a message notification and stops.
func TestHandlePromptEmitsAMessageForAnOrdinaryTurn(t *testing.T) {
	params := json.RawMessage(`{"sessionId":"s1","prompt":[{"text":"ciao"}]}`)
	var result json.RawMessage
	var ok bool
	output := captureStdout(t, func() {
		result, ok = handlePrompt(jsonRPCRequest{Params: params}, scannerOver(""))
	})
	if !ok {
		t.Fatal("an ordinary prompt must be answered")
	}
	var stop map[string]interface{}
	if err := json.Unmarshal(result, &stop); err != nil {
		t.Fatalf("prompt result: %v", err)
	}
	if stop["stopReason"] != "end_turn" {
		t.Fatalf("the mock must end the turn, got %v", stop["stopReason"])
	}
	if !strings.Contains(output, "session/update") || !strings.Contains(output, "s1") {
		t.Fatalf("the notification must name the session, got:\n%s", output)
	}
}

// TestHandlePromptElicitationRoundTrip is the interesting path: the mock asks the
// client, reads the answer, and reports the action it received. This is the only
// place where the elicitation contract is exercised from the peer side.
func TestHandlePromptElicitationRoundTrip(t *testing.T) {
	params := json.RawMessage(`{"sessionId":"s2","prompt":[{"text":"__ELICITATION_TEST__"}]}`)
	// The scripted client answer: a well-formed accept with a chosen option.
	clientReply := `{"jsonrpc":"2.0","id":200,"result":{"action":"accept","content":{"db":"sqlite"}}}` + "\n"

	output := captureStdout(t, func() {
		if _, ok := handlePrompt(jsonRPCRequest{Params: params}, scannerOver(clientReply)); !ok {
			t.Error("the elicitation prompt must be answered")
		}
	})
	if !strings.Contains(output, "elicitation action=accept db=sqlite") {
		t.Fatalf("the mock must report the received answer, got:\n%s", output)
	}
}

// TestHandlePromptElicitationWithoutAClientAnswer keeps a silent client from
// hanging the mock: the round trip must give up and say so.
func TestHandlePromptElicitationWithoutAClientAnswer(t *testing.T) {
	params := json.RawMessage(`{"sessionId":"s3","prompt":[{"text":"__ELICITATION_TEST__"}]}`)
	output := captureStdout(t, func() {
		handlePrompt(jsonRPCRequest{Params: params}, scannerOver(""))
	})
	if !strings.Contains(output, "elicitation") {
		t.Fatalf("a missing client answer must be reported, got:\n%s", output)
	}
}

// TestHandlePromptElicitationRejectsAMalformedAnswer keeps a broken client reply
// from being read as an accepted answer.
func TestHandlePromptElicitationRejectsAMalformedAnswer(t *testing.T) {
	params := json.RawMessage(`{"sessionId":"s4","prompt":[{"text":"__ELICITATION_TEST__"}]}`)
	clientReply := `{"jsonrpc":"2.0","id":200,"result":"not an object"}` + "\n"
	output := captureStdout(t, func() {
		handlePrompt(jsonRPCRequest{Params: params}, scannerOver(clientReply))
	})
	if !strings.Contains(output, "elicitation") {
		t.Fatalf("a malformed answer must be reported, got:\n%s", output)
	}
}

// TestPromptHasTextRequiresAnExactMatch keeps a substring from triggering the
// special test paths by accident.
func TestPromptHasTextRequiresAnExactMatch(t *testing.T) {
	prompt := []promptPart{{Text: "hello"}, {Text: "__ELICITATION_TEST__"}}
	if !promptHasText(prompt, "__ELICITATION_TEST__") {
		t.Fatal("an exact part must match")
	}
	if promptHasText(prompt, "__ELICITATION") {
		t.Fatal("a substring must not match")
	}
	if promptHasText(nil, "__ELICITATION_TEST__") {
		t.Fatal("an empty prompt must not match")
	}
}
