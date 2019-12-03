package dataloader_test

import (
	"testing"

	"github.com/andy9775/dataloader"
	"github.com/stretchr/testify/assert"
)

// ================================================== tests ==================================================
// TestEnsureOKForResult tests getting the result value with a valid key expecting a valid value
func TestEnsureOKForResult(t *testing.T) {
	// setup
	rmap := dataloader.NewResultMap(2)
	key := PrimaryKey(1)
	value := dataloader.Result{Result: 1, Err: nil}
	rmap.Set(key, value)

	// invoke/assert
	result, ok := rmap.GetValue(key)
	assert.True(t, ok, "Expected valid result to have been found")
	assert.Equal(t, result, value, "Expected result")
}
func TestEnsureNotOKForResult(t *testing.T) {
	// setup
	rmap := dataloader.NewResultMap(2)
	key := PrimaryKey(1)
	key2 := PrimaryKey(2)
	value := dataloader.Result{Result: 1, Err: nil}
	rmap.Set(key, value)

	// invoke/assert
	result, ok := rmap.GetValue(key2)
	assert.False(t, ok, "Expected valid result to have been found")
	assert.Nil(t, result.Result, "Expected nil result")
}
