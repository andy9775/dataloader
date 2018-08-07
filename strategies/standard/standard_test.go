package standard_test

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/andy9775/dataloader"
	"github.com/andy9775/dataloader/strategies/standard"
	"github.com/bouk/monkey"
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
func getBatchFunction(cb func(dataloader.Keys), result string) dataloader.BatchFunction {
	return func(ctx context.Context, keys dataloader.Keys) *dataloader.ResultMap {
		cb(keys)
		m := dataloader.NewResultMap(1)
		for _, k := range keys.Keys() {
			key := k.(PrimaryKey).String()
			m.Set(
				key,
				dataloader.Result{
					// ensure each result in ResultMap is uniquely identifiable
					Result: fmt.Sprintf("%s_%s", key, result),
					Err:    nil,
				},
			)
		}
		return &m
	}
}

// timeout will panic if a test takes more than a defined time.
// `timeoutChannel chan struct{}` should be closed when the test completes in order to
// signal that it completed within the defined time
func timeout(t *testing.T, timeoutChannel chan struct{}, after time.Duration) {
	go func() {
		time.Sleep(after)
		select {
		case <-timeoutChannel:
			return
		default:
			panic(fmt.Sprintf("%s took too long to execute", t.Name()))
		}
	}()
}

// ================================================== tests ==================================================

// ================================================ no timeout ===============================================

// TestLoadNoTimeout tests calling the load function without timing out
func TestLoadNoTimeout(t *testing.T) {
	// setup
	wg := sync.WaitGroup{} // ensure batch function called before asserting
	wg.Add(1)

	// blockWG blocks the callback function allowing the test to assert that Load() function doesn't block
	blockWG := sync.WaitGroup{}
	blockWG.Add(1)
	closeChan := make(chan struct{})
	timeout(t, closeChan, TEST_TIMEOUT)

	var k []interface{}
	callCount := 0
	expectedResult := "batch_on_load"
	cb := func(keys dataloader.Keys) {
		blockWG.Wait()
		callCount += 1
		k = keys.Keys()
		close(closeChan)
		wg.Done()
	}

	timedOut := false
	monkey.Patch(time.After, func(t time.Duration) <-chan time.Time {
		defer monkey.Unpatch(time.After)
		toChan := make(chan time.Time, 1)

		go func() {
			time.Sleep(t)
			timedOut = true
			toChan <- time.Now()
		}()

		return toChan
	})

	key := PrimaryKey(1)
	key2 := PrimaryKey(2)

	opts := standard.Options{
		/*
			ensure the loader doesn't call batch after timeout. If it does, the test will timeout and panic
		*/
		Timeout: TEST_TIMEOUT * 5,
	}
	batch := getBatchFunction(cb, expectedResult)
	strategy := standard.NewStandardStrategy(batch, opts)(3) // expects 3 load calls

	// invoke/assert
	strategy.Load(context.Background(), key)           // --------- Load 		 - call 1
	thunk := strategy.Load(context.Background(), key2) // --------- Load 		 - call 2
	strategy.LoadNoOp(context.Background())            // --------- LoadNoOp - call 3
	assert.Equal(t, 0, callCount, "Load() not expected to block or call batch function")
	blockWG.Done()
	wg.Wait()

	assert.Equal(t, 1, callCount, "Batch function expected to be called once")
	assert.Equal(t, 2, len(k), "Expected to be called with 2 keys")

	r := thunk()
	assert.Equal(
		t,
		fmt.Sprintf("2_%s", expectedResult),
		r.Result.(string),
		"Expected result from thunk()",
	)

	assert.False(t, timedOut, "Expected loader not to timeout")
}

