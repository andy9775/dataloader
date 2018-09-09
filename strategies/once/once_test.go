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

	r := thunk()
	assert.Equal(t, 1, callCount, "Batch function expected to be called on thunk()")
	assert.Equal(t, expectedResult, r.Result.(string), "Expected result from batch function")

	r = thunk()
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
	assert.Equal(t, 1, callCount, "Batch function expected to be called on thunkMany()")
	assert.Equal(t, expectedResult, r.GetValue(key).Result.(string), "Expected result from batch function")

	r = thunkMany()
	assert.Equal(t, 1, callCount, "Batch function expected to be called on thunkMany()")
	assert.Equal(t, expectedResult, r.GetValue(key).Result.(string), "Expected result from batch function")
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

	r := thunk()
	assert.Equal(t, expectedResult, r.Result.(string), "Expected value from batch function")
	r = thunk()
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
	assert.Equal(t, expectedResult, r.GetValue(key).Result.(string), "Expected result from batch function")
	r = thunkMany()
	assert.Equal(t, expectedResult, r.GetValue(key).Result.(string), "Expected result from batch function")
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
