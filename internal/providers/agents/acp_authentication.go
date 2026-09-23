package agents

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/Josepavese/matrix/internal/middleware"
	"github.com/Josepavese/matrix/pkg/zedacp"
)

// ----------------------------------------------------------------------------
// ACP v2 authentication
//
// ACP version 2 makes an authentication method's type discriminator mandatory
// and defines the structured auth_required error for requests that are gated
// behind a login. Matrix runs both standard types:
//
//   - "agent": auth/login carries the method's identifier and the agent performs
//     the login over the protocol.
//   - "terminal": Matrix launches the configured agent program itself, with the
//     method's args and env, waits for it to exit, then reconnects and
//     reinitializes the connection. auth/login is never sent for it.
//
// Running a terminal method is a claim about what Matrix can reproduce, so it is
// opt-in, it follows the same predicate as the initialize-time capability, and it
// is only offered on a connection that actually negotiated version 2.
// ----------------------------------------------------------------------------

// terminalAuthEnabled is the single predicate behind both the initialize-time
// advertisement and the runtime offer to run a terminal method. Both halves are
// required: an operator opt-in, because advertising the capability changes what
// a real agent offers, and a process backend, because without one Matrix cannot
// reproduce the configured invocation at all.
func terminalAuthEnabled(deps middleware.ConversationFactoryDeps) bool {
	return deps.TerminalAuth && deps.Process != nil
}

// terminalAuthAdvertisement derives capabilities.auth.terminal from that same
// predicate, so the advertised capability and the runnable surface cannot
// diverge: Matrix never claims a flow it would refuse to run.
func terminalAuthAdvertisement(deps middleware.ConversationFactoryDeps) *zedacp.AuthCapabilities {
	if !terminalAuthEnabled(deps) {
		return nil
	}
	return &zedacp.AuthCapabilities{Terminal: &zedacp.TerminalAuthCapabilities{}}
}

// acpInitializeRequestFor is the initialize request every connection Matrix
// opens sends, including the one a terminal login reconnects with. Keeping it in
// one place is what makes a reconnected connection negotiate exactly like the
// first one, capability advertisement included.
func acpInitializeRequestFor(deps middleware.ConversationFactoryDeps) acpInitializeRequest {
	return acpInitializeRequest{
		// Zero asks the client to negotiate: it requests the highest version
		// Matrix supports and drops to v1 if the agent refuses it.
		ProtocolVersion:    0,
		ClientInfo:         map[string]interface{}{"name": "matrix", "version": "1.0"},
		ClientCapabilities: acpClientCapabilitiesForDeps(deps),
	}
}

// initializeACPConnection performs the version-negotiating handshake.
func initializeACPConnection(ctx context.Context, client acpClient, deps middleware.ConversationFactoryDeps) (*acpInitializeResponse, error) {
	resp, err := client.Initialize(ctx, acpInitializeRequestFor(deps))
	if err != nil {
		return nil, fmt.Errorf("ACP initialize failed: %w", err)
	}
	if err := validateNegotiatedProtocolVersion(client); err != nil {
		return nil, err
	}
	return resp, nil
}

// validateNegotiatedProtocolVersion fails closed when the connection agreed on a
// generation Matrix cannot speak. It reads the version the client recorded while
// negotiating rather than the raw response field: a v1 agent may omit the field
// entirely, and an agent that declares nothing is a v1 agent, not an unsupported
// one.
func validateNegotiatedProtocolVersion(client acpClient) error {
	agreed := client.AuthenticatedProtocolVersion()
	if agreed < supportedACPProtocolVersion || agreed > zedacp.MaxSupportedProtocolVersion {
		return fmt.Errorf("ACP protocol version %d is not supported (matrix supports %d..%d)",
			agreed, supportedACPProtocolVersion, zedacp.MaxSupportedProtocolVersion)
	}
	return nil
}

// negotiatedProtocolVersion reports the generation this connection speaks. Zero
// means initialize has not completed, which is never version 2.
func (c *acpConversationClient) negotiatedProtocolVersion() int {
	client := c.currentACPClient()
	if client == nil {
		return 0
	}
	return client.AuthenticatedProtocolVersion()
}