// TestLoadManyNoTimeout tests calling the load function without timing out
func TestLoadManyNoTimeout(t *testing.T) {
	// setup
	wg := sync.WaitGroup{} // ensure batch function called before asserting
	wg.Add(1)

	// blockWG blocks the callback function allowing the test to assert that Load() function doesn't block
	blockWG := sync.WaitGroup{}
	blockWG.Add(1)
	closeChan := make(chan struct{})
	timeout(t, closeChan, TEST_TIMEOUT)

	var k []interface{}
	callCount := 0
	expectedResult := "batch_on_load_many"
	cb := func(keys dataloader.Keys) {
		blockWG.Wait()
		callCount += 1
		k = keys.Keys()
		close(closeChan)
		wg.Done()
	}

	timedOut := false
	monkey.Patch(time.After, func(t time.Duration) <-chan time.Time {
		defer monkey.Unpatch(time.After)
		toChan := make(chan time.Time, 1)

		go func() {
			time.Sleep(t)
			timedOut = true
			toChan <- time.Now()
		}()

		return toChan
	})

	key := PrimaryKey(1)
	key2 := PrimaryKey(2)
	key3 := PrimaryKey(3)

	opts := standard.Options{
		/*
			ensure the loader doesn't call batch after timeout. If it does, the test will timeout and panic
		*/
		Timeout: TEST_TIMEOUT * 5,
	}
	batch := getBatchFunction(cb, expectedResult)
	strategy := standard.NewStandardStrategy(batch, opts)(3) // expects 3 load calls

	// invoke/assert
	strategy.LoadMany(context.Background(), key)                 // --------- LoadMany 		 - call 1
	thunk := strategy.LoadMany(context.Background(), key2, key3) // --------- LoadMany 		 - call 2
	strategy.LoadNoOp(context.Background())                      // --------- LoadNoOp 		 - call 3
	assert.Equal(t, 0, callCount, "Load() not expected to block or call batch function")
	blockWG.Done()
	wg.Wait()

	assert.Equal(t, 1, callCount, "Batch function expected to be called once")
	assert.Equal(t, 3, len(k), "Expected to be called with 2 keys")

	r := thunk()
	assert.Equal(
		t,
		fmt.Sprintf("2_%s", expectedResult),
		r.(dataloader.ResultMap).GetValue(key2).Result.(string),
		"Expected result from thunk()",
	)

	assert.False(t, timedOut, "Expected loader not to timeout")
}

// ================================================= timeout =================================================

// TestLoadTimeout tests that the first load call is performed after a timeout and the second occurs when the
// thunk is called.
func TestLoadTimeout(t *testing.T) {
	// setup
	wg := sync.WaitGroup{} // ensure batch function called before asserting
	wg.Add(1)

	// blockWG blocks the callback function allowing the test to assert that Load() function doesn't block
	blockWG := sync.WaitGroup{}
	blockWG.Add(1)
	closeChan := make(chan struct{})
	timeout(t, closeChan, TEST_TIMEOUT)

	var k []interface{}
	callCount := 0
	expectedResult := "batch_on_timeout_load"
	cb := func(keys dataloader.Keys) {
		blockWG.Wait()
		callCount += 1
		k = keys.Keys()
		if callCount == 2 {
			close(closeChan)
		}
		wg.Done()
	}

	timedOut := false
	monkey.Patch(time.After, func(t time.Duration) <-chan time.Time {
		defer monkey.Unpatch(time.After)
		toChan := make(chan time.Time, 1)

		go func() {
			time.Sleep(t)
			timedOut = true
			toChan <- time.Now()
		}()

		return toChan
	})

	key := PrimaryKey(1)
	key2 := PrimaryKey(2)

	opts := standard.Options{}
	batch := getBatchFunction(cb, expectedResult)
	strategy := standard.NewStandardStrategy(batch, opts)(3) // expects 3 load calls

	// invoke/assert

	thunk2 := strategy.Load(context.Background(), key2) // --------- Load 		 - call 1
	strategy.LoadNoOp(context.Background())             // --------- LoadNoOp  - call 2
	assert.Equal(t, 0, callCount, "Load() not expected to block or call batch function")
	blockWG.Done()
	wg.Wait()

	assert.Equal(t, 1, callCount, "Batch function expected to be called once")
	assert.Equal(t, 1, len(k), "Expected to be called with 1 key")
	assert.True(t, timedOut, "Expected loader to timeout")

	r := thunk2()
	assert.Equal(
		t,
		fmt.Sprintf("2_%s", expectedResult),
		r.Result.(string),
		"Expected result from thunk()",
	)

	// don't wait below - callback is not executing in background go routine - ensure wg doesn't go negative
	wg.Add(1)
	thunk := strategy.Load(context.Background(), key) // --------- Load 		 - call 3
	r = thunk()

	// called once in go routine after timeout, once in thunk
	assert.Equal(t, 2, callCount, "Batch function expected to be called twice")
	assert.Equal(t,
		fmt.Sprintf("1_%s", expectedResult),
		r.Result.(string),
		"Expected result from thunk",
	)
}

