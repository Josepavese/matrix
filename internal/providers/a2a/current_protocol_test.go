package a2a

import (
	"encoding/json"
	a2asdk "github.com/a2aproject/a2a-go/v2/a2a"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

// Raw v1 callers must negotiate the protocol version, as SDK clients do.
func postCurrentA2A(url, contentType string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodPost, url, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("A2A-Version", "1.0")
	return http.DefaultClient.Do(req)
}

func TestCurrentProtocolRejectsUnsupportedVersionAndRetiredMethods(t *testing.T) {
	s := newA2ATestServer(t, NewServer(&stubSessionRouter{}, "http://127.0.0.1:0", "opencode"))
	for _, tc := range []struct {
		version, method string
		code            int
	}{{"99.0", "ListTasks", -32009}, {"0.3", "ListTasks", -32009}, {"1.0", "message/send", -32601}} {
		req, err := http.NewRequest(http.MethodPost, s.URL+"/a2a", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"`+tc.method+`","params":{}}`))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("A2A-Version", tc.version)
		req.Header.Set("Content-Type", "application/json")
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		var e rpcEnvelope
		err = json.NewDecoder(res.Body).Decode(&e)
		_ = res.Body.Close()
		if err != nil {
			t.Fatal(err)
		}
		if e.Error == nil || e.Error.Code != tc.code {
			t.Fatalf("%s %s expected %d, got %+v", tc.version, tc.method, tc.code, e.Error)
		}
	}
}
func TestCurrentRESTNegotiationAndTrailingDataRejection(t *testing.T) {
	s := newA2ATestServer(t, NewServer(&stubSessionRouter{}, "http://127.0.0.1:0", "opencode"))
	for _, tc := range []struct {
		body    string
		invalid bool
	}{{sendMessageParams("current", "hello"), false}, {sendMessageParams("trailing", "hello") + ` {}`, true}} {
		res, err := postCurrentA2A(s.URL+"/a2a/rest/message:send", "application/a2a+json", strings.NewReader(tc.body))
		if err != nil {
			t.Fatal(err)
		}
		b, err := io.ReadAll(res.Body)
		_ = res.Body.Close()
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(res.Header.Get("Content-Type"), "application/a2a+json") {
			t.Fatalf("media type negotiation lost: %s", res.Header.Get("Content-Type"))
		}
		if tc.invalid && res.StatusCode != http.StatusBadRequest {
			t.Fatalf("trailing JSON accepted: %d %s", res.StatusCode, b)
		}
		if !tc.invalid && res.StatusCode != http.StatusOK {
			t.Fatalf("valid v1 message failed: %d %s", res.StatusCode, b)
		}
	}
}

func TestAdvertisedInputFamiliesAcceptImagesAndRejectVideo(t *testing.T) {
	s := newA2ATestServer(t, NewServer(&stubSessionRouter{}, "http://127.0.0.1:0", "opencode"))
	for _, tc := range []struct {
		kind string
		code int
	}{{"image/png", 0}, {"audio/wav", 0}, {"video/mp4", -32005}} {
		params := `{"message":{"messageId":"media","role":"user","parts":[{"text":"hello"},{"raw":"eA==","mediaType":"` + tc.kind + `"}]}}`
		e := callRPC(t, s.URL, "SendMessage", params)
		if tc.code != 0 {
			e.requireError(t, "SendMessage", tc.code)
		} else if e.Error != nil {
			t.Fatalf("advertised family rejected %s: %+v", tc.kind, e.Error)
		}
	}
}

func TestCurrentAgentCardScopesAndStatusUTC(t *testing.T) {
	var card a2asdk.AgentCard
	if err := json.Unmarshal([]byte(`{"securityRequirements":[{"schemes":{"oauth":{"list":["read","write"]}}}]}`), &card); err != nil {
		t.Fatal(err)
	}
	scopes := card.SecurityRequirements[0]["oauth"]
	if len(scopes) != 2 || scopes[1] != "write" {
		t.Fatalf("ProtoJSON scopes lost: %#v", card.SecurityRequirements)
	}
	encoded, err := json.Marshal(card)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(encoded), `"list":["read","write"]`) {
		t.Fatalf("canonical scopes not emitted: %s", encoded)
	}
	stamp := time.Date(2026, 10, 21, 12, 0, 0, 0, time.FixedZone("test", 7200))
	status := a2asdk.TaskStatus{State: a2asdk.TaskStateWorking, Timestamp: &stamp}
	encoded, err = json.Marshal(status)
	if err != nil {
		t.Fatal(err)
	}
	var wire struct {
		Timestamp string `json:"timestamp"`
	}
	if err = json.Unmarshal(encoded, &wire); err != nil {
		t.Fatal(err)
	}
	parsed, parseErr := time.Parse(time.RFC3339Nano, wire.Timestamp)
	if parseErr != nil || !parsed.Equal(stamp) || !strings.HasSuffix(wire.Timestamp, "Z") {
		t.Fatalf("task status not UTC: %s", encoded)
	}
}

func TestRESTListTasksUsesCurrentTimestampQuery(t *testing.T) {
	s := newA2ATestServer(t, NewServer(&stubSessionRouter{}, "http://127.0.0.1:0", "opencode"))
	callRPC(t, s.URL, "SendMessage", sendMessageParams("timestamp-filter", "hello")).requireTask(t, "SendMessage")
	future := time.Now().Add(time.Hour).UTC().Format(time.RFC3339Nano)
	response := restRequest(t, http.MethodGet, s.URL+"/a2a/rest/tasks?statusTimestampAfter="+url.QueryEscape(future), "")
	if response.status != http.StatusOK {
		t.Fatalf("filter request failed: %s", response.body)
	}
	var page struct {
		Tasks []json.RawMessage `json:"tasks"`
	}
	if err := json.Unmarshal(response.body, &page); err != nil {
		t.Fatal(err)
	}
	if len(page.Tasks) != 0 {
		t.Fatalf("statusTimestampAfter was ignored: %s", response.body)
	}
}
