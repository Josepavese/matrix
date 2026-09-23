package agents

import "strings"

// ----------------------------------------------------------------------------
// What a session update contributes to Matrix's neutral representation
//
// One function per update family, each projecting the protocol's own fields onto
// the keys a caller, an operator or a channel frontend reads. The version 2
// additions live beside their version 1 counterparts so the two generations of
// the same idea stay comparable: a diff is a diff whether it arrived as one path
// with old and new text or as structured changes with an optional patch, and a
// plan is a plan whether its entries were top level or nested under the plan.
// ----------------------------------------------------------------------------

func toolUpdateMetadata(notif acpSessionNotification) map[string]interface{} {
	update := notif.Update
	meta := map[string]interface{}{
		"source_update_type": update.SessionUpdate,
		"content_type":       update.Content.Type,
		"protocol":           "acp",
		"protocol_method":    "session/update",
		"acp": map[string]interface{}{
			"session_id": notif.SessionID, "session_update": update.SessionUpdate,
			"tool_call_id": update.ToolCallID, "tool_name": update.Name, "tool_kind": update.Kind,
			"status": update.Status, "raw_input": update.RawInput, "raw_output": update.RawOutput,
			"locations":      update.Locations,
			"content":        map[string]interface{}{"type": update.Content.Type, "text": updateContentText(update)},
			"content_blocks": update.Contents, "tool_contents": update.ToolContents,
			"title": update.Title, "updated_at": update.UpdatedAt, "_meta": update.Meta,
		},
	}
	addOptionalToolMetadata(meta, notif)
	addToolContentProjection(meta, update.ToolContents)
	for k, v := range update.Meta {
		meta[k] = v
	}
	return meta
}

// addOptionalToolMetadata surfaces the identity fields a caller filters on,
// leaving out the ones this update did not carry. A version 2 tool_call_update
// patches the tool call, so an omitted field means "unchanged" rather than
// "empty" and must not overwrite what an earlier update reported.
func addOptionalToolMetadata(meta map[string]interface{}, notif acpSessionNotification) {
	update := notif.Update
	for key, value := range map[string]string{
		"title": update.Title, "remote_session_id": notif.SessionID, "tool_call_id": update.ToolCallID,
		"tool_name": update.Name, "status": update.Status,
	} {
		if strings.TrimSpace(value) != "" {
			meta[key] = value
		}
	}
	if kind := strings.TrimSpace(update.Kind); kind != "" {
		meta["tool_kind"], meta["acp_tool_kind"] = kind, kind
	}
	if len(update.RawInput) > 0 {
		meta["raw_input"] = update.RawInput
	}
	if update.RawOutput != nil {
		meta["raw_output"] = update.RawOutput
	}
	if len(update.Locations) > 0 {
		meta["locations"] = update.Locations
	}
}

func structuralUpdateMetadata(notif acpSessionNotification) map[string]interface{} {
	update := notif.Update
	entries := update.PlanEntries()
	acp := map[string]interface{}{
		"session_id": notif.SessionID, "session_update": update.SessionUpdate,
		"entries": entries, "available_commands": update.AvailableCommands,
		"current_mode_id": update.CurrentModeID, "config_options": update.ConfigOptions,
		"usage": update.Usage, "title": update.Title, "updated_at": update.UpdatedAt,
		"_meta": update.Meta,
	}
	meta := map[string]interface{}{
		"source_update_type": update.SessionUpdate,
		"protocol":           "acp",
		"protocol_method":    "session/update",
		"acp":                acp,
	}
	if update.Plan != nil && strings.TrimSpace(update.Plan.PlanID) != "" {
		// Version 2 identifies a plan by id and replaces it whole on every
		// update, so the id is what tells two plans apart.
		acp["plan_id"], meta["plan_id"] = update.Plan.PlanID, update.Plan.PlanID
	}
	if len(entries) > 0 {
		meta["plan_entries"] = entries
	}
	if len(update.AvailableCommands) > 0 {
		meta["available_commands"] = update.AvailableCommands
	}
	if len(update.ConfigOptions) > 0 {
		meta["config_options"] = update.ConfigOptions
	}
	if len(update.Usage) > 0 {
		meta["usage"] = update.Usage
	}
	if strings.TrimSpace(update.CurrentModeID) != "" {
		meta["current_mode_id"] = update.CurrentModeID
	}
	if strings.TrimSpace(update.Title) != "" {
		meta["title"] = update.Title
	}
	if strings.TrimSpace(update.UpdatedAt) != "" {
		meta["updated_at"] = update.UpdatedAt
	}
	for k, v := range update.Meta {
		meta[k] = v
	}
	return meta
}