// terminalAuthenticationOffered reports whether a terminal method may be offered
// on this connection: the agent agreed to version 2, where the type exists, and
// Matrix both opted in and has a process backend to run it with.
func (c *acpConversationClient) terminalAuthenticationOffered() bool {
	return terminalAuthEnabled(c.deps) && c.negotiatedProtocolVersion() >= zedacp.ProtocolVersionV2
}

// AuthenticationMethods reports the advertised methods Matrix can run. The
// "agent" type is always runnable, and so is a missing discriminator, which is
// how v1 agents describe one. A v2 "terminal" method is offered only on a v2
// connection that advertised the capability. A custom "_" type is reported as
// the agent declared it, because Matrix must not invent semantics for it, and an
// unknown non-underscore type is reserved for a future protocol variant and
// stays out.
func (c *acpConversationClient) AuthenticationMethods() []middleware.AuthenticationMethod {
	methods := c.currentAuthMethods()
	out := make([]middleware.AuthenticationMethod, 0, len(methods))
	for _, method := range methods {
		methodType, ok := c.runnableAuthenticationType(method)
		if !ok {
			continue
		}
		out = append(out, middleware.AuthenticationMethod{
			ID:          method.Identifier(),
			Type:        methodType,
			Name:        method.Name,
			Description: method.Description,
			Metadata:    method.Meta,
		})
	}
	return out
}

func (c *acpConversationClient) runnableAuthenticationType(method acpAuthMethod) (string, bool) {
	switch methodType := method.MethodType(); {
	case methodType == "" || methodType == zedacp.AuthMethodTypeAgent:
		return zedacp.AuthMethodTypeAgent, true
	case methodType == zedacp.AuthMethodTypeTerminal:
		if !c.terminalAuthenticationOffered() {
			return "", false
		}
		return zedacp.AuthMethodTypeTerminal, true
	case method.IsCustom():
		return methodType, true
	default:
		return "", false
	}
}

// advertisedAuthMethod finds one of the agent's own entries by the identifier
// Matrix published, so the launch configuration is read from the agent's payload
// and never reconstructed from the neutral projection.
func (c *acpConversationClient) advertisedAuthMethod(methodID string) (acpAuthMethod, bool) {
	for _, method := range c.currentAuthMethods() {
		if method.Identifier() == methodID {
			return method, true
		}
	}
	return acpAuthMethod{}, false
}

// Authenticate runs the advertised method through the flow its type defines. A
// method Matrix did not offer cannot be run, so an unsupported or filtered-out
// identifier fails closed instead of reaching the wire.
func (c *acpConversationClient) Authenticate(ctx context.Context, methodID string) error {
	methodID = strings.TrimSpace(methodID)
	offered, ok := c.offeredAuthenticationMethod(methodID)
	if !ok {
		return fmt.Errorf("ACP agent does not advertise stable authentication method %q", methodID)
	}
	switch offered.Type {
	case zedacp.AuthMethodTypeAgent:
		return c.currentACPClient().Authenticate(ctx, methodID)
	case zedacp.AuthMethodTypeTerminal:
		return c.runTerminalAuthentication(ctx, methodID)
	default:
		return fmt.Errorf("ACP authentication method %q has type %q, which Matrix cannot run", methodID, offered.Type)
	}
}

func (c *acpConversationClient) offeredAuthenticationMethod(methodID string) (middleware.AuthenticationMethod, bool) {
	for _, method := range c.AuthenticationMethods() {
		if method.ID == methodID {
			return method, true
		}
	}
	return middleware.AuthenticationMethod{}, false
}

