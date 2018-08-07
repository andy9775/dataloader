package strategies_test

import (
	"testing"

	"github.com/andy9775/dataloader/strategies"
	"github.com/stretchr/testify/assert"
)

// TestCounter checks if counter returns true after capacity is hit.
// It also tests resetting and reusing the counter
func TestCounter(t *testing.T) {
	// setup
	counter := strategies.NewCounter(3)

	// assert/test

	// capacity is 3, third increment should return true
	assert.False(t, counter.Increment(), "Counter should not have hit capacity")
	assert.False(t, counter.Increment(), "Counter should not have hit capacity")
	assert.True(t, counter.Increment(), "Counter should  have hit capacity")

	counter.ResetCount() // zero out

	// capacity is 3, third increment should return true
	assert.False(t, counter.Increment(), "Counter should not have hit capacity")
	assert.False(t, counter.Increment(), "Counter should not have hit capacity")
	assert.True(t, counter.Increment(), "Counter should  have hit capacity")
}
