package agents

import "github.com/Josepavese/matrix/internal/middleware"

func (o *simpleObserver) applySessionInfoPatch(u acpSessionUpdate) {
	if o.version < 2 || u.SessionUpdate != "session_info_update" {
		return
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	patch := o.metadata.SessionPatch
	if patch == nil {
		patch = &middleware.SessionMetadataPatch{}
		o.metadata.SessionPatch = patch
	}
	if u.TitleSet {
		title := u.Title
		patch.Title = &title
		o.metadata.Title = title
	}
	if u.UpdatedAtSet {
		stamp := u.UpdatedAt
		patch.UpdatedAt = &stamp
		o.metadata.UpdatedAt = stamp
	}
	if u.MetaCleared {
		patch.ClearMetadata = true
		patch.Metadata = nil
	}
	patch.Metadata = composeSessionMetadataPatch(patch.Metadata, u.Meta)
}

// Patch composition retains null removals until the neutral mirror applies it.
func composeSessionMetadataPatch(current, update map[string]interface{}) map[string]interface{} {
	out := clonePresentationMap(current)
	for key, value := range update {
		if out == nil {
			out = map[string]interface{}{}
		}
		out[key] = clonePresentationValue(value)
	}
	return out
}
func cloneSessionInfoPatch(p *middleware.SessionMetadataPatch) *middleware.SessionMetadataPatch {
	if p == nil {
		return nil
	}
	out := *p
	out.Metadata = clonePresentationMap(p.Metadata)
	if p.Title != nil {
		value := *p.Title
		out.Title = &value
	}
	if p.UpdatedAt != nil {
		value := *p.UpdatedAt
		out.UpdatedAt = &value
	}
	return &out
}
