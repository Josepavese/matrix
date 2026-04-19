package a2astate

import (
	"encoding/json"
	"strings"
)

type State struct {
	TaskID    string `json:"task_id,omitempty"`
	ContextID string `json:"context_id,omitempty"`
}

func Encode(state State) string {
	if state.TaskID == "" && state.ContextID == "" {
		return ""
	}
	data, _ := json.Marshal(state)
	return string(data)
}

func Decode(raw string) State {
	if strings.TrimSpace(raw) == "" {
		return State{}
	}
	var state State
	if err := json.Unmarshal([]byte(raw), &state); err != nil {
		return State{}
	}
	return state
}

func TaskID(raw string) string {
	if taskID := strings.TrimSpace(Decode(raw).TaskID); taskID != "" {
		return taskID
	}
	return strings.TrimSpace(raw)
}
