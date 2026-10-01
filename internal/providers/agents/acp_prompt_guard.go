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
		c.addPromptWaiter(remoteSessionID)
		select {
		case <-ctx.Done():
			c.removePromptWaiter(remoteSessionID)
			return nil, ctx.Err()
		case <-done:
			c.removePromptWaiter(remoteSessionID)
		}
	}
}

// addPromptWaiter and removePromptWaiter count the turns blocked on a session's
// active prompt. The guard never reads the count: it exists so the wait itself
// is observable, which is what lets a test assert that a second turn is
// serialized instead of sleeping and hoping it arrived in time.
func (c *acpConversationClient) addPromptWaiter(remoteSessionID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.promptWaiters == nil {
		c.promptWaiters = map[string]int{}
	}
	c.promptWaiters[remoteSessionID]++
}

func (c *acpConversationClient) removePromptWaiter(remoteSessionID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.promptWaiters[remoteSessionID] <= 0 {
		// Reading a nil map is fine, writing one is not: an unmatched removal
		// stays a no-op rather than taking the agent down with a panic.
		return
	}
	c.promptWaiters[remoteSessionID]--
	if c.promptWaiters[remoteSessionID] <= 0 {
		delete(c.promptWaiters, remoteSessionID)
	}
}

// promptWaiterCount reports how many turns are waiting for this session's active
// prompt to finish, so a caller observes the wait as a fact rather than as an
// elapsed interval.
func (c *acpConversationClient) promptWaiterCount(remoteSessionID string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.promptWaiters[strings.TrimSpace(remoteSessionID)]
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
