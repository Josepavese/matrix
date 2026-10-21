package agents

// Protocol identity and evidence classification belong to Matrix. Provider
// extension metadata remains available under acp._meta without gaining authority.
func reservedACPProjectionKey(key string) bool {
	_, reserved := map[string]struct{}{
		"source_update_type": {}, "protocol": {}, "protocol_method": {}, "acp": {}, "message_id": {}, "message_phase": {}, "message_classification": {}, "tool_call_id": {}, "remote_session_id": {}, "tool_kind": {}, "acp_tool_kind": {}, "terminal_id": {}, "plan_id": {},
	}[key]
	return reserved
}

// Per-turn entity metadata materializes v2's top-level patches. Matrix keeps no
// private message history: a new turn reconstructs entities from explicit replay.
func (o *simpleObserver) resolveEntityMetadata(n acpSessionNotification) acpSessionNotification {
	if o.version < 2 {
		return n
	}
	key := acpEntityKey(n.Update)
	if key == "" {
		return n
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.entityMetadata == nil {
		o.entityMetadata = make(map[string]map[string]interface{})
	}
	u := n.Update
	set := u.MetaSet || len(u.Meta) > 0
	resolved := patchACPMetadata(o.entityMetadata[key], u.Meta, set, u.MetaCleared)
	o.entityMetadata[key] = resolved
	n.Update.Meta = clonePresentationMap(resolved)
	return n
}

func acpEntityKey(u acpSessionUpdate) string {
	ids := map[string]string{
		"agent_message": "message:" + u.MessageID, "agent_message_chunk": "message:" + u.MessageID,
		"agent_thought": "thought:" + u.MessageID, "agent_thought_chunk": "thought:" + u.MessageID,
		"tool_call": "tool:" + u.ToolCallID, "tool_call_update": "tool:" + u.ToolCallID, "tool_call_content_chunk": "tool:" + u.ToolCallID,
		"terminal_update": "terminal:" + u.TerminalID, "terminal_output_chunk": "terminal:" + u.TerminalID,
	}
	return ids[u.SessionUpdate]
}

func mergeACPExtensionMetadata(meta, extension map[string]interface{}) {
	for key, value := range extension {
		if _, owned := meta[key]; !owned && !reservedACPProjectionKey(key) {
			meta[key] = value
		}
	}
}
