package main

// Synthetic presentation, deliberately distinct from the assistant answer.
func streamPresentationUpdates(sessionID string) {
	writeSessionUpdate(sessionID, map[string]interface{}{"sessionUpdate": "notice", "severity": "info", "title": "Protocol notice", "description": "live presentation only"})
	writeSessionUpdate(sessionID, map[string]interface{}{"sessionUpdate": "compaction_summary_chunk", "compactionId": "compact-mock-1", "content": map[string]interface{}{"type": "text", "text": "retained context"}})
	writeSessionUpdate(sessionID, map[string]interface{}{"sessionUpdate": "compaction_update", "compactionId": "compact-mock-1", "status": "completed", "_meta": map[string]interface{}{"source": "mock"}})
}
