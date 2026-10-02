package runtrace

func (s *Store) Trace(runID string) (Trace, bool, error) {
	run, found, err := s.LoadRun(runID)
	if err != nil || !found {
		return Trace{}, found, err
	}
	events, err := s.LoadEvents(runID, 0)
	if err != nil {
		return Trace{}, false, err
	}
	// The elicitation lifecycle lives in the notification stream, not in the
	// events, so the trace reads it too. The limit bounds the read: a run with
	// more than this many notifications has long since stopped being a run whose
	// stall view fits in a diagnostic answer.
	notifications, _, err := s.LoadNotificationsAfter(0, maxTraceNotifications, map[string]struct{}{runID: {}})
	if err != nil {
		return Trace{}, false, err
	}
	return Project(run, events, notifications), true, nil
}

// maxTraceNotifications bounds how much of a run's notification stream a trace
// projection reads.
const maxTraceNotifications = 100

func Project(run Run, events []Event, notifications []Notification) Trace {
	contentRef := run.InputRef
	if contentRef == "" {
		contentRef = "matrix://runs/" + run.ID + "/input"
	}
	// The stall view is derived from the raw events, before the trace policy
	// strips them. It has to be: session attribution lives in protocol metadata
	// and tool names are dropped in redacted mode, so a view computed after the
	// policy would go blind on exactly the runs an operator is diagnosing.
	// What the view may not do is carry content past the policy: the one piece
	// of user text it can hold is the question a person is being asked, and that
	// is dropped by the same rule that drops an event message.
	stall := ObserveStall(run, events, notifications)
	if !includesContent(run.TracePolicy) {
		stall = withoutStallQuestion(stall)
	}
	events = applyTracePolicy(events, run.TracePolicy)
	outcome := Outcome{Status: run.Status, StopReason: run.StopReason, SummaryRef: run.OutputRef, Error: run.Error}
	if includesContent(run.TracePolicy) {
		outcome.Summary = run.Output
	}
	return Trace{
		Schema:      SchemaAgentCommunicationRunTraceV0,
		Run:         projectRun(run),
		Surface:     projectSurface(run, contentRef),
		Routing:     projectRouting(run),
		Events:      events,
		Outcome:     outcome,
		Stall:       &stall,
		TracePolicy: run.TracePolicy,
		Context:     run.Context,
	}
}

func applyTracePolicy(events []Event, policy TracePolicy) []Event {
	out := make([]Event, len(events))
	for i, event := range events {
		out[i] = applyEventTracePolicy(event, policy)
	}
	return out
}

// includesContent is the single rule for whether a policy lets text through. An
// event message and the question a run is blocked on are the same kind of thing
// - text a person or a peer wrote - so they answer to one condition rather than
// two that can drift apart, and a policy that gains a mode gains it for both.
func includesContent(policy TracePolicy) bool {
	return policy.ContentMode == ContentModeInline
}

// withoutStallQuestion drops the user content a stall view can carry, at both
// places a request appears: the view's own list and the per-session lists. What
// is left is the picture the view was built for - who is blocked, on what, since
// when - with the text the person was shown removed.
func withoutStallQuestion(stall StallView) StallView {
	for i := range stall.Pending {
		stall.Pending[i] = withoutQuestion(stall.Pending[i])
	}
	for i := range stall.Sessions {
		for j := range stall.Sessions[i].Pending {
			stall.Sessions[i].Pending[j] = withoutQuestion(stall.Sessions[i].Pending[j])
		}
	}
	return stall
}

func withoutQuestion(request StallRequest) StallRequest {
	request.Question = ""
	request.QuestionTruncated = false
	return request
}

func applyEventTracePolicy(event Event, policy TracePolicy) Event {
	if !policy.IncludeProtocolMeta {
		event.ProtocolMeta = nil
	}
	if !includesContent(policy) {
		event.Message = ""
	}
	if policy.ContentMode == ContentModeRedacted {
		event.ToolName = ""
		event.ToolKind = ""
		event.ToolSemanticKind = ""
		event.ToolEffect = ""
		event.ToolSubjectKind = ""
		event.ToolClassificationSource = ""
		event.ToolClassificationConfidence = ""
		event.Summary = ""
		event.Inputs = nil
		event.Outputs = nil
		event.ArtifactRefs = nil
		event.Metadata = redactedEventMetadata(event)
	}
	return event
}

func redactedEventMetadata(event Event) map[string]interface{} {
	if event.Kind != "sidecar.capsule.delivered" {
		return nil
	}
	out := map[string]interface{}{}
	for _, key := range []string{"frontend_visible", "audit_visible", "trace_visible"} {
		if value, ok := event.Metadata[key]; ok {
			out[key] = value
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func projectRun(run Run) TraceRun {
	return TraceRun{
		ID:                  run.ID,
		AgentID:             run.AgentID,
		RequestedModel:      run.RequestedModel,
		ConfiguredModel:     run.ConfiguredModel,
		EffectiveModel:      run.EffectiveModel,
		ModelVerification:   run.ModelVerification,
		ModelFallbackUsed:   run.ModelFallbackUsed,
		ModelFallbackReason: run.ModelFallbackReason,
		Protocol:            run.Protocol,
		WorkspaceID:         run.WorkspaceID,
		LogicalSessionID:    run.LogicalSessionID,
		RemoteSessionID:     run.RemoteSessionID,
		StartedAt:           run.StartedAt,
		CompletedAt:         run.CompletedAt,
		Status:              run.Status,
		StopReason:          run.StopReason,
	}
}

func projectSurface(run Run, contentRef string) Surface {
	return Surface{
		Channel:       run.ChannelID,
		InputKind:     run.InputKind,
		ContentRef:    contentRef,
		ContentDigest: run.InputDigest,
		Redaction:     redactionFor(run.TracePolicy.ContentMode),
	}
}

func projectRouting(run Run) Routing {
	return Routing{
		SelectedAgentID:    run.AgentID,
		SelectedSessionID:  run.LogicalSessionID,
		SelectedProtocol:   run.Protocol,
		SelectedWorkspace:  run.WorkspaceID,
		SelectedRemoteSess: run.RemoteSessionID,
	}
}
