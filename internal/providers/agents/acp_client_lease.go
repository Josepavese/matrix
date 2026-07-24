package agents

import "fmt"

func (c *acpConversationClient) AcquireConversationClientLease() (func() (bool, error), error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.client == nil || c.closed || c.closeRequested {
		return nil, fmt.Errorf("ACP conversation client is closing")
	}
	c.activeClientLeases++
	released := false
	return func() (bool, error) {
		c.mu.Lock()
		if released {
			c.mu.Unlock()
			return false, nil
		}
		released = true
		c.activeClientLeases--
		shouldClose := c.activeClientLeases == 0 && c.closeRequested && !c.closed
		if shouldClose {
			c.closed = true
		}
		c.mu.Unlock()
		if !shouldClose {
			return false, nil
		}
		return true, c.closeUnderlyingClient()
	}, nil
}

func (c *acpConversationClient) Close() error {
	c.mu.Lock()
	if c.closed {
		err := c.closeErr
		c.mu.Unlock()
		return err
	}
	c.closeRequested = true
	if c.activeClientLeases > 0 {
		c.mu.Unlock()
		return nil
	}
	c.closed = true
	c.mu.Unlock()
	return c.closeUnderlyingClient()
}

func (c *acpConversationClient) closeUnderlyingClient() error {
	if c.client == nil {
		return nil
	}
	err := c.client.Close()
	c.mu.Lock()
	c.closeErr = err
	c.mu.Unlock()
	return err
}
