package zedacp

import (
	"strings"
	"sync/atomic"
)

func (c *Client) registerObserver(sessionID string, observer SessionObserver) func() {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" || observer == nil {
		return func() {}
	}
	id := atomic.AddUint64(&c.nextObsID, 1)
	c.mu.Lock()
	if c.observers == nil {
		c.observers = make(map[string]map[uint64]SessionObserver)
	}
	if c.observers[sessionID] == nil {
		c.observers[sessionID] = make(map[uint64]SessionObserver)
	}
	c.observers[sessionID][id] = observer
	c.mu.Unlock()
	return func() {
		c.mu.Lock()
		defer c.mu.Unlock()
		observers := c.observers[sessionID]
		delete(observers, id)
		if len(observers) == 0 {
			delete(c.observers, sessionID)
		}
	}
}

func (c *Client) sessionObservers(sessionID string) []SessionObserver {
	c.mu.RLock()
	defer c.mu.RUnlock()
	registered := c.observers[sessionID]
	if len(registered) == 0 {
		return nil
	}
	observers := make([]SessionObserver, 0, len(registered))
	for _, observer := range registered {
		if observer != nil {
			observers = append(observers, observer)
		}
	}
	return observers
}
