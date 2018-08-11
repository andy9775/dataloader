package dataloader_test

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/andy9775/dataloader"
	"github.com/stretchr/testify/assert"
)

// ============================================== test constants =============================================
const TEST_TIMEOUT time.Duration = time.Millisecond * 500

// ==================================== implement concrete keys interface ====================================
type PrimaryKey int

func (p PrimaryKey) String() string {
	return strconv.Itoa(int(p))
}

func (p PrimaryKey) Raw() interface{} {
	return p
}

// =============================================== test helpers ==============================================

// getBatchFunction returns a generic batch function which returns the provided result and calls the provided
// callback function
func getBatchFunction(cb func(), result dataloader.Result) dataloader.BatchFunction {
	return func(ctx context.Context, keys dataloader.Keys) *dataloader.ResultMap {
		cb()
		m := dataloader.NewResultMap(1)
		m.Set(keys.Keys()[0].(PrimaryKey).String(), result)
		return &m
	}
}

// ========================= mock cache =========================
type mockCache struct {
	r map[string]dataloader.Result
}

func newMockCache(cap int) dataloader.Cache {
	return &mockCache{r: make(map[string]dataloader.Result, cap)}
}

func (c *mockCache) SetResult(ctx context.Context, key dataloader.Key, result dataloader.Result) {
	c.r[key.String()] = result
}

func (c *mockCache) SetResultMap(ctx context.Context, resultMap dataloader.ResultMap) {
	for _, k := range resultMap.Keys() {
		c.r[k] = resultMap.GetValueForString(k)
	}
}

func (c *mockCache) GetResult(ctx context.Context, key dataloader.Key) (dataloader.Result, bool) {
	r, ok := c.r[key.String()]
	return r, ok
}

func (c *mockCache) GetResultMap(ctx context.Context, keys ...dataloader.Key) (dataloader.ResultMap, bool) {
	result := dataloader.NewResultMap(len(keys))
	for _, k := range keys {
		r, ok := c.r[k.String()]
		if !ok {
			return dataloader.NewResultMap(len(keys)), false
		}
		result.Set(k.String(), r)
	}
	return result, true
}

func (c *mockCache) Delete(ctx context.Context, key dataloader.Key) bool {
	_, ok := c.r[key.String()]
	if ok {
		delete(c.r, key.String())
		return true
	}
	return false
}

func (c *mockCache) ClearAll(ctx context.Context) bool {
	c.r = make(map[string]dataloader.Result, len(c.r))
	return true
}

// ========================= mock strategy =========================
type mockStrategy struct {
	batchFunc dataloader.BatchFunction
}

func newMockStrategy() func(int, dataloader.BatchFunction) dataloader.Strategy {
	return func(capacity int, batch dataloader.BatchFunction) dataloader.Strategy {
		return &mockStrategy{
			batchFunc: batch,
		}
	}
}

func (s *mockStrategy) Load(ctx context.Context, key dataloader.Key) dataloader.Thunk {
	return func() dataloader.Result {
		keys := dataloader.NewKeys(1)
		keys.Append(key)
		r := s.batchFunc(ctx, keys)

		return (*r).GetValue(key)
	}
}

func (s *mockStrategy) LoadMany(ctx context.Context, keyArr ...dataloader.Key) dataloader.ThunkMany {
	return func() dataloader.ResultMap {
		keys := dataloader.NewKeys(len(keyArr))
		for _, k := range keyArr {
			keys.Append(k)
		}
		r := s.batchFunc(ctx, keys)
		return *r
	}
}

func (s *mockStrategy) LoadNoOp(ctx context.Context) {}

// ================================================== tests ==================================================
/*
	NOTE: cache is not go routine safe. Hence mock strategy executes
*/

