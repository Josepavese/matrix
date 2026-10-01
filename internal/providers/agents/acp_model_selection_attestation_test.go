package agents

import (
	"context"
	"strings"
	"testing"

	"github.com/Josepavese/matrix/internal/middleware"
)

// ----------------------------------------------------------------------------
// Model attestation on reused sessions (issue 5)
//
// The tests below pin the contract the external consumer needs: a requested
// model is selected on the session, but only a statement made by the provider
// itself can confirm it. A reused session that receives no session state must
// say the verification was not repeated instead of blaming the provider, and a
// provider whose surface cannot state a model must say so.
// ----------------------------------------------------------------------------

type attestationACPClient struct {
	ctx             context.Context
	protocolVersion int
	newSessionID    string
	echoModel       bool
	configOptions   []acpConfigOption
	configErr       error
	setModelErr     error
	lastConfigValue string
	lastModelID     string
	prompts         []acpPromptRequest
	// promptErrs is consumed one entry per prompt: a nil entry lets the prompt
	// succeed, a non-nil one fails it. It is how a test makes the first attempt
	// lose its session and the retry succeed.
	promptErrs      []error
	newSessionCalls int
	setModelCalls   int
	modelBySession  map[string]string
}

func (c *attestationACPClient) Context() context.Context            { return c.ctx }
func (c *attestationACPClient) Close() error                        { return nil }
func (c *attestationACPClient) SetRequestHandler(acpRequestHandler) {}
func (c *attestationACPClient) AuthenticatedProtocolVersion() int   { return c.protocolVersion }
func (c *attestationACPClient) Initialize(context.Context, acpInitializeRequest) (*acpInitializeResponse, error) {
	return &acpInitializeResponse{ProtocolVersion: c.protocolVersion}, nil
}
func (c *attestationACPClient) Authenticate(context.Context, string) error { return nil }
func (c *attestationACPClient) Logout(context.Context, acpLogoutRequest) (*acpLogoutResponse, error) {
	return &acpLogoutResponse{}, nil
}
func (c *attestationACPClient) NewSession(context.Context, acpNewSessionRequest) (*acpNewSessionResponse, error) {
	c.newSessionCalls++
	if c.newSessionID == "" {
		c.newSessionID = "remote-attested"
	}
	return &acpNewSessionResponse{SessionID: c.newSessionID}, nil
}
func (c *attestationACPClient) LoadSession(context.Context, acpLoadSessionRequest, acpSessionObserver) (*acpLoadSessionResponse, error) {
	return &acpLoadSessionResponse{}, nil
}
func (c *attestationACPClient) ResumeSession(context.Context, acpResumeSessionRequest) (*acpResumeSessionResponse, error) {
	return &acpResumeSessionResponse{}, nil
}
func (c *attestationACPClient) ListSessions(context.Context) (*acpListSessionsResponse, error) {
	return &acpListSessionsResponse{}, nil
}
func (c *attestationACPClient) ListSessionsWithRequest(context.Context, acpListSessionsRequest) (*acpListSessionsResponse, error) {
	return &acpListSessionsResponse{}, nil
}
func (c *attestationACPClient) CancelSession(context.Context, string) error { return nil }
func (c *attestationACPClient) CloseSession(context.Context, string) error  { return nil }
func (c *attestationACPClient) DeleteSession(context.Context, string) error { return nil }
func (c *attestationACPClient) ForkSession(context.Context, acpForkSessionRequest) (*acpForkSessionResponse, error) {
	return &acpForkSessionResponse{}, nil
}
func (c *attestationACPClient) Prompt(_ context.Context, req acpPromptRequest, _ acpSessionObserver) (*acpPromptResponse, error) {
	c.prompts = append(c.prompts, req)
	if len(c.promptErrs) > 0 {
		err := c.promptErrs[0]
		c.promptErrs = c.promptErrs[1:]
		if err != nil {
			return nil, err
		}
	}
	return &acpPromptResponse{StopReason: "end_turn"}, nil
}
func (c *attestationACPClient) SetMode(context.Context, string, string) error { return nil }
func (c *attestationACPClient) SetConfigOption(_ context.Context, req acpSetConfigOptionRequest) (*acpSetConfigOptionResponse, error) {
	c.lastConfigValue, _ = req.Value.(string)
	if c.configErr != nil {
		return nil, c.configErr
	}
	if c.echoModel {
		return &acpSetConfigOptionResponse{ConfigOptions: []acpConfigOption{{ID: acpModelConfigOptionID, Current: c.lastConfigValue}}}, nil
	}
	return &acpSetConfigOptionResponse{ConfigOptions: c.configOptions}, nil
}
func (c *attestationACPClient) SetSessionModel(_ context.Context, req acpSetSessionModelRequest) (*acpSetSessionModelResponse, error) {
	c.lastModelID = req.ModelID
	c.setModelCalls++
	if c.modelBySession == nil {
		c.modelBySession = map[string]string{}
	}
	c.modelBySession[req.SessionID] = req.ModelID
	if c.setModelErr != nil {
		return nil, c.setModelErr
	}
	return &acpSetSessionModelResponse{}, nil
}
func (c *attestationACPClient) ExtRequest(context.Context, string, interface{}, interface{}) error {
	return nil
}
func (c *attestationACPClient) ExtNotification(context.Context, string, interface{}) error {
	return nil
}

