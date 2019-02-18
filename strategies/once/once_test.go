package once_test

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/andy9775/dataloader"
	"github.com/andy9775/dataloader/strategies/once"
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

type mockLogger struct {
	logMsgs []string
	m       sync.Mutex
}

func (l *mockLogger) Log(v ...interface{}) {
	l.m.Lock()
	defer l.m.Unlock()

	for _, value := range v {
		switch i := value.(type) {
		case string:
			l.logMsgs = append(l.logMsgs, i)
		default:
			panic("mock logger only takes single log string")
		}
	}
}

func (l *mockLogger) Logf(format string, v ...interface{}) {
	l.m.Lock()
	defer l.m.Unlock()

	l.logMsgs = append(l.logMsgs, fmt.Sprintf(format, v...))
}

func (l *mockLogger) Messages() []string {
	l.m.Lock()
	defer l.m.Unlock()

	result := make([]string, len(l.logMsgs))
	copy(result, l.logMsgs)
	return result
}

// ================================================== tests ==================================================

// ========================= foreground calls =========================
// TestBatchInForegroundCalled asserts that the once strategy will call the batch function only
// once the thunk function is called (returned from `Load`). It also checks for the returned
// result
func TestBatchLoadInForegroundCalled(t *testing.T) {
	// setup
	callCount := 0
	expectedResult := "result_foreground_load"
	cb := func() { callCount += 1 }

	key := PrimaryKey(1)
	result := dataloader.Result{Result: expectedResult, Err: nil}

	batch := getBatchFunction(cb, result)
	strategy := once.NewOnceStrategy()(5, batch)

	// invoke/assert

	thunk := strategy.Load(context.Background(), key)
	assert.Equal(t, 0, callCount, "Batch function not expected to be called on Load()")

	r, ok := thunk()
	assert.True(t, ok, "Expected result to have been found")
	assert.Equal(t, 1, callCount, "Batch function expected to be called on thunk()")
	assert.Equal(t, expectedResult, r.Result.(string), "Expected result from batch function")

	r, ok = thunk()
	assert.True(t, ok, "Expected result to have been found")
	assert.Equal(t, 1, callCount, "Batch function expected to be called on thunk()")
	assert.Equal(t, expectedResult, r.Result.(string), "Expected result from batch function")
}

// TestBatchInForegroundCalled asserts that the once strategy will call the batch function only
// once the thunk function is called (returned from `LoadMany`). It also checks for the
// returned result
func TestBatchLoadManyInForegroundCalled(t *testing.T) {
	// setup
	callCount := 0
	expectedResult := "result_foreground_load_many"
	cb := func() { callCount += 1 }

	key := PrimaryKey(1)
	result := dataloader.Result{Result: expectedResult, Err: nil}

	batch := getBatchFunction(cb, result)
	strategy := once.NewOnceStrategy()(5, batch)

	// invoke/assert

	thunkMany := strategy.LoadMany(context.Background(), key, key)
	assert.Equal(t, 0, callCount, "Batch function not expected to be called on LoadMany()")

	r := thunkMany()
	returned, ok := r.GetValue(key)
	assert.True(t, ok, "Expected result to have been found")
	assert.Equal(t, 1, callCount, "Batch function expected to be called on thunkMany()")
	assert.Equal(t, expectedResult, returned.Result.(string), "Expected result from batch function")

	r = thunkMany()
	returned, ok = r.GetValue(key)
	assert.True(t, ok, "Expected result to have been found")
	assert.Equal(t, 1, callCount, "Batch function expected to be called on thunkMany()")
	assert.Equal(t, expectedResult, returned.Result.(string), "Expected result from batch function")
}

// ========================= background calls =========================
/*
blockWG is used as a shortcut to assert that the Load/LoadMany calls don't block the caller and do return a
non-blocking callback (Thunk/ThunkMany). blockWG blocks the batch function from executing until Done() is
called. If the Load() call blocks (via blockWG.Wait()), then the test will timeout and panic.
*/

// TestBatchLoadInBackgroundCalled asserts that the once strategy will call the batch function
// when Load is called. It also checks for the returned result
func TestBatchLoadInBackgroundCalled(t *testing.T) {
	// setup
	wg := sync.WaitGroup{} // ensure batch function called before asserting
	wg.Add(1)
	// blockWG blocks the callback function allowing the test to assert that Load() function doesn't block
	blockWG := sync.WaitGroup{}
	blockWG.Add(1)
	closeChan := make(chan struct{})
	timeout(t, closeChan, TEST_TIMEOUT)

	callCount := 0
	expectedResult := "result_background_load"
	cb := func() { blockWG.Wait(); callCount = +1; close(closeChan); wg.Done() }

	key := PrimaryKey(1)
	result := dataloader.Result{Result: expectedResult, Err: nil}

	batch := getBatchFunction(cb, result)
	strategy := once.NewOnceStrategy(once.WithInBackground())(5, batch)

	// invoke/assert

	thunk := strategy.Load(context.Background(), key)
	assert.Equal(t, 0, callCount, "Load() not expected to block and call batch function")
	blockWG.Done() // allow callback function to complete in background
	wg.Wait()      // wait for callback to complete

	assert.Equal(t, 1, callCount, "Batch function expected to be called on Load() in background")

	r, ok := thunk()
	assert.True(t, ok, "Expected result to have been found")
	assert.Equal(t, expectedResult, r.Result.(string), "Expected value from batch function")

	r, ok = thunk()
	assert.True(t, ok, "Expected result to have been found")
	assert.Equal(t, expectedResult, r.Result.(string), "Expected value from batch function")
	assert.Equal(t, 1, callCount, "Batch function expected to be called on Load() in background")
}

