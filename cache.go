package dataloader

// Cache provides an interface for caching strategies
type Cache interface {
	// SetResult sets a single result for a specified key
	SetResult(Key, Result)
	// SetResultMap passes a ResultMap to the cache
	SetResultMap(ResultMap)
	// GetResult returns a single result for a key
	GetResult(Key) (Result, bool)
	// GetResultMap returns a ResultMap for a set of keys. The returned ResultMap
	// only contains the values that belong to the provided keys
	GetResultMap(...Key) (ResultMap, bool)
	// Delete removes the specific value for the provided key
	Delete(Key)
	// Clear cleans the cache
	Clear()
}

// ========================== no-op cache implementation ==========================

// NewNoOpCache returns a cache strategy with no internal implementation
func NewNoOpCache() Cache {
	return &noopCache{}
}

// noopCache does not cache any values, always return nil for any request to get data.
type noopCache struct{}

func (*noopCache) SetResult(key Key, result Result) {}

func (*noopCache) SetResultMap(resultMap ResultMap) {}

func (*noopCache) GetResult(key Key) (Result, bool) {
	return Result{}, false
}

func (*noopCache) GetResultMap(keys ...Key) (ResultMap, bool) {
	return nil, false
}

func (*noopCache) Delete(key Key) {}

func (*noopCache) Clear() {}
