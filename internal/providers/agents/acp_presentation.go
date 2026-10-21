package agents

import (
	"encoding/json"
	"sync"

	"github.com/Josepavese/matrix/internal/middleware"
)

type acpPresentation struct {
	mu         sync.Mutex
	sessions   map[string]*acpPresentationSession
	generation uint64
	version    int
}

type acpPresentationSession struct {
	compactions map[string]*middleware.ContextCompaction
	order       []string
	notifier    middleware.ThoughtNotifier
	generation  uint64
}

func (p *acpPresentation) sessionLocked(id string) *acpPresentationSession {
	if p.sessions == nil {
		p.sessions = make(map[string]*acpPresentationSession)
	}
	if p.sessions[id] == nil {
		p.sessions[id] = &acpPresentationSession{compactions: make(map[string]*middleware.ContextCompaction)}
	}
	return p.sessions[id]
}

// Bind owns the live frontend only for this turn. Stored compactions outlive
// that binding; late notices cannot write to a completed run's old notifier.
func (p *acpPresentation) bind(id string, notifier middleware.ThoughtNotifier) func() {
	p.mu.Lock()
	session := p.sessionLocked(id)
	p.generation++
	generation := p.generation
	session.notifier, session.generation = notifier, generation
	p.mu.Unlock()
	return func() {
		p.mu.Lock()
		defer p.mu.Unlock()
		if session.generation == generation {
			session.notifier = nil
		}
	}
}

func (p *acpPresentation) OnUpdate(notification acpSessionNotification) {
	u := notification.Update
	if u.SessionUpdate != "notice" && u.SessionUpdate != "compaction_update" && u.SessionUpdate != "compaction_summary_chunk" {
		return
	}
	p.mu.Lock()
	session := p.sessionLocked(notification.SessionID)
	update := middleware.ThoughtUpdate{Type: middleware.ThoughtTypeNotice, Title: u.Title, Content: u.Description, Metadata: streamUpdateMetadata(notification)}
	if u.SessionUpdate == "notice" {
		update.Metadata["severity"] = u.Severity
	} else {
		applyCompaction(session, u, p.version >= 2)
		snapshot := cloneCompaction(session.compactions[u.CompactionID])
		update.Type = middleware.ThoughtTypeCompaction
		update.Title = "Context compaction"
		update.Content = snapshot.Status
		update.Metadata["compaction"] = snapshot
	}
	notifier := session.notifier
	p.mu.Unlock()
	if notifier != nil {
		notifier.OnThought(update)
	}
}

func applyCompaction(session *acpPresentationSession, u acpSessionUpdate, patchMetadata bool) {
	item := session.compactions[u.CompactionID]
	if item == nil {
		item = &middleware.ContextCompaction{ID: u.CompactionID, Status: "in_progress"}
		session.compactions[u.CompactionID] = item
		session.order = append(session.order, u.CompactionID)
	}
	if u.SessionUpdate == "compaction_summary_chunk" {
		item.Summary = append(item.Summary, middlewareContents(u.Contents)...)
		return
	}
	item.Status = u.Status
	if u.SummarySet {
		item.Summary = middlewareContents(u.Summary)
	}
	if u.ErrorSet {
		_ = json.Unmarshal(u.Error, &item.Error)
		if string(u.Error) == "null" {
			item.Error = ""
		}
	}
	if patchMetadata {
		item.Meta = patchACPMetadata(item.Meta, u.Meta, u.MetaSet, u.MetaCleared)
	} else if u.MetaSet {
		item.Meta = clonePresentationMap(u.Meta)
	}
}

func (p *acpPresentation) snapshot(id string) []middleware.ContextCompaction {
	if p == nil {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	session := p.sessions[id]
	if session == nil {
		return nil
	}
	out := make([]middleware.ContextCompaction, 0, len(session.order))
	for _, id := range session.order {
		out = append(out, cloneCompaction(session.compactions[id]))
	}
	return out
}

func cloneCompaction(item *middleware.ContextCompaction) middleware.ContextCompaction {
	out := *item
	out.Meta = clonePresentationMap(item.Meta)
	out.Summary = append([]middleware.Content(nil), item.Summary...)
	for i := range out.Summary {
		block := &out.Summary[i]
		block.Meta = clonePresentationMap(block.Meta)
		block.Annotations = clonePresentationMap(block.Annotations)
		block.Resource = clonePresentationMap(block.Resource)
		if block.Size != nil {
			size := *block.Size
			block.Size = &size
		}
	}
	return out
}

func clonePresentationMap(source map[string]interface{}) map[string]interface{} {
	if source == nil {
		return nil
	}
	out := make(map[string]interface{}, len(source))
	for key, value := range source {
		out[key] = clonePresentationValue(value)
	}
	return out
}

func clonePresentationValue(value interface{}) interface{} {
	switch typed := value.(type) {
	case map[string]interface{}:
		return clonePresentationMap(typed)
	case []interface{}:
		out := make([]interface{}, len(typed))
		for i, item := range typed {
			out[i] = clonePresentationValue(item)
		}
		return out
	default:
		return value
	}
}

// ACP v2 metadata patches are top-level operations, never recursive merges.
func patchACPMetadata(current, patch map[string]interface{}, set, cleared bool) map[string]interface{} {
	if !set {
		return current
	}
	if cleared {
		return nil
	}
	out := clonePresentationMap(current)
	if out == nil {
		out = make(map[string]interface{})
	}
	for key, value := range patch {
		if value == nil {
			delete(out, key)
		} else {
			out[key] = clonePresentationValue(value)
		}
	}
	return out
}