// runTerminalAuthentication reproduces the configured agent invocation: the same
// program and base launch configuration as the ACP connection, with the method's
// args appended and its env applied over the base env, so a same-named entry from
// the method wins. Exit status zero is success; a non-zero status, termination
// without an exit status, or cancellation is a failure. Nothing is sent to the
// agent before the process runs, because ACP forbids auth/login for a terminal
// method: the login happens inside the process.
func (c *acpConversationClient) runTerminalAuthentication(ctx context.Context, methodID string) error {
	method, ok := c.advertisedAuthMethod(methodID)
	if !ok {
		return fmt.Errorf("ACP agent does not advertise authentication method %q", methodID)
	}
	if strings.TrimSpace(c.endpoint.Command) == "" {
		return fmt.Errorf("ACP terminal authentication %q requires a configured agent program to launch", methodID)
	}
	log := slog.With("component", "acp_adapter", "agent", c.deps.AgentID, "method", methodID)
	spec := terminalAuthenticationCommand(c.endpoint, method)
	log.Info("running ACP terminal authentication", "event", "acp_terminal_auth_start", "command", spec.Runner, "args", spec.Args)
	result, err := c.deps.Process.ExecSeparate(ctx, spec)
	if err != nil {
		return fmt.Errorf("ACP terminal authentication %q failed to run: %w", methodID, err)
	}
	if result == nil {
		return fmt.Errorf("ACP terminal authentication %q reported no exit status", methodID)
	}
	if result.ExitCode != 0 {
		return fmt.Errorf("ACP terminal authentication %q exited with status %d", methodID, result.ExitCode)
	}
	log.Info("ACP terminal authentication completed", "event", "acp_terminal_auth_completed")
	if c.reconnect == nil {
		return fmt.Errorf("ACP terminal authentication %q succeeded but no reconnection is configured", methodID)
	}
	if err := c.reconnect(ctx); err != nil {
		return fmt.Errorf("ACP terminal authentication %q succeeded but reconnecting failed: %w", methodID, err)
	}
	return nil
}

// terminalAuthenticationCommand derives the launch spec from the endpoint that
// configured the ACP connection. EnvIsolation is handed to the process provider
// rather than applied here, so the login process is launched exactly the way the
// ACP connection itself is.
func terminalAuthenticationCommand(endpoint middleware.ProtocolEndpoint, method acpAuthMethod) middleware.CommandSpec {
	return middleware.CommandSpec{
		Runner:       endpoint.Command,
		Args:         append(append([]string{}, endpoint.Args...), method.Args...),
		Env:          mergeAuthenticationEnv(endpoint.Env, method.Env),
		EnvIsolation: endpoint.EnvIsolation,
	}
}

// mergeAuthenticationEnv applies the method's environment over the base launch
// configuration, with the method winning on a same-named variable. Entries
// without a name are dropped: ACP requires every entry to name a variable, and a
// nameless one would otherwise become a malformed "=value" pair.
func mergeAuthenticationEnv(base []string, overrides []zedacp.EnvVar) []string {
	merged := make([]string, 0, len(base)+len(overrides))
	for _, entry := range base {
		name, _, _ := strings.Cut(entry, "=")
		if !authenticationEnvOverridden(name, overrides) {
			merged = append(merged, entry)
		}
	}
	for _, override := range overrides {
		if strings.TrimSpace(override.Name) == "" {
			continue
		}
		merged = append(merged, override.Name+"="+override.Value)
	}
	return merged
}

func authenticationEnvOverridden(name string, overrides []zedacp.EnvVar) bool {
	for _, override := range overrides {
		if override.Name == name {
			return true
		}
	}
	return false
}

// reconnect rebuilds the ACP connection after a terminal login. The login process
// may have written credentials the already-running agent only reads at startup,
// so ACP requires a reconnect and a fresh initialize before the gated operation
// is retried. It is a field because the connection is the seam between this
// adapter and the transport it was built on, exactly like the SDK port; the
// factory wires it to reconnectACPConnection, and a client that has no way to
// reconnect fails the login rather than pretending the old connection is new.
func (c *acpConversationClient) reconnectACPConnection(ctx context.Context) error {
	transport, err := createTransport(ctx, c.connectionTransportSpec())
	if err != nil {
		return err
	}
	client := NewACPClient(ctx, transport)
	client.SetRequestHandler(c.handler)
	resp, err := initializeACPConnection(ctx, client, c.deps)
	if err != nil {
		_ = transport.Close()
		return err
	}
	c.replaceACPClient(client, resp)
	return nil
}

