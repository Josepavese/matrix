package zedacp

// SessionSurface is the session method surface an initialize response
// advertised.
//
// The two protocol generations describe it differently. Version 2 nests the
// session capability groups under capabilities.session and makes the baseline
// methods — session/new, session/list, session/resume, session/close,
// session/prompt, session/cancel and session/update — implicit in that object
// being present, with only the optional extras (delete, fork,
// additionalDirectories, prompt, mcp) keeping their own markers. Version 1
// marks each method separately and has no session-surface marker at all, so its
// baseline is the methods the generation always had. Resolving both shapes here
// keeps that difference inside the protocol package instead of leaking two
// spellings into every adapter that asks whether a peer can list or resume.
type SessionSurface struct {
	Advertised, Load, List, Resume, Close, Delete, Fork, AdditionalDirectories bool
	// SystemPrompt reports whether the agent accepts a client-provided system
	// prompt on session lifecycle requests. SystemPromptOverride reports the
	// separately advertised replacement mode; append is implied by
	// SystemPrompt itself.
	SystemPrompt, SystemPromptOverride bool
}

// SessionSurface resolves what the initialize response advertised, reading the
// shape of the generation that answered. A version 2 agent that omits
// capabilities.session advertises no session surface, and session/load does not
// exist in version 2, so Load is only ever true for a version 1 agent.
func (r *InitializeResponse) SessionSurface() SessionSurface {
	if r == nil || r.Capabilities == nil {
		return SessionSurface{}
	}
	if r.ProtocolVersion >= ProtocolVersionV2 {
		session, _ := r.Capabilities["session"].(map[string]interface{})
		baseline := session != nil
		systemPrompt, override := sessionSystemPrompt(session)
		return SessionSurface{Advertised: baseline, List: baseline, Resume: baseline, Close: baseline,
			Delete: sessionMarker(session, "delete"), Fork: sessionMarker(session, "fork"),
			AdditionalDirectories: sessionMarker(session, "additionalDirectories"),
			SystemPrompt:          systemPrompt, SystemPromptOverride: override}
	}
	caps, _ := r.Capabilities["sessionCapabilities"].(map[string]interface{})
	load, _ := r.Capabilities["loadSession"].(bool)
	systemPrompt, override := sessionSystemPrompt(caps)
	return SessionSurface{Advertised: true, Load: load,
		List: sessionMarker(caps, "list"), Resume: sessionMarker(caps, "resume"), Close: sessionMarker(caps, "close"),
		Delete: sessionMarker(caps, "delete"), Fork: sessionMarker(caps, "fork"), AdditionalDirectories: sessionMarker(caps, "additionalDirectories"),
		SystemPrompt: systemPrompt, SystemPromptOverride: override}
}

// sessionSystemPrompt reads the stable system-prompt capability: the presence of
// the group means append mode, and a nested non-null mode.override object means
// the replacement mode was advertised too. Both generations nest the group at
// the same place inside their session capability object.
func sessionSystemPrompt(group map[string]interface{}) (supported, override bool) {
	prompt, ok := group["systemPrompt"].(map[string]interface{})
	if !ok {
		return false, false
	}
	mode, _ := prompt["mode"].(map[string]interface{})
	_, override = mode["override"].(map[string]interface{})
	return true, override
}

// sessionMarker reports whether a capability group carries a support marker,
// which both generations define as the presence of a non-null object.
func sessionMarker(group map[string]interface{}, name string) bool {
	_, ok := group[name].(map[string]interface{})
	return ok
}
