package agents

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/Josepavese/matrix/internal/middleware"
)

// Opt-in real adapter proof. Only the final's size/hash and model receipt are
// logged; reasoning and provider credentials never become qualification output.
func TestRealACPFinalDelivery(t *testing.T) {
	bin, model := os.Getenv("MATRIX_TEST_ACP_DELIVERY_BIN"), os.Getenv("MATRIX_TEST_ACP_DELIVERY_MODEL")
	if bin == "" || model == "" {
		t.Skip("set MATRIX_TEST_ACP_DELIVERY_BIN and MODEL for real final delivery")
	}
	var args []string
	if err := json.Unmarshal([]byte(os.Getenv("MATRIX_TEST_ACP_DELIVERY_ARGS")), &args); err != nil {
		t.Fatal(err)
	}
	prompt := `No tools. Return ONLY a JSON object {"schema":"matrix.delivery.probe","fields":{"field01":"synthetic"}}. Include fourteen string fields named field01 through field14, all with value synthetic.`
	if path := os.Getenv("MATRIX_TEST_ACP_DELIVERY_PROMPT_FILE"); path != "" {
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		prompt = string(b)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	workspace := t.TempDir()
	client, err := (&acpConversationFactory{}).NewClient(ctx, middleware.ProtocolEndpoint{
		Kind: middleware.ProtocolKindACP, Transport: "stdio", Command: bin, Args: args,
	}, middleware.ConversationFactoryDeps{AgentID: "delivery-probe", Cwd: workspace})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	delay := time.Duration(0)
	if raw := os.Getenv("MATRIX_TEST_ACP_DELIVERY_NOTIFIER_DELAY"); raw != "" {
		var err error
		delay, err = time.ParseDuration(raw)
		if err != nil || delay <= 0 {
			t.Fatal("invalid notifier delay")
		}
	}
	capture := &deliveryReceiptCapture{modelReceiptCapture: &modelReceiptCapture{}, delay: delay}
	result, err := client.ExecuteTurn(ctx, middleware.ConversationTurn{
		AgentID: "delivery-probe", ModelID: model, LogicalSessionID: "delivery-probe", WorkspacePath: workspace, Message: prompt,
		ThoughtNotifier: capture,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(capture.receipts) != 1 || capture.receipts[0].EffectiveModel != model ||
		capture.receipts[0].Verification != middleware.ModelVerificationConfirmed || capture.receipts[0].FallbackUsed {
		t.Fatalf("requested model not attested: %+v", capture.receipts)
	}
	var proposal struct {
		Fields map[string]json.RawMessage `json:"fields"`
	}
	if err := json.Unmarshal([]byte(result.Output), &proposal); err != nil {
		t.Fatalf("final is not JSON: bytes=%d error=%v", len(result.Output), err)
	}
	if len(proposal.Fields) != 14 {
		t.Fatalf("final fields=%d, want 14", len(proposal.Fields))
	}
	sum := sha256.Sum256([]byte(result.Output))
	t.Logf("remote_session=%s stop_reason=%s final_bytes=%d final_sha256=%s fields=%d model_receipt=%v",
		result.RemoteSessionID, result.StopReason, len(result.Output), hex.EncodeToString(sum[:]), len(proposal.Fields), capture.receipts[0])
}

type deliveryReceiptCapture struct {
	*modelReceiptCapture
	delay time.Duration
}

func (c *deliveryReceiptCapture) OnThought(middleware.ThoughtUpdate) { time.Sleep(c.delay) }
