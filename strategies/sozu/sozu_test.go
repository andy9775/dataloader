package sozu_test

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/andy9775/dataloader"
	"github.com/andy9775/dataloader/strategies/sozu"
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

// ========================= test timeout =========================
// TestLoadTimeoutTriggered ensures that the timeout function triggers the batch function when not enough load
// calls occur
func TestLoadTimeoutTriggered(t *testing.T) {
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
		close(closeChan)
		wg.Done()
	}

	toWG := sync.WaitGroup{}
	toWG.Add(1)
	timedOut := false
	monkey.Patch(time.After, func(t time.Duration) <-chan time.Time {
		defer monkey.Unpatch(time.After)
		toChan := make(chan time.Time, 1)

		go func() {
			time.Sleep(t)
			timedOut = true
			toWG.Done()
			toChan <- time.Now()
		}()

		return toChan
	})

	key := PrimaryKey(1)
	key2 := PrimaryKey(2)

	batch := getBatchFunction(cb, expectedResult)
	strategy := sozu.NewSozuStrategy()(10, batch) // expects 10 load calls

	// invoke/assert

	// load twice with data, once simulating a cache hit
	strategy.Load(context.Background(), key)           // --------- Load 		 - call 1
	thunk := strategy.Load(context.Background(), key2) // --------- Load 		 - call 2
	strategy.LoadNoOp(context.Background())            // --------- LoadNoOp - call 3
	assert.Equal(t, 0, callCount, "Load() not expected to block or call batch function")
	blockWG.Done() // allow batch function to execute
	wg.Wait()      // wait for batch function to execute

	assert.Equal(t, 1, callCount, "Batch function expected to be called once")
	assert.Equal(t, 2, len(k), "Expected to be called with 2 keys")

	toWG.Wait()
	assert.True(t, timedOut, "Expected function to timeout")

	r := thunk()
	assert.Equal(
		t,
		fmt.Sprintf("2_%s", expectedResult),
		r.Result.(string),
		"Expected batch function to return result",
	)

	// test double call to thunk
	r = thunk()
	assert.Equal(
		t,
		fmt.Sprintf("2_%s", expectedResult),
		r.Result.(string),
		"Expected batch function to return result",
	)
	assert.Equal(t, 1, callCount, "Batch function expected to be called once")
}

// TestLoadManyTimeoutTriggered ensures that the timeout function triggers the batch function
// when not enough load calls occur.
func TestLoadManyTimeoutTriggered(t *testing.T) {
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
		close(closeChan)
		wg.Done()
	}

	toWG := sync.WaitGroup{}
	toWG.Add(1)
	timedOut := false
	monkey.Patch(time.After, func(t time.Duration) <-chan time.Time {
		defer monkey.Unpatch(time.After)
		toChan := make(chan time.Time, 1)

		go func() {
			time.Sleep(t)
			timedOut = true
			toWG.Done()
			toChan <- time.Now()
		}()

		return toChan
	})

	key := PrimaryKey(1)
	key2 := PrimaryKey(2)
	key3 := PrimaryKey(3)

	batch := getBatchFunction(cb, expectedResult)
	strategy := sozu.NewSozuStrategy()(10, batch) // expected 10 load calls

	// invoke/assert

	// load twice with data, once simulating a cache hit
	thunkMany := strategy.LoadMany(context.Background(), key)         // --------- LoadMany - call 1
	thunkMany2 := strategy.LoadMany(context.Background(), key2, key3) // --------- LoadMany - call 2
	strategy.LoadNoOp(context.Background())                           // --------- LoadNoOp - call 3
	assert.Equal(t, 0, callCount, "LoadMany() not expected to block or call batch function")
	blockWG.Done() // allow batch function to execute
	wg.Wait()      // wait for batch function to execute

	assert.Equal(t, 1, callCount, "Batch function expected to be called once")
	// capacity is 10, called 2 times with 3 unique keys
	assert.Equal(t, 3, len(k), "Expected batch function to be called with 3 keys")

	toWG.Wait()
	assert.True(t, timedOut, "Expected function to have timed out")

	r1 := thunkMany2()
	assert.Equal(t,
		fmt.Sprintf("2_%s", expectedResult),
		r1.(dataloader.ResultMap).GetValue(key2).Result.(string),
		"Expected batch function to return on thunkMany()",
	)

	r2 := thunkMany()
	assert.Equal(
		t,
		3,
		r1.(dataloader.ResultMap).Length()+r2.(dataloader.ResultMap).Length(),
		"Expected 3 total results from both thunkMany function",
	)

	// test double call to thunk
	r2 = thunkMany()
	assert.Equal(
		t,
		3,
		r1.(dataloader.ResultMap).Length()+r2.(dataloader.ResultMap).Length(),
		"Expected 3 total results from both thunkMany function",
	)
	assert.Equal(t, 1, callCount, "Batch function expected to be called once")
}

