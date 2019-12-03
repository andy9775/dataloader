package dataloader

import (
	"context"

	"github.com/go-log/log"
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

// StrategyFunction defines the return type of strategy builder functions.
// A strategy builder function returns a specific strategy when called.
type StrategyFunction func(int, BatchFunction) Strategy

// BatchFunction is called with n keys after the keys passed to the loader reach
// the loader capacity
type BatchFunction func(context.Context, Keys) *ResultMap

// Thunk returns a result for the key that it was generated for.
// Calling the Thunk function will block until the result is returned from the batch function.
type Thunk func() (Result, bool)

// ThunkMany returns a result map for the keys that it was generated for.
// Calling ThunkMany will block until the result is returned from the batch function.
type ThunkMany func() ResultMap

// Option accepts the dataloader and sets an option on it.
type Option func(*dataloader)

// NewDataLoader returns a new DataLoader with a count capacity of `capacity`.
// The capacity value determines when the batch loader function will execute.
// The dataloader requires a strategy to execute and a cache strategy to use for
// storing data.
func NewDataLoader(
	capacity int,
	batch BatchFunction,
	fn StrategyFunction,
	opts ...Option,
) DataLoader {

	loader := dataloader{}

	// set the options
	for _, apply := range opts {
		apply(&loader)
	}

	// default options
	if loader.cache == nil {
		loader.cache = NewNoOpCache()
	}

	if loader.tracer == nil {
		loader.tracer = NewNoOpTracer()
	}

	if loader.logger == nil {
		loader.logger = log.DefaultLogger // no op logger
	}

	// wrap the batch function and implement tracing around it
	batchFunc := func(ogCtx context.Context, keys Keys) *ResultMap {
		ctx, finish := loader.tracer.Batch(ogCtx)

		r := batch(ctx, keys)

		finish(*r)
		return r
	}

	loader.strategy = fn(capacity, batchFunc)

	return &loader
}

// ============================================= options setters =============================================

// WithCache adds a cache strategy to the dataloader
func WithCache(cache Cache) Option {
	return func(l *dataloader) {
		l.cache = cache
	}
}

// WithTracer adds a tracer to the dataloader
func WithTracer(tracer Tracer) Option {
	return func(l *dataloader) {
		l.tracer = tracer
	}
}

// WithLogger adds a logger to the dataloader. The default is a no op logger
func WithLogger(logger log.Logger) Option {
	return func(l *dataloader) {
		l.logger = logger
	}
}

// ================================================================================================

type dataloader struct {
	strategy Strategy
	cache    Cache
	tracer   Tracer
	logger   log.Logger
}

// Load returns the Thunk for the specified Key by calling the Load method on the provided strategy.
// Load method references the cache to check if a result already exists for the key. If a result exists,
// it returns a Thunk which simply returns the cached result (non-blocking).
func (d *dataloader) Load(ogCtx context.Context, key Key) Thunk {
	ctx, finish := d.tracer.Load(ogCtx, key)

	if r, ok := d.cache.GetResult(ctx, key); ok {
		d.logger.Logf("cache hit for: %d", key)
		d.strategy.LoadNoOp(ctx)
		return func() (Result, bool) {
			finish(r)

			return r, ok
		}
	}

	thunk := d.strategy.Load(ctx, key)
	return func() (Result, bool) {
		result, ok := thunk()
		d.cache.SetResult(ctx, key, result)

		finish(result)

		return result, ok
	}
}

// LoadMany returns a ThunkMany for the specified keys by calling the LoadMany method on the provided
// strategy.
// LoadMany references the cache and returns a ThunkMany which returns the cached values when called
// (non-blocking).
func (d *dataloader) LoadMany(ogCtx context.Context, keyArr ...Key) ThunkMany {
	ctx, finish := d.tracer.LoadMany(ogCtx, keyArr)

	var cached, missed = ResultMap{}, []Key{}
	for _, key := range keyArr {
		if r, ok := d.cache.GetResult(ctx, key); ok {
			d.logger.Logf("cache hit for: %d", key)
			d.strategy.LoadNoOp(ctx)
			cached[key.String()] = r
		} else {
			missed = append(missed, key)
		}
	}

	if len(missed) == 0 {
		return func() ResultMap {
			finish(cached)
			return cached
		}
	}

	thunkMany := d.strategy.LoadMany(ctx, missed...)
	return func() ResultMap {
		cached := cached
		result := thunkMany()
		d.cache.SetResultMap(ctx, result)

		for k, v := range cached {
			result[k] = v
		}
		finish(result)

		return result
	}
}
