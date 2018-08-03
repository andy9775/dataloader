package dataloader

import (
	"context"
)

// DataLoader is the identifying interface for the dataloader.
// Each DataLoader instance tracks the resolved elements.
// Note that calling Load and LoadMany on the same dataloader instance
// will increment the same counter, once for each method call.
type DataLoader interface {
	// Load returns a Thunk for the specified Key.
	// Internally Load adds the provided key to the keys array and returns a callback
	// function which when called returns the value for the key
	Load(context.Context, Key) Thunk

	// LoadMany returns a ThunkMany for the specified keys.
	// Internally LoadMany adds the provided keys to the keys array and returns a callback
	// function which when called returns the values for the provided keys.
	LoadMany(context.Context, ...Key) ThunkMany
}

// BatchFunction is called with n keys after the keys passed to the loader reach
// the loader capacity
type BatchFunction func(context.Context, Keys) *ResultMap

// Thunk returns a result for the key that it was generated for.
// Calling the Thunk function will block until the result is returned from the batch function.
type Thunk func() Result

// ThunkMany returns a result map for the keys that it was generated for.
// Calling ThunkMany will block until the result is returned from the batch function.
type ThunkMany func() ResultMap

// NewDataLoader returns a new DataLoader with a count capacity of `capacity`.
// The capacity value determines when the batch loader function will execute.
// The dataloader requires a strategy to execute and a cache strategy to use for
// storing data.
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

// Load returns the Thunk for the specified Key by calling the Load method on the provided strategy.
// Load method references the cache to check if a result already exists for the key. If a result exists,
// it returns a Thunk which simply returns the cached result (non-blocking).
func (d *dataloader) Load(ctx context.Context, key Key) Thunk {
	if r, ok := d.cache.GetResult(ctx, key); ok {
		d.strategy.LoadNoOp(ctx)
		return func() Result {
			return r
		}
	}

	thunk := d.strategy.Load(ctx, key)
	return func() Result {
		result := thunk()
		d.cache.SetResult(ctx, key, result)

		return result
	}
}

// LoadMany returns a ThunkMany for the specified keys by calling the LoadMany method on the provided
// strategy.
// LoadMany references the cache and returns a ThunkMany which returns the cached values when called
// (non-blocking).
func (d *dataloader) LoadMany(ctx context.Context, keyArr ...Key) ThunkMany {
	if r, ok := d.cache.GetResultMap(ctx, keyArr...); ok {
		d.strategy.LoadNoOp(ctx)
		return func() ResultMap {
			return r
		}
	}

	thunkMany := d.strategy.LoadMany(ctx, keyArr...)
	return func() ResultMap {
		result := thunkMany()
		d.cache.SetResultMap(ctx, result)

		return result
	}
}
