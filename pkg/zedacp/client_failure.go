package zedacp

import "context"

// fail keeps the first cause, including when cancellation subsequently unblocks
// Receive. All calls sharing this connection must reject an incomplete stream.
func (c *Client) fail(err error) {
	c.mu.Lock()
	if c.terminalErr == nil {
		c.terminalErr = err
	}
	c.mu.Unlock()
	if c.cancel != nil {
		c.cancel(err)
	}
}

func (c *Client) deliveryError(ctx context.Context) error {
	if ctx != nil && ctx.Err() != nil {
		return context.Cause(ctx)
	}
	c.mu.RLock()
	err := c.terminalErr
	c.mu.RUnlock()
	if err == nil && c.ctx != nil {
		err = context.Cause(c.ctx)
	}
	return err
}