// TestLoadManyTimeout tests that the first load call is performed after a timeout and the second
// occurs when the thunk is called.
func TestLoadManyTimeout(t *testing.T) {
	// setup
	wg := sync.WaitGroup{} // ensure batch function called before asserting
	wg.Add(1)

	// blockWG blocks the callback function allowing the test to assert that Load() function doesn't block
	blockWG := sync.WaitGroup{}
	blockWG.Add(1)
	closeChan := make(chan struct{})
	timeout(t, closeChan, TEST_TIMEOUT)

	var k []interface{}
	callCount := 0
	expectedResult := "batch_on_timeout_load_many"
	cb := func(keys dataloader.Keys) {
		blockWG.Wait()
		callCount += 1
		k = keys.Keys()
		if callCount == 2 {
			close(closeChan)
		}
		wg.Done()
	}

	timedOut := false
	monkey.Patch(time.After, func(t time.Duration) <-chan time.Time {
		defer monkey.Unpatch(time.After)
		toChan := make(chan time.Time, 1)

		go func() {
			time.Sleep(t)
			timedOut = true
			toChan <- time.Now()
		}()

		return toChan
	})

	key := PrimaryKey(1)
	key2 := PrimaryKey(2)
	key3 := PrimaryKey(3)

	opts := standard.Options{}
	batch := getBatchFunction(cb, expectedResult)
	strategy := standard.NewStandardStrategy(batch, opts)(3) // expects 3 load calls

	// invoke/assert

	thunkMany2 := strategy.LoadMany(context.Background(), key2) // --------- LoadMany 		 - call 1
	strategy.LoadNoOp(context.Background())                     // --------- LoadNoOp      - call 2
	assert.Equal(t, 0, callCount, "Load() not expected to block or call batch function")
	blockWG.Done()
	wg.Wait()

	assert.Equal(t, 1, callCount, "Batch function expected to be called once")
	assert.Equal(t, 1, len(k), "Expected to be called with 1 key")
	assert.True(t, timedOut, "Expected loader to timeout")

	r := thunkMany2()
	assert.Equal(
		t,
		fmt.Sprintf("2_%s", expectedResult),
		r.(dataloader.ResultMap).GetValue(key2).Result.(string),
		"Expected result from thunkMany()",
	)

	// don't wait below - callback is not executing in background go routine - ensure wg doesn't go negative
	wg.Add(1)
	thunkMany := strategy.LoadMany(context.Background(), key, key3) // --------- LoadMany 		 - call 3
	r = thunkMany()

	// called once in go routine after timeout, once in thunkMany
	assert.Equal(t, 2, callCount, "Batch function expected to be called twice")
	assert.Equal(t,
		fmt.Sprintf("3_%s", expectedResult),
		r.(dataloader.ResultMap).GetValue(key3).Result.(string),
		"Expected result from thunkMany",
	)
	assert.Equal(t, 2, len(k), "Expected to be called with 2 keys") // second function call
}
