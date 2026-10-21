package zedacp

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// NoticeCapabilities and CompactionCapabilities advertise the complete stable
// v1 presentation contracts. V2 includes these updates in its baseline.
type NoticeCapabilities struct{}
type CompactionCapabilities struct{}

type sessionUpdateWire struct {
	*wireUpdate
	Title       json.RawMessage `json:"title"`
	UpdatedAt   json.RawMessage `json:"updatedAt"`
	Content     json.RawMessage `json:"content"`
	Meta        json.RawMessage `json:"_meta"`
	StopReason  json.RawMessage `json:"stopReason"`
	Description json.RawMessage `json:"description"`
	Summary     json.RawMessage `json:"summary"`
	Error       json.RawMessage `json:"error"`
}

func (u *SessionUpdate) applyUpdateOptions(wire sessionUpdateWire) error {
	u.Title, u.TitleSet, u.TitleCleared = optionalPresentationString(wire.Title)
	u.UpdatedAt, u.UpdatedAtSet, u.UpdatedAtCleared = optionalPresentationString(wire.UpdatedAt)
	u.MetaCleared = isJSONNull(wire.Meta)
	u.MetaSet = len(wire.Meta) > 0 && (u.MetaCleared || json.Unmarshal(wire.Meta, &u.Meta) == nil)
	// Malformed optional values are absent, never actionable defaults.
	if !u.MetaSet {
		u.Meta = nil
	}
	_ = json.Unmarshal(wire.StopReason, &u.StopReason)
	_ = json.Unmarshal(wire.Description, &u.Description)
	return u.validatePresentationUpdate(wire)
}

func (u *SessionUpdate) validatePresentationUpdate(wire sessionUpdateWire) error {
	switch u.SessionUpdate {
	case "notice":
		if u.Severity == "" || u.Title == "" {
			return fmt.Errorf("notice requires severity and a non-empty title")
		}
	case "compaction_update":
		if u.CompactionID == "" || u.Status == "" {
			return fmt.Errorf("compaction update requires compactionId and status")
		}
		u.decodeCompactionPatch(wire)
	case "compaction_summary_chunk":
		if !validCompactionChunk(u) {
			return fmt.Errorf("compaction summary chunk requires compactionId and content")
		}
	case "state_update":
		if !isJSONNull(wire.Error) {
			u.Error = cloneRawMessage(wire.Error)
		}
	}
	return nil
}

func isJSONNull(raw json.RawMessage) bool { return bytes.Equal(bytes.TrimSpace(raw), []byte("null")) }

func (u *SessionUpdate) decodeCompactionPatch(wire sessionUpdateWire) {
	u.SummaryCleared = isJSONNull(wire.Summary)
	if u.SummaryCleared {
		u.SummarySet = true
	} else if len(wire.Summary) > 0 {
		var items []json.RawMessage
		if json.Unmarshal(wire.Summary, &items) == nil {
			u.SummarySet = true
			for _, item := range items {
				var content Content
				if json.Unmarshal(item, &content) == nil && looksLikeContentBlock(content) {
					u.Summary = append(u.Summary, content)
				}
			}
		}
	}
	if isJSONNull(wire.Error) {
		u.ErrorSet = true
		u.Error = json.RawMessage("null")
	} else if len(wire.Error) > 0 {
		var message string
		if json.Unmarshal(wire.Error, &message) == nil {
			u.ErrorSet = true
			u.Error = cloneRawMessage(wire.Error)
		}
	}
}

// Marshal the explicit clearing operations that omitempty would otherwise erase.
func (u SessionUpdate) marshalUpdatePatch(value interface{}) ([]byte, error) {
	data, err := json.Marshal(value)
	if err != nil || !u.hasPresentationPatch() {
		return data, err
	}
	var fields map[string]json.RawMessage
	if err = json.Unmarshal(data, &fields); err != nil {
		return nil, err
	}
	u.applyStringPatches(fields)
	if u.MetaSet {
		fields["_meta"], err = json.Marshal(u.Meta)
	}
	if err != nil {
		return nil, err
	}
	if u.SummarySet {
		if u.SummaryCleared {
			fields["summary"] = json.RawMessage("null")
		} else {
			summary := u.Summary
			if summary == nil {
				summary = []Content{}
			}
			fields["summary"], err = json.Marshal(summary)
		}
	}
	if err != nil {
		return nil, err
	}
	return json.Marshal(fields)
}

func validCompactionChunk(u *SessionUpdate) bool { return u.CompactionID != "" && u.Content.Type != "" }

func optionalPresentationString(raw json.RawMessage) (string, bool, bool) {
	if len(raw) == 0 {
		return "", false, false
	}
	if isJSONNull(raw) {
		return "", true, true
	}
	var value string
	if json.Unmarshal(raw, &value) != nil {
		return "", false, false
	}
	return value, true, false
}

func (u SessionUpdate) hasPresentationPatch() bool {
	return u.MetaSet || u.SummarySet || u.TitleSet || u.UpdatedAtSet
}
func presentationStringPatch(value string, set, cleared bool) json.RawMessage {
	if !set {
		return nil
	}
	if cleared {
		return json.RawMessage("null")
	}
	data, _ := json.Marshal(value)
	return data
}
func (u SessionUpdate) applyStringPatches(fields map[string]json.RawMessage) {
	for key, value := range map[string]json.RawMessage{
		"title":     presentationStringPatch(u.Title, u.TitleSet, u.TitleCleared),
		"updatedAt": presentationStringPatch(u.UpdatedAt, u.UpdatedAtSet, u.UpdatedAtCleared),
	} {
		if value != nil {
			fields[key] = value
		}
	}
}
