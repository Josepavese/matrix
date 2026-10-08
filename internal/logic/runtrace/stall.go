package runtrace

import "time"

// Wait vocabulary: what a run, or one of its sessions, is waiting on now.
//
// A pending request and the wait it causes use the same two words on purpose. A
// run whose oldest unresolved request is a tool call is waiting for that tool's
// result, and calling those two facts different things would let a consumer
// compare a "waiting" value against a "pending" value that never match.
const (
	// WaitNone is a terminal run: nothing is being waited on because nothing
	// will arrive.
	WaitNone = "none"
	// WaitToolResult is a tool call Matrix observed requested and never
	// observed answered.
	WaitToolResult = "tool_result"
	// WaitPermission is a permission request Matrix observed opened and never
	// observed resolved.
	//
	// With today's recorder this wait is nearly unreachable, and that is worth
	// knowing rather than discovering from an empty field: the notifier writes
	// permission.requested and permission.resolved from the same update, after
	// the decision is already made, so the pair normally closes in the same
	// breath. What this wait does report is the abnormal case — a decision the
	// process never got to write, which is exactly when an operator needs to
	// know the run stopped between a request and its answer.
	//
	// If that recorder ever starts publishing the request before the decision is
	// made, this wait starts reporting pending approvals on its own: nothing here
	// has to change, which is the reason to keep the rule rather than delete it.
	// Whoever touches that recorder should know that this is what lights up.
	WaitPermission = "permission"
	// WaitElicitation is an elicitation Matrix recorded opened and never
	// recorded resolved: the run is blocked on a human answer or approval.
	//
	// It is a word of its own rather than a reuse of WaitPermission because the
	// two are different mechanisms with different evidence. A permission is an
	// ACP decision Matrix makes and writes down already decided; an elicitation
	// is a question the run cannot answer itself, and it is the one wait that a
	// person, not a peer, is holding up.
	WaitElicitation = "elicitation"
	// WaitProviderTurn is a run with no outstanding request: whatever moves it
	// next has to come from the peer.
	WaitProviderTurn = "provider_turn"
	// WaitUnknown is a live run with nothing outstanding and no observed
	// activity to reason from. Matrix says so instead of guessing.
	WaitUnknown = "unknown"
)

// Event kinds the stall view reads. These are the structural events Matrix
// already records; the view adds no new wire vocabulary and no new event.
const (
	KindPromptSent          = "agent.prompt.sent"
	KindToolCallRequested   = "tool.call.requested"
	KindToolResultReceived  = "tool.result.received"
	KindPermissionRequested = "permission.requested"
	KindPermissionResolved  = "permission.resolved"
	// The elicitation lifecycle is recorded as notifications, not as events:
	// runapi observes the elicitation service and appends these two kinds to the
	// run's notification stream. They are named here because they are now part
	// of the stall vocabulary, and a name that only exists in the producer is a
	// name that drifts.
	KindElicitationOpened   = "elicitation.opened"
	KindElicitationResolved = "elicitation.resolved"
)

// sessionMetaKey is where the protocol observer records which session an update
// belongs to. It is the only structured place that attribution exists, which is
// why the view is computed from raw events: the trace policy drops protocol
// metadata for runs that do not opt into it, and those are exactly the runs an
// operator is most likely to be diagnosing.
const sessionMetaKey = "session_id"

// StallActivity is the last thing Matrix observed move, as a fact: which event,
// who produced it, and when. It carries no judgement about whether the movement
// was progress.
type StallActivity struct {
	Kind      string    `json:"kind"`
	Actor     string    `json:"actor,omitempty"`
	ToolName  string    `json:"tool_name,omitempty"`
	SessionID string    `json:"session_id,omitempty"`
	Timestamp time.Time `json:"timestamp"`
	Sequence  int       `json:"sequence,omitempty"`
}

// StallRequest is a request Matrix observed opened and never observed resolved.
// On a terminal run these are what was left open when the run ended.
//
// Requests that carry no identity are not listed: a request with no id cannot be
// paired with its resolution, and reporting it would turn "Matrix cannot tell"
// into a permanent false accusation of stalling.
type StallRequest struct {
	Kind      string    `json:"kind"`
	ID        string    `json:"id,omitempty"`
	Name      string    `json:"name,omitempty"`
	SessionID string    `json:"session_id,omitempty"`
	Since     time.Time `json:"since"`
	Sequence  int       `json:"sequence,omitempty"`
	// Question is what a person is being asked on this request, bounded and
	// marked where it was cut. It is the only user content this view can carry,
	// and it is carried so that a supervisor does not have to read a transcript
	// to know what the run is waiting for.
	Question          string `json:"question,omitempty"`
	QuestionTruncated bool   `json:"question_truncated,omitempty"`
}