type modelReceiptCapture struct {
	receipts []middleware.ModelSelection
}

func (*modelReceiptCapture) OnThought(middleware.ThoughtUpdate) {}
func (*modelReceiptCapture) SetHeader(string, string)           {}
func (*modelReceiptCapture) FormattedHeader() string            { return "" }
func (c *modelReceiptCapture) OnModelSelection(selection middleware.ModelSelection) {
	c.receipts = append(c.receipts, selection)
}

func turnClient(fake *attestationACPClient) *acpConversationClient {
	return &acpConversationClient{client: fake, loadedSessions: map[string]bool{}}
}

// TestNewAndReusedContextualTurnsKeepRequestedSelectedAndConfirmedApart is the
// test the external report asked for: a new session, then a second contextual
// run with the same model and a different request, against a provider whose
// acknowledgement never states a model. Both runs must keep the requested model
// selected and unconfirmed, and the reuse must be visible in the reason rather
// than silently elevated to a confirmation.
func TestNewAndReusedContextualTurnsKeepRequestedSelectedAndConfirmedApart(t *testing.T) {
	fake := &attestationACPClient{ctx: context.Background(), protocolVersion: 2}
	client := turnClient(fake)
	capture := &modelReceiptCapture{}
	model := "deepseek/deepseek-flash"

	first, err := client.ExecuteTurn(context.Background(), middleware.ConversationTurn{
		Message: "prima richiesta", ModelID: model, ThoughtNotifier: capture})
	if err != nil {
		t.Fatalf("first turn: %v", err)
	}
	second, err := client.ExecuteTurn(context.Background(), middleware.ConversationTurn{
		Message: "seconda richiesta, diversa", ModelID: model,
		RemoteSessionID: first.RemoteSessionID, ThoughtNotifier: capture})
	if err != nil {
		t.Fatalf("second contextual turn: %v", err)
	}
	if len(capture.receipts) != 2 {
		t.Fatalf("expected one model selection per run, got %#v", capture.receipts)
	}
	if second.RemoteSessionID != first.RemoteSessionID || first.RemoteSessionID == "" {
		t.Fatalf("second run did not reuse the contextual session: first=%q second=%q",
			first.RemoteSessionID, second.RemoteSessionID)
	}
	if len(fake.prompts) != 2 || fake.prompts[0].Prompt[0].Text == fake.prompts[1].Prompt[0].Text {
		t.Fatalf("the second run must carry a different request on the same session: %#v", fake.prompts)
	}
	for index, selection := range capture.receipts {
		if selection.ConfiguredModel != model {
			t.Fatalf("run %d: selected model missing: %#v", index+1, selection)
		}
		if selection.EffectiveModel != "" {
			t.Fatalf("run %d: an acknowledgement with no model was treated as confirmation: %#v", index+1, selection)
		}
		if selection.Verification != middleware.ModelVerificationUnverified {
			t.Fatalf("run %d: unconfirmed selection reported as %q", index+1, selection.Verification)
		}
		if selection.EvidenceSource != acpSetSessionModelMethod {
			t.Fatalf("run %d: evidence source must name the inspected response, got %q", index+1, selection.EvidenceSource)
		}
	}
	// The creating run blames the surface that cannot state a model; the second
	// run reused the live session, so it must report that the verification was
	// not repeated instead of claiming the same provider limitation.
	if got := capture.receipts[0].VerificationReason; got != middleware.ModelUnverifiedProviderDoesNotAttest {
		t.Fatalf("created run reason=%q want %q", got, middleware.ModelUnverifiedProviderDoesNotAttest)
	}
	if got := capture.receipts[1].VerificationReason; got != middleware.ModelUnverifiedNotRepeated {
		t.Fatalf("reused run reason=%q want %q", got, middleware.ModelUnverifiedNotRepeated)
	}
	if fake.lastModelID != model {
		t.Fatalf("requested model was not applied to the session: %q", fake.lastModelID)
	}
}

