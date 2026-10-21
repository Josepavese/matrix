package agents

import (
	"encoding/json"
	"fmt"
	"github.com/Josepavese/matrix/pkg/zedacp"
)

// Idle/replay snapshots are not completions. A foreground transition must have
// been observed on this turn before idle can finish it; background text is not
// a substitute for the agent's explicit lifecycle.
func (o *simpleObserver) observeForegroundState(update acpSessionUpdate) {
	if update.SessionUpdate != "state_update" {
		return
	}
	o.mu.Lock()
	switch update.State {
	case "running", "requires_action":
		o.foreground, o.terminal = true, false
		o.stopReason, o.turnError = "", nil
	case "idle":
		if o.foreground {
			o.foreground, o.terminal = false, true
			o.stopReason = update.StopReason
			o.turnError = append(json.RawMessage(nil), update.Error...)
		}
	}
	o.mu.Unlock()
	o.signalUpdate()
}

func (o *simpleObserver) foregroundError() error {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.stopReason != "error" {
		return nil
	}
	var failure struct {
		Code    int         `json:"code"`
		Message string      `json:"message"`
		Data    interface{} `json:"data"`
	}
	if len(o.turnError) > 0 && json.Unmarshal(o.turnError, &failure) == nil && failure.Message != "" {
		return &zedacp.RPCError{Code: failure.Code, Message: failure.Message, Data: failure.Data}
	}
	return fmt.Errorf("provider foreground work ended with error")
}
