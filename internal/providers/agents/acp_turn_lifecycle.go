package agents

import (
	"context"
	"os"
	"strings"
	"time"

	"github.com/Josepavese/matrix/internal/logic/sidecar"
	"github.com/Josepavese/matrix/internal/middleware"
	"github.com/Josepavese/matrix/internal/providers/sidecarprojection"
	"github.com/Josepavese/matrix/pkg/zedacp"
)

func (c *acpConversationClient) promptACP(ctx context.Context, remoteSessionID string, turn middleware.ConversationTurn, obs acpSessionObserver) (*acpPromptResponse, error) {
	promptText := sidecar.ProjectPrompt(turn.Message, turn.SidecarCapsules)
	return c.currentACPClient().Prompt(ctx, acpPromptRequest{
		SessionID: remoteSessionID,
		Prompt:    acpPromptContent(promptText, turn.ContentBlocks),
		Meta:      sidecarprojection.ACPMeta(turn.SidecarCapsules),
	}, obs)
}

// defaultACPV2TurnBudget bounds how long a version 2 turn waits for the terminal
// idle state_update after its prompt was accepted. The specification makes that
// update the only signal that ends foreground work, so a peer that never sends
// one would otherwise hold the turn open forever. The value is deliberately
// generous because a real turn can run tools for minutes; a caller's context
// remains the tighter bound whenever it carries a deadline.
const defaultACPV2TurnBudget = 10 * time.Minute

// acpV2TurnEnv names the operator override for that budget. It exists so a
// deployment can fit the bound to its own turn timeouts, and so a test can prove
// the bound without waiting for the default.
const acpV2TurnEnv = "MATRIX_ACP_V2_TURN_BUDGET"

// acpV2TurnBudget reports the bound to use, falling back to the default for an
// unset, unparsable or non-positive override rather than failing a turn over
// configuration.
func acpV2TurnBudget() time.Duration {
	raw := strings.TrimSpace(os.Getenv(acpV2TurnEnv))
	if raw == "" {
		return defaultACPV2TurnBudget
	}
	parsed, err := time.ParseDuration(raw)
	if err != nil || parsed <= 0 {
		return defaultACPV2TurnBudget
	}
	return parsed
}

// acpSessionWatcher is the part of the protocol client that keeps an observer
// registered outside a call. It is a local interface so the port stays small.
type acpSessionWatcher interface {
	WatchSession(sessionID string, observer acpSessionObserver) func()
}

// awaitsTerminalState reports whether this connection's turns end on the peer's
// idle state update. Version 2 defined that lifecycle and made the prompt
// response an acknowledgement of insertion only; every other generation ends a
// turn the way it always did.
func (c *acpConversationClient) awaitsTerminalState() bool {
	return c.negotiatedProtocolVersion() >= zedacp.ProtocolVersionV2
}

// turnObservation is the turn's observer together with how its updates reach it
// and how the turn ends.
//
// A version 2 turn continues after session/prompt is acknowledged: content and
// the terminal state arrive as notifications, and the prompt response says only
// that the user message was inserted. Its observer is therefore registered for
// the whole turn and the prompt itself is handed none, because the same update
// must not be applied twice. A version 1 turn keeps the arrangement it always
// had, where the prompt call owns the observer and its quiet wait ends the turn.
type turnObservation struct {
	observer *simpleObserver
	prompt   acpSessionObserver
	stop     func()
	terminal bool
}

// observeTurn builds the turn's observation, falling back to the version 1
// arrangement when the client cannot keep an observer registered outside a call.
func (c *acpConversationClient) observeTurn(remoteSessionID string, turn middleware.ConversationTurn) turnObservation {
	observation := turnObservation{
		observer: &simpleObserver{updates: make(chan struct{}, 1), notifier: turn.ThoughtNotifier},
		stop:     func() {},
	}
	watcher, ok := c.currentACPClient().(acpSessionWatcher)
	if !ok || !c.awaitsTerminalState() {
		observation.prompt = observation.observer
		return observation
	}
	observation.terminal = true
	observation.stop = watcher.WatchSession(remoteSessionID, observation.observer)
	return observation
}
