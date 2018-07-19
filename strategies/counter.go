package strategies

import "sync"

// Counter provides the interface to a Load call counter.
// A Load call counter provides helper methods to track the
// number of increments and identify when the increments equal
// the counter capacity
type Counter interface {
	// Increment increases the internal counter and returns true if the counter
	// has hit its capacity
	Increment() bool
	// ResetCount resets the Load call counter back to 0
	ResetCount()
}

// NewCounter returns a new instance of a Load call counter
func NewCounter(capacity int) Counter {
	return &counter{
		capacity:     capacity,
		loadCalls:    0,
		counterMutex: &sync.Mutex{},
	}
}

type counter struct {
	counterMutex *sync.Mutex
	loadCalls    int
	capacity     int
}

func (c *counter) Increment() bool {
	c.counterMutex.Lock()
	defer c.counterMutex.Unlock()

	c.loadCalls++
	return c.loadCalls == c.capacity
}

func (c *counter) ResetCount() {
	c.counterMutex.Lock()
	defer c.counterMutex.Unlock()

	c.loadCalls = 0
}