// ========================= test non-timeout =========================

// TestLoadTriggered asserts that load calls do not timeout and call the batch function after
// capacity is hit
func TestLoadTriggered(t *testing.T) {
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

	/*
		ensure the loader doesn't call batch after timeout. If it does, the test will timeout and panic
	*/
	timeout := TEST_TIMEOUT * 5
	batch := getBatchFunction(cb, expectedResult)
	strategy := sozu.NewSozuStrategy(sozu.WithTimeout(timeout))(2, batch) // expected 2 load calls

	// invoke/assert

	// load once with data, once simulating a cache hit
	thunk := strategy.Load(context.Background(), key) // --------- Load     -  call 1
	strategy.LoadNoOp(context.Background())           // --------- LoadNoOp -  call 2
	assert.Equal(t, 0, callCount, "Load() not expected to block or call batch function")
	blockWG.Done() // allow batch function to execute
	wg.Wait()      // wait for batch function to execute

	assert.Equal(t, 1, callCount, "Batch function expected to be called once")
	// capacity is 2, called with 1 key and 1 cache hit
	assert.Equal(t, 1, len(k), "Expected to be called with 1 keys")
	r1 := thunk()
	assert.Equal(
		t,
		fmt.Sprintf("1_%s", expectedResult),
		r1.Result.(string),
		"Expected batch function to return on thunk()",
	)

	assert.False(t, timedOut, "Expected function to not timeout")

	// test double call to thunk
	r1 = thunk()
	assert.Equal(
		t,
		fmt.Sprintf("1_%s", expectedResult),
		r1.Result.(string),
		"Expected batch function to return on thunk()",
	)
	assert.Equal(t, 1, callCount, "Batch function expected to be called once")
}

// TestLoadManyTriggered asserts that load calls do not timeout and call the batch function after
// capacity is hit
func TestLoadManyTriggered(t *testing.T) {
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

	/*
		ensure the loader doesn't call batch after timeout. If it does, the test will timeout and panic
	*/
	timeout := TEST_TIMEOUT * 5
	batch := getBatchFunction(cb, expectedResult)
	strategy := sozu.NewSozuStrategy(sozu.WithTimeout(timeout))(2, batch) // expected 2 load calls

	// invoke/assert

	// load once with data, once simulating a cache hit
	thunk := strategy.LoadMany(context.Background(), key, key2) // --------- LoadMany - call 1
	strategy.LoadNoOp(context.Background())                     // --------- LoadNoOp - call 2
	assert.Equal(t, 0, callCount, "Load() not expected to block or call batch function")
	blockWG.Done() // allow batch function to execute
	wg.Wait()      // wait for batch function to execute

	assert.Equal(t, 1, callCount, "Batch function expected to be called once ")
	// capacity is 2, called with 2 keys and one cache hit
	assert.Equal(t, 2, len(k), "Expected to be called with 2 keys")
	r1 := thunk()
	assert.Equal(
		t,
		fmt.Sprintf("1_%s", expectedResult),
		r1.(dataloader.ResultMap).GetValue(key).Result,
		"Expected batch function to return on thunk()",
	)

	assert.False(t, timedOut, "Expected function to not timeout")

	// test double call to thunk
	r1 = thunk() // don't block on second call
	assert.Equal(
		t,
		fmt.Sprintf("1_%s", expectedResult),
		r1.(dataloader.ResultMap).GetValue(key).Result,
		"Expected batch function to return on thunk()",
	)
	assert.Equal(t, 1, callCount, "Batch function expected to be called once ")
}