// StallSession is the same picture for one session observed during the run.
// RunSession marks the session the run itself attached to: it is a structural
// comparison against the run record, not a role inferred from a provider or an
// agent name. Sessions the run record does not name are sessions the peer opened
// while the run was in flight.
type StallSession struct {
	SessionID    string         `json:"session_id,omitempty"`
	RunSession   bool           `json:"run_session,omitempty"`
	Waiting      string         `json:"waiting"`
	WaitingSince time.Time      `json:"waiting_since,omitzero"`
	LastActivity *StallActivity `json:"last_activity,omitempty"`
	Pending      []StallRequest `json:"pending,omitempty"`
}

// StallView answers the question a consumer otherwise has to read a provider's
// private database to answer: is this run still moving, and if not, what is it
// stuck on.
//
// Waiting describes the longest-outstanding wait, because that is when the run
// stopped being able to make progress; Pending lists everything outstanding with
// its own age, so a nested approval is visible without having to be called the
// current wait.
//
// WindowTruncated says the event window is a suffix of the run, not the whole
// run. It matters in one direction only: a retained request always keeps its
// resolution, because a resolution is written after the request it resolves, so
// a truncated window can under-report pending requests but can never fabricate
// one. A consumer that ignores the flag reads an incomplete picture as a
// complete one.
type StallView struct {
	Waiting         string         `json:"waiting"`
	WaitingSince    time.Time      `json:"waiting_since,omitzero"`
	LastActivity    *StallActivity `json:"last_activity,omitempty"`
	Pending         []StallRequest `json:"pending,omitempty"`
	Sessions        []StallSession `json:"sessions,omitempty"`
	WindowTruncated bool           `json:"window_truncated,omitempty"`
	PendingComplete bool           `json:"pending_complete"`
	Cause           string         `json:"cause"`
}

// ObserveStall derives the view from what Matrix already recorded: the run
// record, the event window, and the run's notifications. It is a pure function
// of those inputs — no clock, no store, no second opinion about what they mean.
//
// Notifications are read for one reason: the elicitation lifecycle is the single
// fact Matrix records outside the event stream, so a view built from events
// alone would be blind to the one wait a person is holding up.
func ObserveStall(run Run, events []Event, notifications []Notification) StallView {
	scan := stallScan{runSession: run.RemoteSessionID, resolved: map[string]bool{}}
	for _, event := range events {
		scan.consume(event)
	}
	for _, notification := range notifications {
		scan.consumeNotification(notification)
	}
	return scan.finish(run)
}

// consumeNotification pairs the elicitation lifecycle. Everything else in the
// notification stream is a terminal wakeup, which the event stream already
// carries, so it is ignored here rather than counted twice.
func (s *stallScan) consumeNotification(notification Notification) {
	switch notification.Kind {
	case KindElicitationOpened:
		s.addRequest(StallRequest{
			Kind: WaitElicitation, ID: notification.ElicitationID,
			SessionID: notification.SessionID, Since: notification.Timestamp,
			Question: notification.Question, QuestionTruncated: notification.QuestionTruncated,
		})
	case KindElicitationResolved:
		s.resolve(WaitElicitation, notification.ElicitationID, notification.SessionID)
	}
}

type stallScan struct {
	runSession string
	firstSeq   int
	activities []StallActivity
	requests   []StallRequest
	resolved   map[string]bool
	promptSent bool
}

func (s *stallScan) consume(event Event) {
	if s.firstSeq == 0 {
		s.firstSeq = event.Sequence
	}
	s.consumeRequest(event)
	if !isTurnEvidence(event) {
		return
	}
	s.activities = append(s.activities, StallActivity{
		Kind:      event.Kind,
		Actor:     event.Actor,
		ToolName:  event.ToolName,
		SessionID: eventSessionID(event),
		Timestamp: event.Timestamp,
		Sequence:  event.Sequence,
	})
}

func (s *stallScan) consumeRequest(event Event) {
	switch event.Kind {
	case KindToolCallRequested:
		s.addRequest(requestFrom(event, WaitToolResult, event.ToolCallID, event.ToolName))
	case KindToolResultReceived:
		s.resolve(WaitToolResult, event.ToolCallID, eventSessionID(event))
	case KindPermissionRequested:
		s.addRequest(requestFrom(event, WaitPermission, event.PermissionID, event.Summary))
	case KindPermissionResolved:
		s.resolve(WaitPermission, event.PermissionID, eventSessionID(event))
	case KindPromptSent:
		s.promptSent = true
	}
}

