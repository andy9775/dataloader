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
// The dataloader requires a strategy to execute and a cache strategy to use.
func NewDataLoader(
	capacity int,
	fn func(int /* capacity */) Strategy,
	cacheStrategy Cache,
) DataLoader {

	return &dataloader{
		strategy: fn(capacity),
		cache:    cacheStrategy,
	}
}

// ================================================================================================

type dataloader struct {
	strategy Strategy
	cache    Cache
}

// Load returns the Result for the specified Key by calling the Load method on the provided strategy.
// Load method references the cache to check if a result already exists for the key.
func (d *dataloader) Load(ctx context.Context, key Key) Result {
	if r, ok := d.cache.GetResult(key); ok {
		d.strategy.LoadNoOp()
		return r
	}

	result := d.strategy.Load(ctx, key)
	d.cache.SetResult(key, result)

	return result
}

// LoadMany returns a result map which contains all resolved elements that were asked for.
// LoadMany references the cache and returns a ResultMap if all the keys are already present
// in the cache.
func (d *dataloader) LoadMany(ctx context.Context, keyArr ...Key) ResultMap {
	if r, ok := d.cache.GetResultMap(keyArr...); ok {
		d.strategy.LoadNoOp()
		return r
	}

	result := d.strategy.LoadMany(ctx, keyArr...)
	d.cache.SetResultMap(result)

	return result
}
