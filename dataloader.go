package dataloader

import (
	"context"
)

// DataLoader is the identifying interface for the dataloader.
// Each DataLoader instance tracks the resolved elements.
// Note that calling Load and LoadMany on the same dataloader instance
// will increment the same counter, once for each method call.
type DataLoader interface {
	// Load returns the Result for the specified Key.
	// Internally load adds the provided key to the keys array and blocks until a result
	// is returned.
	Load(context.Context, Key) Result

	// LoadMany returns a result map containing all the elements the caller requested.
	LoadMany(context.Context, ...Key) ResultMap
}

// BatchFunction is called with n keys after the keys passed to the loader reach
// the loader capacity
type BatchFunction func(context.Context, Keys) *ResultMap

// NewDataLoader returns a new DataLoader with a count capacity of `capacity`.
// The capacity value determines when the batch loader function will execute.
// The dataloader requires a strategy to execute.
func NewDataLoader(
	capacity int,
	fn func(int) Strategy,
) DataLoader {

	return &dataloader{
		strategy: fn(capacity),
	}
}

// ================================================================================================

type dataloader struct {
	strategy Strategy
}

// Load returns the Result for the specified Key by calling the Load method on the provided strategy.
func (d *dataloader) Load(ctx context.Context, key Key) Result {
	return d.strategy.Load(ctx, key)
}

// LoadMany returns a result map which contains all resolved elements that were asked for
func (d *dataloader) LoadMany(ctx context.Context, keyArr ...Key) ResultMap {
	return d.strategy.LoadMany(ctx, keyArr...)
}