func (c *acpConversationClient) connectionTransportSpec() transportSpec {
	return transportSpec{
		Protocol:     c.endpoint.Transport,
		Address:      c.endpoint.Address,
		Command:      c.endpoint.Command,
		Args:         c.endpoint.Args,
		Env:          c.endpoint.Env,
		EnvIsolation: c.endpoint.EnvIsolation,
	}
}

// replaceACPClient swaps in the reconnected client and the methods its initialize
// response advertised. Reads go through currentACPClient because a concurrent
// turn may still be holding the previous connection, which is closed here once
// every in-flight request has had its chance to fail on the old transport rather
// than on a nil field.
func (c *acpConversationClient) replaceACPClient(client acpClient, resp *acpInitializeResponse) {
	c.mu.Lock()
	previous := c.client
	c.client = client
	if resp != nil {
		c.authMethods = append([]acpAuthMethod(nil), resp.AuthMethods...)
	}
	c.mu.Unlock()
	if previous != nil && previous != client {
		_ = previous.Close()
	}
}

// currentACPClient returns the client for the currently connected generation.
func (c *acpConversationClient) currentACPClient() acpClient {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.client
}

// currentAuthMethods returns the methods advertised by the current connection,
// copied so a caller can iterate without holding the lock a reconnect needs.
func (c *acpConversationClient) currentAuthMethods() []acpAuthMethod {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]acpAuthMethod(nil), c.authMethods...)
}

// withAuthenticationRetry runs op and, when the agent answers with ACP v2's
// structured auth_required marker, performs one login and runs op exactly once
// more. The bound is deliberate: an authentication retry is not a loop, and a
// second failure has to reach the caller — wrapped, so the original signal stays
// visible — instead of being retried away.
func (c *acpConversationClient) withAuthenticationRetry(ctx context.Context, op func() error) error {
	err := op()
	if err == nil || !zedacp.IsAuthenticationRequired(err) {
		return err
	}
	method, ok := c.retryAuthenticationMethod()
	if !ok {
		return fmt.Errorf("%w (the agent advertises no authentication method Matrix can run)", err)
	}
	if authErr := c.Authenticate(ctx, method.ID); authErr != nil {
		return fmt.Errorf("authentication required; method %q failed: %w", method.ID, authErr)
	}
	if retryErr := op(); retryErr != nil {
		return fmt.Errorf("retry after authentication with method %q failed: %w", method.ID, retryErr)
	}
	return nil
}

// retryAuthenticationMethod picks the method an automatic retry may run. An
// "agent" method comes first because it needs no user: the agent performs the
// login over the protocol. A "terminal" method is only reachable here when the
// operator opted into terminal authentication, since a terminal login runs the
// agent program and is, by design, an interactive flow. A custom type is never
// run automatically.
func (c *acpConversationClient) retryAuthenticationMethod() (middleware.AuthenticationMethod, bool) {
	methods := c.AuthenticationMethods()
	for _, method := range methods {
		if method.Type == zedacp.AuthMethodTypeAgent {
			return method, true
		}
	}
	for _, method := range methods {
		if method.Type == zedacp.AuthMethodTypeTerminal {
			return method, true
		}
	}
	return middleware.AuthenticationMethod{}, false
}

// ExecuteTurn runs one conversation turn with the authentication retry around
// it: a request the agent gated behind a login is logged in and retried once
// instead of surfacing as a provider failure.
func (c *acpConversationClient) ExecuteTurn(ctx context.Context, turn middleware.ConversationTurn) (middleware.ConversationResult, error) {
	var result middleware.ConversationResult
	err := c.withAuthenticationRetry(ctx, func() error {
		var turnErr error
		result, turnErr = c.executeTurnOnce(ctx, turn)
		return turnErr
	})
	return result, err
}