// terminalSummary is the one-line description of a version 2 agent-owned
// terminal update, for the live surface.
func terminalSummary(update acpSessionUpdate) string {
	switch {
	case update.SessionUpdate == "terminal_output_chunk":
		return "terminal output " + update.TerminalID
	case update.Command != nil:
		return "terminal " + update.TerminalID + ": " + *update.Command
	default:
		return "terminal " + update.TerminalID
	}
}

// terminalUpdateMetadata projects an agent-owned terminal onto the structured
// surface. Matrix holds no terminal of its own here: the terminal belongs to the
// peer, so what is projected is its identity, the command that produced it, its
// exit status and the size of its output, never the raw bytes as turn content.
func terminalUpdateMetadata(notif acpSessionNotification) map[string]interface{} {
	update := notif.Update
	acp := map[string]interface{}{
		"session_id": notif.SessionID, "session_update": update.SessionUpdate,
		"terminal_id": update.TerminalID, "command": update.Command, "cwd": update.Cwd,
		"output": update.Output, "exit_status": update.ExitStatus, "_meta": update.Meta,
	}
	meta := map[string]interface{}{
		"source_update_type": update.SessionUpdate,
		"protocol":           "acp",
		"protocol_method":    "session/update",
		"terminal_id":        update.TerminalID,
		"acp":                acp,
	}
	if update.Command != nil {
		meta["command"] = *update.Command
	}
	if update.ExitStatus != nil {
		meta["exit_status"] = update.ExitStatus
	}
	if update.Output != nil {
		meta["terminal_output_bytes"] = len(update.Output.Data)
	}
	if update.Data != "" {
		meta["terminal_output_chunk_bytes"] = len(update.Data)
	}
	for k, v := range update.Meta {
		meta[k] = v
	}
	return meta
}

func updateContentText(update acpSessionUpdate) string {
	if update.Content.Text != "" {
		return update.Content.Text
	}
	if len(update.Contents) == 0 && len(update.ToolContents) == 0 {
		return ""
	}
	var b strings.Builder
	for _, content := range update.Contents {
		if strings.TrimSpace(content.Text) == "" {
			continue
		}
		b.WriteString(content.Text)
	}
	for _, item := range update.ToolContents {
		if item.Content == nil || strings.TrimSpace(item.Content.Text) == "" {
			continue
		}
		b.WriteString(item.Content.Text)
	}
	return b.String()
}

func addToolContentProjection(meta map[string]interface{}, contents []acpToolCallContent) {
	if len(contents) == 0 {
		return
	}
	meta["tool_contents"] = contents
	var types []string
	for _, content := range contents {
		if strings.TrimSpace(content.Type) != "" {
			types = append(types, content.Type)
		}
		if strings.TrimSpace(content.Path) != "" && meta["path"] == nil {
			meta["path"] = content.Path
		}
		if strings.TrimSpace(content.TerminalID) != "" {
			meta["terminal_id"] = content.TerminalID
		}
		addToolDiffProjection(meta, content)
	}
	if len(types) > 0 {
		meta["tool_content_types"] = types
	}
}

// addToolDiffProjection projects a diff's affected files and renderable patch.
// Version 2 describes a diff as structured changes plus an optional patch, where
// version 1 carried a single path with old and new text; both reach the same
// neutral keys, and the first affected path is the one a caller displays.
func addToolDiffProjection(meta map[string]interface{}, content acpToolCallContent) {
	for _, change := range content.Changes {
		meta["diff_changes"] = appendToolChange(meta["diff_changes"], change)
		if strings.TrimSpace(change.Path) != "" && meta["path"] == nil {
			meta["path"] = change.Path
		}
	}
	if content.Patch != nil {
		meta["diff_patch_format"] = content.Patch.Format
		meta["diff_patch"] = content.Patch.Text
	}
}

func appendToolChange(existing interface{}, change acpDiffChange) []acpDiffChange {
	changes, _ := existing.([]acpDiffChange)
	return append(changes, change)
}
