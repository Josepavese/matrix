package agents

import (
	"context"
	"fmt"
	"strings"

	"github.com/Josepavese/matrix/internal/middleware"
)

func (c *acpConversationClient) beginPrompt(ctx context.Context, remoteSessionID string, rejectIfActive bool) (func(), error) {
	remoteSessionID = strings.TrimSpace(remoteSessionID)
	if remoteSessionID == "" {
		return func() {}, nil
	}
	for {
		done, active := c.promptGuard(remoteSessionID)
		if !active {
			return func() { c.finishPrompt(remoteSessionID, done) }, nil
		}
		if rejectIfActive {
			return nil, fmt.Errorf("%w: remote session %s has an active prompt turn", middleware.ErrConversationTurnActive, remoteSessionID)
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-done:
		}
	}
}

func (c *acpConversationClient) promptGuard(remoteSessionID string) (chan struct{}, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.activePrompts == nil {
		c.activePrompts = map[string]chan struct{}{}
	}
	done, active := c.activePrompts[remoteSessionID]
	if active {
		return done, true
	}
	done = make(chan struct{})
	c.activePrompts[remoteSessionID] = done
	return done, false
}

func (c *acpConversationClient) finishPrompt(remoteSessionID string, done chan struct{}) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.activePrompts[remoteSessionID] != done {
		return
	}
	delete(c.activePrompts, remoteSessionID)
	close(done)
}