// ============================================= test cache hits =============================================
// TestLoadCacheHit tests to ensure that a cache hit doesn't call the callback function.
func TestLoadCacheHit(t *testing.T) {
	// setup
	callCount := 0
	result := dataloader.Result{Result: "cache_miss", Err: nil}
	expectedResult := dataloader.Result{Result: "cache_hit", Err: nil}
	cb := func() { callCount += 1 }
	cache := newMockCache(1)
	key := PrimaryKey(1)
	cache.SetResult(context.Background(), key, expectedResult)

	batch := getBatchFunction(cb, result)
	strategy := newMockStrategy()
	loader := dataloader.NewDataLoader(1, batch, strategy, dataloader.WithCache(cache))

	// invoke / assert

	thunk := loader.Load(context.Background(), key)
	r := thunk()
	assert.Equal(t, expectedResult.Result.(string), r.Result.(string), "Expected result from thunk")
	assert.Equal(t, 0, callCount, "Expected batch function to not be called")
}

// TestLoadManyCacheHit tests to ensure that a cache hit doesn't call the callback function.
func TestLoadManyCacheHit(t *testing.T) {
	// setup
	callCount := 0
	result := dataloader.Result{Result: "cache_miss", Err: nil}
	expectedResult := dataloader.Result{Result: "cache_hit", Err: nil}
	expectedResult2 := dataloader.Result{Result: "cache_hit_2", Err: nil}
	cb := func() { callCount += 1 }
	cache := newMockCache(2)
	key := PrimaryKey(1)
	key2 := PrimaryKey(2)
	cache.SetResult(context.Background(), key, expectedResult)
	cache.SetResult(context.Background(), key2, expectedResult2)

	batch := getBatchFunction(cb, result)
	strategy := newMockStrategy()
	loader := dataloader.NewDataLoader(1, batch, strategy, dataloader.WithCache(cache))

	// invoke / assert

	thunk := loader.LoadMany(context.Background(), key, key2)
	r := thunk()
	assert.Equal(t,
		expectedResult.Result.(string),
		r.(dataloader.ResultMap).GetValue(key).Result.(string),
		"Expected result from thunk",
	)
	assert.Equal(t, 2, r.(dataloader.ResultMap).Length(), "Expected 2 result from cache")
	assert.Equal(t, 0, callCount, "Expected batch function to not be called")
}

// ============================================= test cache miss =============================================

// TestLoadCacheMiss ensures the batch function is called on a cache miss
func TestLoadCacheMiss(t *testing.T) {
	// setup
	callCount := 0
	result := dataloader.Result{Result: "cache_miss", Err: nil}
	cb := func() { callCount += 1 }
	cache := newMockCache(1)
	key := PrimaryKey(1)

	batch := getBatchFunction(cb, result)
	strategy := newMockStrategy()
	loader := dataloader.NewDataLoader(1, batch, strategy, dataloader.WithCache(cache))

	// invoke / assert

	thunk := loader.Load(context.Background(), key)
	r := thunk()
	assert.Equal(t, result.Result.(string), r.Result.(string), "Expected result from thunk")
	assert.Equal(t, 1, callCount, "Expected batch function to  be called")
}

// TestLoadManyCacheMiss ensures the batch function is called on a cache miss
func TestLoadManyCacheMiss(t *testing.T) {
	// setup
	callCount := 0
	result := dataloader.Result{Result: "cache_miss", Err: nil}
	cb := func() { callCount += 1 }
	cache := newMockCache(1)
	key := PrimaryKey(1)

	batch := getBatchFunction(cb, result)
	strategy := newMockStrategy()
	loader := dataloader.NewDataLoader(1, batch, strategy, dataloader.WithCache(cache))

	// invoke / assert

	thunk := loader.LoadMany(context.Background(), key)
	r := thunk()
	assert.Equal(t,
		result.Result.(string),
		r.(dataloader.ResultMap).GetValue(key).Result.(string),
		"Expected result from thunk",
	)
	assert.Equal(t, 1, callCount, "Expected batch function to  be called")
}
