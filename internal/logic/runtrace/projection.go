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
	stall := ObserveStall(run, events, notifications)
	events = applyTracePolicy(events, run.TracePolicy)
	outcome := Outcome{Status: run.Status, StopReason: run.StopReason, SummaryRef: run.OutputRef, Error: run.Error}
	if run.TracePolicy.ContentMode == ContentModeInline {
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

func applyEventTracePolicy(event Event, policy TracePolicy) Event {
	if !policy.IncludeProtocolMeta {
		event.ProtocolMeta = nil
	}
	if policy.ContentMode != ContentModeInline {
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