// TestSessionOriginSelectsTheUnverifiedReason pins the reuse distinction: the
// same non-attesting provider yields "the provider does not attest" on a freshly
// created session and "verification was not repeated" when the session was
// reused without any state response on that turn.
func TestSessionOriginSelectsTheUnverifiedReason(t *testing.T) {
	fake := &attestationACPClient{ctx: context.Background(), protocolVersion: 2}
	client := turnClient(fake)

	created, err := client.selectTurnModelFor(context.Background(), turnModelSelection{
		SessionID: "remote-attested", ModelID: "chosen-model", Origin: sessionOriginCreated})
	if err != nil || created.VerificationReason != middleware.ModelUnverifiedProviderDoesNotAttest {
		t.Fatalf("created session reason=%q err=%v", created.VerificationReason, err)
	}
	reused, err := client.selectTurnModelFor(context.Background(), turnModelSelection{
		SessionID: "remote-attested", ModelID: "chosen-model", Origin: sessionOriginReused})
	if err != nil || reused.VerificationReason != middleware.ModelUnverifiedNotRepeated {
		t.Fatalf("reused session reason=%q err=%v", reused.VerificationReason, err)
	}
	if created.EffectiveModel != "" || reused.EffectiveModel != "" {
		t.Fatalf("no statement from the provider may become a confirmation: created=%#v reused=%#v", created, reused)
	}
	// Without a classified origin the selection never claims that verification
	// was not repeated: the adapter must not guess the session's history.
	unclassified, err := client.selectTurnModelFor(context.Background(), turnModelSelection{
		SessionID: "remote-attested", ModelID: "chosen-model"})
	if err != nil || unclassified.VerificationReason != middleware.ModelUnverifiedProviderDoesNotAttest {
		t.Fatalf("unclassified origin reason=%q err=%v", unclassified.VerificationReason, err)
	}
}