// TestLoadBlocked calls thunk without using a wait group and expects to be blocked before getting data back.
func TestLoadBlocked(t *testing.T) {
	// setup
	closeChan := make(chan struct{})
	timeout(t, closeChan, TEST_TIMEOUT)

	var k []interface{}
	callCount := 0
	expectedResult := "batch_on_timeout_load_many"
	cb := func(keys dataloader.Keys) {
		callCount += 1
		k = keys.Keys()
		close(closeChan)
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

	/*
		ensure the loader doesn't call batch after timeout. If it does, the test will timeout and panic
	*/
	timeout := TEST_TIMEOUT * 5
	batch := getBatchFunction(cb, expectedResult)
	strategy := sozu.NewSozuStrategy(sozu.WithTimeout(timeout))(2, batch) // expected 2 load calls

	// invoke/assert

	// load once with data, once simulating a cache hit
	thunk := strategy.Load(context.Background(), key) // --------- Load     -  call 1
	strategy.LoadNoOp(context.Background())           // --------- LoadNoOp -  call 2

	r := thunk() // block until batch function executes

	assert.Equal(t, 1, callCount, "Batch function should have been called once")
	assert.False(t, timedOut, "Batch function should not have timed out")
	assert.Equal(t, 1, len(k), "Should have been called with one key")
	assert.Equal(t, fmt.Sprintf("1_%s", expectedResult), r.Result.(string), "Expected result from thunk()")

	// test double call to thunk
	r = thunk() // don't block on second call

	assert.Equal(t, 1, callCount, "Batch function should have been called once")
	assert.False(t, timedOut, "Batch function should not have timed out")
	assert.Equal(t, 1, len(k), "Should have been called with one key")
	assert.Equal(t, fmt.Sprintf("1_%s", expectedResult), r.Result.(string), "Expected result from thunk()")
}

// TestLoadManyBlocked calls thunkMany without using a wait group and expects to be blocked before
// getting data back.
func TestLoadManyBlocked(t *testing.T) {
	// setup
	closeChan := make(chan struct{})
	timeout(t, closeChan, TEST_TIMEOUT)

	var k []interface{}
	callCount := 0
	expectedResult := "batch_on_timeout_load_many"
	cb := func(keys dataloader.Keys) {
		callCount += 1
		k = keys.Keys()
		close(closeChan)
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

	/*
		ensure the loader doesn't call batch after timeout. If it does, the test will timeout and panic
	*/
	timeout := TEST_TIMEOUT * 5
	batch := getBatchFunction(cb, expectedResult)
	strategy := sozu.NewSozuStrategy(sozu.WithTimeout(timeout))(2, batch) // expected 2 load calls

	// invoke/assert

	// load once with data, once simulating a cache hit
	thunkMany := strategy.LoadMany(context.Background(), key, key2) // --------- Load     -  call 1
	strategy.LoadNoOp(context.Background())                         // --------- LoadNoOp -  call 2

	r := thunkMany() // block until batch function executes

	assert.Equal(t, 1, callCount, "Batch function should have been called once")
	assert.False(t, timedOut, "Batch function should not have timed out")
	assert.Equal(t, 2, len(k), "Should have been called with two keys")
	assert.Equal(
		t,
		fmt.Sprintf("2_%s", expectedResult),
		r.(dataloader.ResultMap).GetValue(key2).Result.(string),
		"Expected result from thunkMany()",
	)

	// test double call to thunk
	r = thunkMany() // don't block on second call

	assert.Equal(t, 1, callCount, "Batch function should have been called once")
	assert.False(t, timedOut, "Batch function should not have timed out")
	assert.Equal(t, 2, len(k), "Should have been called with two keys")
	assert.Equal(
		t,
		fmt.Sprintf("2_%s", expectedResult),
		r.(dataloader.ResultMap).GetValue(key2).Result.(string),
		"Expected result from thunkMany()",
	)
}

// =========================================== cancellable context ===========================================

// TestCancellableContextLoad ensures that a call to cancel the context kills the background worker
func TestCancellableContextLoad(t *testing.T) {
	// setup
	closeChan := make(chan struct{})
	timeout(t, closeChan, TEST_TIMEOUT*3)

	callCount := 0
	expectedResult := "cancel_via_context"
	cb := func(keys dataloader.Keys) {
		callCount += 1
		close(closeChan)
	}

	key := PrimaryKey(1)
	log := mockLogger{logMsgs: make([]string, 2), m: sync.Mutex{}}
	batch := getBatchFunction(cb, expectedResult)
	strategy := sozu.NewSozuStrategy(
		sozu.WithLogger(&log),
	)(2, batch) // expected 2 load calls
	ctx, cancel := context.WithCancel(context.Background())

	// invoke
	go cancel()
	thunk := strategy.Load(ctx, key)
	thunk()
	time.Sleep(100 * time.Millisecond)

	// assert
	assert.Equal(t, 0, callCount, "Batch should not have been called")
	m := log.Messages()
	assert.Equal(t, "worker cancelled", m[len(m)-1], "Expected worker to cancel and log exit")
}

// TestCancellableContextLoadMany ensures that a call to cancel the context kills the background worker
func TestCancellableContextLoadMany(t *testing.T) {
	// setup
	closeChan := make(chan struct{})
	timeout(t, closeChan, TEST_TIMEOUT*3)

	callCount := 0
	expectedResult := "cancel_via_context"
	cb := func(keys dataloader.Keys) {
		callCount += 1
		close(closeChan)
	}

	key := PrimaryKey(1)
	log := mockLogger{logMsgs: make([]string, 2), m: sync.Mutex{}}
	batch := getBatchFunction(cb, expectedResult)
	strategy := sozu.NewSozuStrategy(
		sozu.WithLogger(&log),
	)(2, batch) // expected 2 load calls
	ctx, cancel := context.WithCancel(context.Background())

	// invoke
	go cancel()
	thunk := strategy.LoadMany(ctx, key)
	thunk()
	time.Sleep(100 * time.Millisecond)

	// assert
	assert.Equal(t, 0, callCount, "Batch should not have been called")
	m := log.Messages()
	assert.Equal(t, "worker cancelled", m[len(m)-1], "Expected worker to cancel and log exit")
}
