package orchestration

// Capability describes one orchestration capability exposed by Matrix.
type Capability struct {
	ID          string   `json:"id"`
	Category    string   `json:"category"`
	Description string   `json:"description"`
	Surfaces    []string `json:"surfaces,omitempty"`
}

// Surface describes one machine-usable Matrix surface.
type Surface struct {
	ID          string   `json:"id"`
	Description string   `json:"description"`
	Actions     []string `json:"actions,omitempty"`
}

// Profile is the machine-readable description of Matrix as an agent communication matrix.
type Profile struct {
	Name         string       `json:"name"`
	Category     string       `json:"category"`
	Role         string       `json:"role"`
	Capabilities []Capability `json:"capabilities"`
	Surfaces     []Surface    `json:"surfaces"`
}

// ProfileV1 returns the stable orchestration capability profile for Matrix v1 surfaces.
func ProfileV1() Profile {
	return Profile{
		Name:     "matrix",
		Category: "local-first Agent Communication Matrix",
		Role:     "communication crossroads for humans, supervisory AI, and agents",
		Capabilities: []Capability{
			{ID: "conversation.route", Category: "conversation", Description: "Route one human, channel, or agent turn into the active workspace/session context.", Surfaces: []string{"http:/v1/runs"}},
			{ID: "run.observability", Category: "run", Description: "Expose protocol-neutral communication run traces, ordered events, and operational controls for humans and supervisory agents.", Surfaces: []string{"http:/v1/runs/{id}/trace", "http:/v1/runs/{id}/events", "http:/v1/runs/{id}/actions", "http:/v1/event-sinks"}},
			{ID: "session.lifecycle", Category: "session", Description: "Create, inspect, switch, cancel, and delete logical sessions.", Surfaces: []string{"http:/v1/session-actions", "chat:/session", "chat:/cancel", "chat:/stop"}},
			{ID: "workspace.control", Category: "workspace", Description: "List, switch, bind, and snapshot workspace contexts.", Surfaces: []string{"http:/v1/workspace-actions", "chat:/workspace", "chat:/use", "chat:/snapshot"}},
			{ID: "workspace.state", Category: "workspace", Description: "Read the current materialized state of a workspace.", Surfaces: []string{"http:/v1/workspace-state", "chat:/now", "cli:workspace state"}},
			{ID: "workspace.timeline", Category: "workspace", Description: "Read recent semantic events for a workspace.", Surfaces: []string{"http:/v1/workspace-timeline", "chat:/timeline", "cli:workspace timeline"}},
			{ID: "workspace.decisions", Category: "workspace", Description: "Read recent orchestration decisions and routing explanations for a workspace.", Surfaces: []string{"http:/v1/workspace-decisions", "chat:/why", "chat:/decisions", "cli:workspace decisions"}},
			{ID: "workspace.memory", Category: "workspace", Description: "Read recent locally stored work-memory turns for a workspace.", Surfaces: []string{"http:/v1/workspace-memory", "chat:/memory", "cli:workspace memory"}},
			{ID: "workspace.snapshots", Category: "workspace", Description: "Read and create workspace snapshots.", Surfaces: []string{"http:/v1/workspace-snapshots", "chat:/snapshots", "chat:/snapshot", "cli:workspace snapshots"}},
			{ID: "intent.high_level", Category: "intent", Description: "Drive high-level orchestration intents such as continue, resume, review, triage, explain, and handoff.", Surfaces: []string{"http:/v1/intents", "http:/v1/modes", "chat:/continue", "chat:/resume", "chat:/review", "chat:/explain", "chat:/triage", "chat:/handoff"}},
		},
		Surfaces: []Surface{
			{ID: "http:/v1/runs", Description: "Canonical Matrix communication run ingress.", Actions: []string{"sync", "async", "stream"}},
			{ID: "http:/v1/runs/{id}/trace", Description: "Versioned protocol-neutral communication run trace projection.", Actions: []string{"read"}},
			{ID: "http:/v1/runs/{id}/events", Description: "Ordered run event read surface for live supervision and replayable observation.", Actions: []string{"read"}},
			{ID: "http:/v1/runs/{id}/actions", Description: "Operational run control surface.", Actions: []string{"cancel"}},
			{ID: "http:/v1/event-sinks", Description: "Generic external consumer registration for Matrix run events.", Actions: []string{"register"}},
			{ID: "http:/v1/session-actions", Description: "Typed session lifecycle API.", Actions: []string{"status", "new", "name", "list", "switch", "cancel", "delete"}},
			{ID: "http:/v1/workspace-actions", Description: "Typed workspace control API.", Actions: []string{"list", "status", "switch", "bind", "snapshot"}},
			{ID: "http:/v1/workspace-state", Description: "Typed workspace current-state read API.", Actions: []string{"state"}},
			{ID: "http:/v1/workspace-timeline", Description: "Typed workspace timeline read API.", Actions: []string{"timeline"}},
			{ID: "http:/v1/workspace-decisions", Description: "Typed workspace decision-trace read API.", Actions: []string{"decisions"}},
			{ID: "http:/v1/workspace-memory", Description: "Typed workspace memory read API.", Actions: []string{"memory"}},
			{ID: "http:/v1/workspace-snapshots", Description: "Typed workspace snapshot read API.", Actions: []string{"snapshots"}},
			{ID: "http:/v1/intents", Description: "Typed high-level operator intent API.", Actions: []string{"continue", "resume", "review", "explain", "triage", "handoff"}},
			{ID: "http:/v1/modes", Description: "Typed explicit mode-switch API.", Actions: []string{"implementation", "review", "explain", "triage"}},
		},
	}
}
