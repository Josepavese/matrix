package agents

import "github.com/Josepavese/matrix/internal/middleware"

type acpSessionUpdatesWatcher interface {
	WatchSessionUpdates(acpSessionObserver) func()
}

// Presentation state is owned by one physical connection. A replacement starts
// empty and reconstructs retained entities from the peer's explicit replay.
func (c *acpConversationClient) installPresentation(client acpClient) {
	c.releasePresentation()
	watcher, ok := client.(acpSessionUpdatesWatcher)
	if !ok {
		return
	}
	state := &acpPresentation{version: client.AuthenticatedProtocolVersion()}
	stop := watcher.WatchSessionUpdates(state)
	c.mu.Lock()
	c.presentation, c.presentationStop = state, stop
	c.mu.Unlock()
}

func (c *acpConversationClient) releasePresentation() {
	c.mu.Lock()
	stop := c.presentationStop
	c.presentationStop, c.presentation = nil, nil
	c.mu.Unlock()
	if stop != nil {
		stop()
	}
}

func (c *acpConversationClient) currentPresentation() *acpPresentation {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.presentation
}

func (c *acpConversationClient) bindPresentation(id string, notifier middleware.ThoughtNotifier) func() {
	if state := c.currentPresentation(); state != nil {
		return state.bind(id, notifier)
	}
	return func() {}
}

func (c *acpConversationClient) withPresentationMetadata(id string, metadata middleware.ConversationMetadata) middleware.ConversationMetadata {
	items := c.currentPresentation().snapshot(id)
	if len(items) > 0 {
		if metadata.Meta == nil {
			metadata.Meta = make(map[string]interface{})
		}
		metadata.Meta["context_compactions"] = items
	}
	return metadata
}
