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
	return Project(run, events), true, nil
}

func Project(run Run, events []Event) Trace {
	contentRef := run.InputRef
	if contentRef == "" {
		contentRef = "matrix://runs/" + run.ID + "/input"
	}
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
		event.Metadata = nil
	}
	return event
}

func projectRun(run Run) TraceRun {
	return TraceRun{
		ID:               run.ID,
		AgentID:          run.AgentID,
		Protocol:         run.Protocol,
		WorkspaceID:      run.WorkspaceID,
		LogicalSessionID: run.LogicalSessionID,
		RemoteSessionID:  run.RemoteSessionID,
		StartedAt:        run.StartedAt,
		CompletedAt:      run.CompletedAt,
		Status:           run.Status,
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
