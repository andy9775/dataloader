package dataloader

import (
	"context"
	"sync"
)

// DataLoader is the identifying interface for the dataloader.
// Each DataLoader instance tracks the resolved elements
type DataLoader interface {
	// Capacity returns the capacity of the DataLoader instance.
	// The capacity dictates the amount of elements to be added to the keys array
	// before the batch function is called.
	Capacity() int
	// Length returns the current length of the Keys array. Each time Load() is called,
	// the length increments
	Length() int

	// Load returns the Result for the specified Key.
	Load(context.Context, Key) Result
}

// BatchFunction is called with n keys after the keys passed to the loader reach
// the loader capacity
type BatchFunction func(context.Context, Keys) *ResultMap

// NewDataLoader returns a new DataLoader with a count capacity of `capacity`.
// The capacity value determines
func NewDataLoader(capacity int, batchFunc BatchFunction) DataLoader {
	wg := sync.WaitGroup{}
	/*
		Adding +1 to the wait group allows the batch function executer to decrement
		the wait group counter once the batch function has resolved and returned
	*/
	wg.Add(capacity + 1)

	return &dataloader{
		waitGroup: &wg,
		keys:      make(Keys, 0, capacity),
		keysMutex: &sync.Mutex{},
		batchFunc: batchFunc,
	}
}

// ================================================================================================

type dataloader struct {
	// Track the keys to pass to the batch function. Once len(keys) == cap(keys),
	// the batch loading function is called with the keys to resolve.
	keys      Keys
	waitGroup *sync.WaitGroup
	keysMutex *sync.Mutex
	results   *ResultMap
	batchFunc BatchFunction
}

func (d *dataloader) Capacity() int {
	return cap(d.keys)
}

func (d *dataloader) Length() int {
	return len(d.keys)
}

// Load returns the Result for the specified Key.
// Internally Load adds Key to the Keys array and blocks the caller until the batch loader
// function resolves. Once resolved, Load returns the data to the caller for the specified
// Key
func (d *dataloader) Load(ctx context.Context, key Key) Result {
	/*
		If no results have been set, it means the batch function hasn't run,
		therefore wait if necessary. Else if results are not empty, just return
		the resulting value for the Key
	*/

	if d.results == nil || d.results.isEmpty() {
		canExecBatchFunc := d.addKey(key)
		if canExecBatchFunc {
			go func() {
				defer d.waitGroup.Done()

				d.results = d.batchFunc(ctx, d.keys)
			}()
		}

		d.waitGroup.Wait()
	}

	return (*d.results)[key.String()]
}

// ============================================== private =============================================
// addKey handles adding a new key to the Keys tracker.
// Returns true if the batch function can execute
func (d *dataloader) addKey(key Key) bool {
	d.keysMutex.Lock()
	defer d.keysMutex.Unlock()

	d.keys = append(d.keys, key)
	d.waitGroup.Done()

	return d.Length() == d.Capacity()
}
