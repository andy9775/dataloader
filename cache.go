package dataloader

import "context"

// Cache provides an interface for caching strategies
type Cache interface {
	// SetResult sets a single result for a specified key
	SetResult(context.Context, Key, Result)
	// SetResultMap passes a ResultMap to the cache
	SetResultMap(context.Context, ResultMap)
	// GetResult returns a single result for a key
	GetResult(context.Context, Key) (Result, bool)
	// GetResultMap returns a ResultMap for a set of keys. The returned ResultMap
	// only contains the values that belong to the provided keys
	GetResultMap(context.Context, ...Key) (ResultMap, bool)
	// Delete removes the specific value for the provided key
	Delete(context.Context, Key) bool
	// Clear cleans the cache
	Clear(context.Context) bool
}

// ========================== no-op cache implementation ==========================

// NewNoOpCache returns a cache strategy with no internal implementation
func NewNoOpCache() Cache {
	return &noopCache{}
}

// noopCache does not cache any values, always return nil for any request to get data.
type noopCache struct{}

func (*noopCache) SetResult(ctx context.Context, key Key, result Result) {}

func (*noopCache) SetResultMap(ctx context.Context, resultMap ResultMap) {}

func (*noopCache) GetResult(ctx context.Context, key Key) (Result, bool) {
	return Result{}, false
}

func (*noopCache) GetResultMap(ctx context.Context, keys ...Key) (ResultMap, bool) {
	return nil, false
}

func (*noopCache) Delete(ctx context.Context, key Key) bool { return true }

func (*noopCache) Clear(ctx context.Context) bool { return true }
