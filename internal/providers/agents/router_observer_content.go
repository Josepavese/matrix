package agents

import "github.com/Josepavese/matrix/internal/middleware"

func (o *simpleObserver) appendMessageChunk(text string, contents []acpContent, metadata map[string]interface{}) {
	o.mu.Lock()
	o.content += text
	for _, content := range contents {
		o.blocks = append(o.blocks, middleware.Content{
			Type:        content.Type,
			Text:        content.Text,
			Data:        content.Data,
			MimeType:    content.MimeType,
			URI:         content.URI,
			Name:        content.Name,
			Title:       content.Title,
			Description: content.Description,
			Size:        content.Size,
			Resource:    content.Resource,
			Annotations: content.Annotations,
			Meta:        content.Meta,
		})
	}
	o.mu.Unlock()
	o.forwardThought(middleware.ThoughtTypeThinking, text, "", metadata)
	o.signalUpdate()
}

func (o *simpleObserver) ContentBlocks() []middleware.Content {
	o.mu.Lock()
	defer o.mu.Unlock()
	return append([]middleware.Content(nil), o.blocks...)
}