// TestAttestingProviderConfirmsTheSelectionOnEveryTurn proves the change does
// not blanket-refuse confirmation: a provider that reports the model it accepted
// confirms the selection, on the creating turn and on a reused one.
func TestAttestingProviderConfirmsTheSelectionOnEveryTurn(t *testing.T) {
	fake := &attestationACPClient{ctx: context.Background(), protocolVersion: 1, echoModel: true}
	client := turnClient(fake)

	first, err := client.ExecuteTurn(context.Background(), middleware.ConversationTurn{
		Message: "prima richiesta", ModelID: "chosen-model", ThoughtNotifier: &modelReceiptCapture{}})
	if err != nil {
		t.Fatalf("first turn: %v", err)
	}
	capture := &modelReceiptCapture{}
	if _, err := client.ExecuteTurn(context.Background(), middleware.ConversationTurn{
		Message: "seconda richiesta", ModelID: "chosen-model",
		RemoteSessionID: first.RemoteSessionID, ThoughtNotifier: capture}); err != nil {
		t.Fatalf("reused turn: %v", err)
	}
	if len(capture.receipts) != 1 {
		t.Fatalf("missing selection receipt on the reused turn: %#v", capture.receipts)
	}
	selection := capture.receipts[0]
	if selection.Verification != middleware.ModelVerificationConfirmed || selection.EffectiveModel != "chosen-model" {
		t.Fatalf("provider-stated model was not confirmed on a reused session: %#v", selection)
	}
	if selection.VerificationReason != "" || selection.EvidenceSource != acpSetConfigOptionMethod {
		t.Fatalf("confirmation must carry no failure reason and name its source: %#v", selection)
	}
}

// TestProviderListingNoModelOptionReportsTheSessionAsNotSelectable covers the
// provider that answers with session state and no model selector. The turn must
// continue with an unverified selection and the specific reason, instead of
// failing a prompt the provider accepted.
func TestProviderListingNoModelOptionReportsTheSessionAsNotSelectable(t *testing.T) {
	fake := &attestationACPClient{ctx: context.Background(), protocolVersion: 1,
		configOptions: []acpConfigOption{{ID: "mode", Current: "code"}}}
	capture := &modelReceiptCapture{}
	if _, err := turnClient(fake).ExecuteTurn(context.Background(), middleware.ConversationTurn{
		Message: "work", ModelID: "chosen-model", ThoughtNotifier: capture}); err != nil {
		t.Fatalf("provider that exposes no model selector must not fail the turn: %v", err)
	}
	if len(fake.prompts) != 1 {
		t.Fatalf("prompt was not sent after an unverifiable selection: %#v", fake.prompts)
	}
	if len(capture.receipts) != 1 {
		t.Fatalf("missing selection receipt: %#v", capture.receipts)
	}
	if selection := capture.receipts[0]; selection.VerificationReason != middleware.ModelUnverifiedModelNotSelectable ||
		selection.EffectiveModel != "" || selection.ConfiguredModel != "chosen-model" {
		t.Fatalf("session without a model selector misreported: %#v", selection)
	}
}

// TestModelOptionWithoutCurrentValueIsLostEvidence keeps "the option exists but
// the provider stated no value" distinct from "the provider stated nothing".
func TestModelOptionWithoutCurrentValueIsLostEvidence(t *testing.T) {
	fake := &attestationACPClient{ctx: context.Background(), protocolVersion: 1,
		configOptions: []acpConfigOption{{ID: acpModelConfigOptionID}}}
	capture := &modelReceiptCapture{}
	if _, err := turnClient(fake).ExecuteTurn(context.Background(), middleware.ConversationTurn{
		Message: "work", ModelID: "chosen-model", ThoughtNotifier: capture}); err != nil {
		t.Fatalf("turn: %v", err)
	}
	if len(capture.receipts) != 1 || capture.receipts[0].VerificationReason != middleware.ModelUnverifiedEvidenceLost {
		t.Fatalf("model option without a current value misreported: %#v", capture.receipts)
	}
}

// TestProviderStatingAnotherModelFailsClosed keeps a contradiction a failure: the
// provider accepted the call and then reported a different model, which is not
// missing evidence but a rejected selection.
func TestProviderStatingAnotherModelFailsClosed(t *testing.T) {
	fake := &attestationACPClient{ctx: context.Background(), protocolVersion: 1,
		configOptions: []acpConfigOption{{ID: acpModelConfigOptionID, Current: "other-model"}}}
	capture := &modelReceiptCapture{}
	_, err := turnClient(fake).ExecuteTurn(context.Background(), middleware.ConversationTurn{
		Message: "work", ModelID: "chosen-model", ThoughtNotifier: capture})
	if err == nil || !strings.Contains(err.Error(), "session reports") {
		t.Fatalf("a contradictory session model must fail closed, got err=%v", err)
	}
	if len(fake.prompts) != 0 {
		t.Fatalf("contradicted selection reached the prompt: %#v", fake.prompts)
	}
	if len(capture.receipts) != 0 {
		t.Fatalf("a contradicted selection must not be published as evidence: %#v", capture.receipts)
	}
}

