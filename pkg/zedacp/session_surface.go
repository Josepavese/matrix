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
		return SessionSurface{Advertised: baseline, List: baseline, Resume: baseline, Close: baseline,
			Delete: sessionMarker(session, "delete"), Fork: sessionMarker(session, "fork"),
			AdditionalDirectories: sessionMarker(session, "additionalDirectories")}
	}
	caps, _ := r.Capabilities["sessionCapabilities"].(map[string]interface{})
	load, _ := r.Capabilities["loadSession"].(bool)
	return SessionSurface{Advertised: true, Load: load,
		List: sessionMarker(caps, "list"), Resume: sessionMarker(caps, "resume"), Close: sessionMarker(caps, "close"),
		Delete: sessionMarker(caps, "delete"), Fork: sessionMarker(caps, "fork"), AdditionalDirectories: sessionMarker(caps, "additionalDirectories")}
}

// sessionMarker reports whether a capability group carries a support marker,
// which both generations define as the presence of a non-null object.
func sessionMarker(group map[string]interface{}, name string) bool {
	_, ok := group[name].(map[string]interface{})
	return ok
}