// requestFrom keeps the identity of a request with the event that carried it, so
// every request in the view can be traced back to the record it came from.
func requestFrom(event Event, kind, id, name string) StallRequest {
	return StallRequest{
		Kind: kind, ID: id, Name: name,
		SessionID: eventSessionID(event),
		Since:     event.Timestamp, Sequence: event.Sequence,
	}
}

func (s *stallScan) addRequest(request StallRequest) {
	if request.ID == "" {
		return
	}
	s.requests = append(s.requests, request)
}

func (s *stallScan) resolve(kind, id, session string) {
	s.resolved[requestKey(kind, id, session)] = true
}

func (s *stallScan) finish(run Run) StallView {
	pending := s.pending()
	view := StallView{
		Waiting:         WaitNone,
		LastActivity:    lastOf(s.activities),
		Pending:         pending,
		Sessions:        s.sessions(pending),
		WindowTruncated: s.firstSeq > 1,
		PendingComplete: s.firstSeq <= 1,
		Cause:           WaitUnknown,
	}
	view.Waiting, view.WaitingSince = waitState(pending, !isTerminalStatus(run.Status), view.LastActivity, s.promptSent)
	if isTerminalStatus(run.Status) {
		view.Cause = WaitNone
	}
	return view
}

// pending returns the requests nothing resolved, oldest first. Event order gives
// that order for free, so the oldest outstanding wait is the head of the list.
func (s *stallScan) pending() []StallRequest {
	pending := make([]StallRequest, 0, len(s.requests))
	seen := map[string]bool{}
	for _, request := range s.requests {
		key := requestKey(request.Kind, request.ID, request.SessionID)
		if !s.resolved[key] && !seen[key] {
			pending = append(pending, request)
			seen[key] = true
		}
	}
	return pending
}

// sessions groups the same facts by the session that produced them, so a parent
// and the sessions it opened are visible side by side instead of interleaved.
func (s *stallScan) sessions(pending []StallRequest) []StallSession {
	byID := map[string]*StallSession{}
	order := []string{}
	ensure := func(id string) *StallSession {
		if existing, ok := byID[id]; ok {
			return existing
		}
		entry := &StallSession{SessionID: id, RunSession: id == s.runSession && id != "", Waiting: WaitUnknown}
		byID[id] = entry
		order = append(order, id)
		return entry
	}
	if s.runSession != "" {
		ensure(s.runSession)
	}
	for _, activity := range s.activities {
		observed := activity
		ensure(observed.SessionID).LastActivity = &observed
	}
	for _, request := range pending {
		entry := ensure(request.SessionID)
		entry.Pending = append(entry.Pending, request)
	}
	out := make([]StallSession, 0, len(order))
	for _, id := range order {
		entry := byID[id]
		entry.Waiting, entry.WaitingSince = waitState(entry.Pending, true, entry.LastActivity, false)
		out = append(out, *entry)
	}
	return out
}

// waitState is the single rule for "what is this scope waiting on": the oldest
// unresolved request if there is one, otherwise the peer, otherwise an honest
// unknown. A finished scope waits on nothing, whatever is still outstanding.
func waitState(pending []StallRequest, active bool, last *StallActivity, promptSent bool) (string, time.Time) {
	if !active {
		return WaitNone, time.Time{}
	}
	if len(pending) > 0 {
		return pending[0].Kind, pending[0].Since
	}
	if last != nil {
		return WaitProviderTurn, last.Timestamp
	}
	if promptSent {
		return WaitProviderTurn, time.Time{}
	}
	return WaitUnknown, time.Time{}
}

func requestKey(kind, id, session string) string {
	return kind + "\x00" + id + "\x00" + session
}

func lastOf(activities []StallActivity) *StallActivity {
	if len(activities) == 0 {
		return nil
	}
	last := activities[len(activities)-1]
	return &last
}

// eventSessionID reads the session an event is attributed to. An event that
// names no session belongs to the run as a whole, which is reported as no
// session rather than assigned to the run's session by default: a guess there
// would attribute a peer's sub-session work to the parent.
func eventSessionID(event Event) string {
	if session, ok := event.ProtocolMeta[sessionMetaKey].(string); ok {
		return session
	}
	return ""
}
