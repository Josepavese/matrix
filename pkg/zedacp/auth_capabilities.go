package zedacp

import "strings"

// ACP version 2 makes an authentication method's type discriminator mandatory and
// reserves the shape of every value Matrix does not mint itself:
//
//   - "agent" is the standard type. The client asks the agent to log in through
//     the protocol, with auth/login carrying the method's identifier.
//   - "terminal" instructs the client to run the configured agent program itself,
//     interactively, and never to send auth/login for that method.
//   - every custom type MUST begin with "_"; a client that does not understand
//     one must carry it through generically instead of inventing semantics.
//   - unknown values without the "_" prefix are reserved for future ACP versions,
//     so a client must not guess at them.
const (
	AuthMethodTypeAgent    = "agent"
	AuthMethodTypeTerminal = "terminal"
	// CustomAuthMethodTypePrefix marks method types that belong to an
	// implementation rather than to ACP itself.
	CustomAuthMethodTypePrefix = "_"
)

// MethodType returns the method's discriminator in the shape every comparison
// wants: lower-cased and trimmed, with the empty string meaning "no type was
// sent", which is how version 1 agents describe an agent-handled login.
func (m AuthMethod) MethodType() string {
	return strings.ToLower(strings.TrimSpace(m.Type))
}

// IsCustom reports whether the method carries an implementation-defined type.
// Such a method is advertised but never run: only the implementation that minted
// the value knows what it means.
func (m AuthMethod) IsCustom() bool {
	return strings.HasPrefix(m.MethodType(), CustomAuthMethodTypePrefix)
}

// AuthCapabilities advertises authentication-related client extensions. It is
// orthogonal to the baseline auth/login and auth/logout methods: an agent that
// returns a non-empty authMethods list implements those whether or not this
// object is present, and one that returns none must not be asked to.
type AuthCapabilities struct {
	Terminal *TerminalAuthCapabilities `json:"terminal,omitempty"`
	Meta     map[string]interface{}    `json:"_meta,omitempty"`
}

// TerminalAuthCapabilities advertises that the client can reproduce the
// configured agent invocation in an interactive terminal. Its presence is the
// whole capability, which is why the object is empty: an agent may advertise a
// terminal method only when the client sent this during initialize.
type TerminalAuthCapabilities struct {
	Meta map[string]interface{} `json:"_meta,omitempty"`
}