// TestLostSessionRetryReappliesAndRenotifiesTheModel pins the retry path a lost
// session takes. The retry rebuilds the turn for a fresh session, and the model
// contract of the original turn travels with it: the requested model is applied
// to the new session and published again, so the issue 5 symptom (no
// configured_model, no model.selection event) cannot come back through here.
func TestLostSessionRetryReappliesAndRenotifiesTheModel(t *testing.T) {
	fake := &attestationACPClient{ctx: context.Background(), protocolVersion: 2,
		newSessionID: "remote-fresh", promptErrs: []error{middleware.ErrSessionNotFound}}
	client := turnClient(fake)
	capture := &modelReceiptCapture{}
	model := "deepseek/deepseek-flash"

	result, err := client.ExecuteTurn(context.Background(), middleware.ConversationTurn{
		AgentID: "opencode", Message: "richiesta iniziale", ModelID: model,
		FallbackModelID: "backup-model", RemoteSessionID: "remote-lost", ThoughtNotifier: capture})
	if err != nil {
		t.Fatalf("turn after the lost-session retry: %v", err)
	}
	if fake.newSessionCalls != 1 || result.RemoteSessionID != "remote-fresh" {
		t.Fatalf("the retry must run on a rebuilt session: remote=%q new=%d",
			result.RemoteSessionID, fake.newSessionCalls)
	}
	if len(fake.prompts) != 2 || fake.prompts[1].SessionID != "remote-fresh" {
		t.Fatalf("the retried prompt did not reach the fresh session: %#v", fake.prompts)
	}
	if got := fake.modelBySession["remote-fresh"]; got != model {
		t.Fatalf("the requested model was not applied to the retried session: set=%#v", fake.modelBySession)
	}
	if len(capture.receipts) != 2 {
		t.Fatalf("the retried turn published %d model selections, want one per attempt: %#v",
			len(capture.receipts), capture.receipts)
	}
	retried := capture.receipts[1]
	if retried.ConfiguredModel != model || retried.EffectiveModel != "" ||
		retried.Verification != middleware.ModelVerificationUnverified ||
		retried.VerificationReason != middleware.ModelUnverifiedProviderDoesNotAttest {
		t.Fatalf("retried selection lost the model contract: %#v", retried)
	}
}

// TestFreshSessionRetryNeverDemotesAStrictTurn pins the security side of the same
// reconstruction. A strict turn is a caller statement that the named remote
// session is the only session this turn may run on; rebuilding it for a fresh
// session must therefore fail closed instead of quietly creating a session.
// The public path does not retry strict turns at all today, so this pins the
// helper's own contract rather than a reachable state.
func TestFreshSessionRetryNeverDemotesAStrictTurn(t *testing.T) {
	fake := &attestationACPClient{ctx: context.Background(), protocolVersion: 2, newSessionID: "remote-fresh"}
	capture := &modelReceiptCapture{}

	_, err := turnClient(fake).retryTurnWithFreshSession(context.Background(), middleware.ConversationTurn{
		AgentID: "opencode", Message: "richiesta", ModelID: "chosen-model",
		RemoteSessionID: "remote-lost", StrictSession: true, ThoughtNotifier: capture})
	if err == nil {
		t.Fatal("a strict turn must not be silently retried onto a newly created session")
	}
	if fake.newSessionCalls != 0 {
		t.Fatalf("a strict turn created %d remote session(s)", fake.newSessionCalls)
	}
	if len(fake.prompts) != 0 {
		t.Fatalf("a strict turn reached the prompt without its session: %#v", fake.prompts)
	}
}
