package agents

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	goexec "os/exec"
	"strings"
	"testing"
	"time"

	"github.com/Josepavese/matrix/internal/logic/agentlaunch"
	"github.com/Josepavese/matrix/internal/middleware"
)

const codexPolicyHelperRole = "MATRIX_CODEX_POLICY_HELPER_ROLE"

type codexPolicyResolver struct {
	endpoint middleware.ProtocolEndpoint
}

func (r codexPolicyResolver) GetAgentEndpoint(string) (middleware.ProtocolEndpoint, error) {
	return r.endpoint, nil
}

func TestRouterCodexPolicyReachesRuntimeChild(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	resolver := codexPolicyResolver{endpoint: middleware.ProtocolEndpoint{
		Kind:      middleware.ProtocolKindACP,
		Transport: "stdio",
		Command:   executable,
		Args: []string{
			"-test.run=TestCodexPolicyTwoProcessHelper",
			"-c", "sandbox_mode=danger-full-access",
			"-c", "approval_policy=never",
		},
		Env: []string{
			codexPolicyHelperRole + "=wrapper",
			agentlaunch.CodexPolicyContractEnv + "=" + agentlaunch.CodexPolicyContractV1,
		},
	}}
	router := NewRouter(resolver)
	defer router.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	output, _, _, _, err := router.Route(ctx, middleware.RouteRequest{
		AgentID:          "codex",
		LogicalSessionID: "codex-policy-two-process",
		Message:          "report effective launch policy",
	})
	if err != nil {
		t.Fatalf("Route failed: %v", err)
	}
	for _, expected := range []string{
		`"initial_agent_mode":"agent-full-access"`,
		`"approval_policy":"never"`,
		`"sandbox_mode":"danger-full-access"`,
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("runtime child proof missing %q: %s", expected, output)
		}
	}
}

func TestCodexPolicyTwoProcessHelper(_ *testing.T) {
	switch os.Getenv(codexPolicyHelperRole) {
	case "":
		return
	case "child":
		var config map[string]interface{}
		_ = json.Unmarshal([]byte(os.Getenv("CODEX_CONFIG")), &config)
		proof := map[string]string{
			"initial_agent_mode": os.Getenv("INITIAL_AGENT_MODE"),
			"approval_policy":    fmt.Sprint(config["approval_policy"]),
			"sandbox_mode":       fmt.Sprint(config["sandbox_mode"]),
		}
		payload, _ := json.Marshal(proof)
		fmt.Println(string(payload))
		os.Exit(0)
	case "wrapper":
		runCodexPolicyWrapper()
		os.Exit(0)
	default:
		os.Exit(2)
	}
}

func runCodexPolicyWrapper() {
	executable, err := os.Executable()
	if err != nil {
		return
	}
	child := goexec.Command(executable, "-test.run=TestCodexPolicyTwoProcessHelper")
	child.Env = replaceProcessEnv(os.Environ(), codexPolicyHelperRole, "child")
	childProof, err := child.Output()
	if err != nil {
		return
	}
	proof := strings.TrimSpace(string(childProof))
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		var req codexPolicyRPCRequest
		if json.Unmarshal(scanner.Bytes(), &req) != nil {
			continue
		}
		result := json.RawMessage(`{}`)
		switch req.Method {
		case "initialize":
			result = json.RawMessage(`{"protocolVersion":1,"agentCapabilities":{}}`)
		case "session/new":
			result = json.RawMessage(`{"sessionId":"policy-session","modes":{"currentModeId":"agent-full-access","availableModes":[{"id":"read-only","name":"Read-only"},{"id":"agent","name":"Agent"},{"id":"agent-full-access","name":"Agent full access"}]}}`)
		case "session/prompt":
			writeCodexPolicyNotification("policy-session", proof)
			result = json.RawMessage(`{"stopReason":"end_turn"}`)
		}
		writeCodexPolicyRPC(codexPolicyRPCResponse{JSONRPC: "2.0", ID: req.ID, Result: result})
	}
}

type codexPolicyRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type codexPolicyRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
}

func writeCodexPolicyNotification(sessionID, text string) {
	params, _ := json.Marshal(map[string]interface{}{
		"sessionId": sessionID,
		"update": map[string]interface{}{
			"sessionUpdate": "agent_message_chunk",
			"content":       map[string]interface{}{"type": "text", "text": text},
		},
	})
	writeCodexPolicyRPC(codexPolicyRPCResponse{JSONRPC: "2.0", Method: "session/update", Params: params})
}

func writeCodexPolicyRPC(value codexPolicyRPCResponse) {
	payload, err := json.Marshal(value)
	if err == nil {
		fmt.Println(string(payload))
	}
}

func replaceProcessEnv(env []string, key, value string) []string {
	prefix := key + "="
	out := make([]string, 0, len(env)+1)
	for _, entry := range env {
		if !strings.HasPrefix(entry, prefix) {
			out = append(out, entry)
		}
	}
	return append(out, prefix+value)
}