// TestBatchLoadManyInBackgroundCalled asserts that the once strategy will call the batch function
// when LoadMany is called. It also checks for the returned result
func TestBatchLoadManyInBackgroundCalled(t *testing.T) {
	// setup
	wg := sync.WaitGroup{} // ensure batch function called before asserting
	wg.Add(1)
	// blockWG blocks the callback function allowing the test to assert that Load() function doesn't block
	blockWG := sync.WaitGroup{}
	blockWG.Add(1)
	closeChan := make(chan struct{})
	timeout(t, closeChan, TEST_TIMEOUT)

	callCount := 0
	expectedResult := "result_background_load_many"
	cb := func() { blockWG.Wait(); callCount += 1; close(closeChan); wg.Done() }

	key := PrimaryKey(1)
	result := dataloader.Result{Result: expectedResult, Err: nil}

	batch := getBatchFunction(cb, result)
	strategy := once.NewOnceStrategy(once.WithInBackground())(5, batch)

	// invoke/assert

	thunkMany := strategy.LoadMany(context.Background(), key, key)
	assert.Equal(t, 0, callCount, "LoadMany() not expected to block and call batch function")
	blockWG.Done() // allow callback function to complete in background
	wg.Wait()      // wait for callback to complete

	assert.Equal(t, 1, callCount, "Batch function expected to be called on LoadMany()")

	r := thunkMany()
	returned, ok := r.GetValue(key)
	assert.True(t, ok, "Expected result to have been found")
	assert.Equal(t, expectedResult, returned.Result.(string), "Expected result from batch function")

	r = thunkMany()
	returned, ok = r.GetValue(key)
	assert.True(t, ok, "Expected result to have been found")
	assert.Equal(t, expectedResult, returned.Result.(string), "Expected result from batch function")
	assert.Equal(t, 1, callCount, "Batch function expected to be called on LoadMany()")
}

// =========================================== cancellable context ===========================================

// TestCancellableContextLoadMany ensures that a call to cancel the context kills the background worker
func TestCancellableContextLoadMany(t *testing.T) {
	// setup
	closeChan := make(chan struct{})
	timeout(t, closeChan, TEST_TIMEOUT*3)

	expectedResult := "cancel_via_context"
	cb := func() {
		close(closeChan)
	}

	key := PrimaryKey(1)
	result := dataloader.Result{Result: expectedResult, Err: nil}
	/*
		ensure the loader doesn't call batch after timeout. If it does, the test will timeout and panic
	*/
	log := mockLogger{logMsgs: make([]string, 2), m: sync.Mutex{}}
	batch := getBatchFunction(cb, result)
	strategy := once.NewOnceStrategy(
		once.WithLogger(&log),
		once.WithInBackground(),
	)(2, batch) // expected 2 load calls
	ctx, cancel := context.WithCancel(context.Background())

	// invoke
	cancel()
	thunk := strategy.LoadMany(ctx, key)
	thunk()
	time.Sleep(100 * time.Millisecond)

	// assert
	m := log.Messages()
	assert.Equal(t, "worker cancelled", m[len(m)-1], "Expected worker to cancel and log exit")
}

// =============================================== result keys ===============================================
// TestKeyHandling ensures that processed and unprocessed keys by the batch function are handled correctly
// This test accomplishes this by skipping processing a single key and then asserts that they skipped key
// returns the correct ok value when loading the data
func TestKeyHandling(t *testing.T) {
	// setup
	expectedResult := map[PrimaryKey]interface{}{
		PrimaryKey(1): "valid_result",
		PrimaryKey(2): nil,
		PrimaryKey(3): "__skip__", // this key should be skipped by the batch function
	}

	batch := func(ctx context.Context, keys dataloader.Keys) *dataloader.ResultMap {
		m := dataloader.NewResultMap(2)
		for i := 0; i < keys.Length(); i++ {
			key := keys.Keys()[i].(PrimaryKey)
			if expectedResult[key] != "__skip__" {
				m.Set(key.String(), dataloader.Result{Result: expectedResult[key], Err: nil})
			}
		}
		return &m
	}

	// invoke/assert

	// iterate through multiple strategies table test style
	strategies := []dataloader.Strategy{
		once.NewOnceStrategy()(3, batch),
		once.NewOnceStrategy(once.WithInBackground())(3, batch),
	}
	for _, strategy := range strategies {

		// Load
		for key, expected := range expectedResult {
			thunk := strategy.Load(context.Background(), key)
			r, ok := thunk()

			switch expected.(type) {
			case string:
				if expected == "__skip__" {
					assert.False(t, ok, "Expected skipped result to not be found")
					assert.Nil(t, r.Result, "Expected skipped result to be nil")
				} else {
					assert.True(t, ok, "Expected processed result to be found")
					assert.Equal(t, r.Result, expected, "Expected result")
				}
			case nil:
				assert.True(t, ok, "Expected processed result to be found")
				assert.Nil(t, r.Result, "Expected result to be nil")
			}
		}

		// LoadMany
		thunkMany := strategy.LoadMany(context.Background(), PrimaryKey(1), PrimaryKey(2), PrimaryKey(3))
		for key, expected := range expectedResult {
			result := thunkMany()
			r, ok := result.GetValue(key)

			switch expected.(type) {
			case string:
				if expected == "__skip__" {
					assert.False(t, ok, "Expected skipped result to not be found")
					assert.Nil(t, r.Result, "Expected skipped result to be nil")
				} else {
					assert.True(t, ok, "Expected processed result to be found")
					assert.Equal(t, r.Result, expected, "Expected result")
				}
			case nil:
				assert.True(t, ok, "Expected processed result to be found")
				assert.Nil(t, r.Result, "Expected result to be nil")
			}

		}
	}
}
