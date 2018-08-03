package dataloader

import (
	"context"
)

// Strategy specifies the interface of loader strategies. A loader strategy specifies the process
// of calling the batch function and handling requests to fetch data.
type Strategy interface {
	// Load returns a Thunk for the specified Key.
	// Internally load adds the provided key to the keys array and returns a callback function linked
	// to the key which blocks when called until the the batch function is called.
	Load(context.Context, Key) Thunk

	// LoadMany returns a ThunkMany which returns a ResultMap for the keys it was called for.
	// When called, callback blocks until the batch function is called.
	LoadMany(context.Context, ...Key) ThunkMany

	// LoadNoNop doesn't block the caller and doesn't return a value when called.
	// LoadNoOp is called after a cache hit and the found result value is returned to the caller
	// and thus simply increments the loads call counter.
	LoadNoOp(context.Context)
}
