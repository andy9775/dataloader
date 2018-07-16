package dataloader

import (
	"context"
	"sync"
	"time"
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
	// Internally load adds the provided key to the keys array and blocks until a result
	// is returned.
	Load(context.Context, Key) Result
}

// Options contains the settings the user for the dataloader
type Options struct {
	Timeout time.Duration
}

// BatchFunction is called with n keys after the keys passed to the loader reach
// the loader capacity
type BatchFunction func(context.Context, Keys) *ResultMap

// NewDataLoader returns a new DataLoader with a count capacity of `capacity`.
// The capacity value determines when the batch loader function will execute.
func NewDataLoader(capacity int, batchFunc BatchFunction, opts Options) DataLoader {
	formatOptions(&opts)

	// TODO: requests block on adding keys to key channel if channel capacity is less than 5
	keyChanCapacity := capacity
	if capacity < 5 {
		keyChanCapacity = 5
	}

	return &dataloader{
		keys:        NewKeys(capacity),
		workerMutex: &sync.Mutex{},
		keyChan:     make(chan Key, keyChanCapacity),

		goroutineStatus: notRunning,

		batchFunc: batchFunc,
		options:   opts,
	}
}

// ================================================================================================

// go routine status values
// Ensure that only one worker go routine is working to call the batch function
const (
	notRunning = int32(0) // go routine default start value
	running    = int32(1) // go routine is waiting for keys array to fill up
	ran        = int32(2) // go routine ran
)

type dataloader struct {
	// Track the keys to pass to the batch function. Once len(keys) == cap(keys),
	// the batch loading function is called with the keys to resolve.
	keys      Keys
	results   *ResultMap
	batchFunc BatchFunction

	workerMutex *sync.Mutex

	keyChan   chan Key
	closeChan chan struct{}

	goroutineStatus int32

	options Options
}

func (d *dataloader) Capacity() int {
	return d.keys.Capacity()
}

func (d *dataloader) Length() int {
	return d.keys.Length()
}

// Load returns the Result for the specified Key.
// Internally Load adds Key to the Keys array and blocks the caller until the batch loader
// function resolves. Once resolved, Load returns the data to the caller for the specified
// Key
func (d *dataloader) Load(ctx context.Context, key Key) Result {
	d.startWorker()

	d.keyChan <- key // pass key to the worker go routine (buffered channel)

	<-d.closeChan // wait for the worker to complete and channel to close

	if r := d.getResult(key); r == nil {
		return (*d.batchFunc(nil, newKeys(key))).GetValue(key)
	} else if r == MissingValue {
		return nil
	} else {
		return r
	}
}

// ============================================== private =============================================

func (d *dataloader) startWorker() {
	d.workerMutex.Lock() // ensure only one worker is started
	defer d.workerMutex.Unlock()

	if d.goroutineStatus == notRunning {
		d.goroutineStatus = running
		d.closeChan = make(chan struct{})

		go func() {
			defer func() {
				d.goroutineStatus = ran
				d.keys.ClearAll()
				close(d.closeChan)
			}()

			// loop while adding keys or timeout
			for d.results == nil {
				select {
				case key := <-d.keyChan:
					if d.keys.Append(key) { // hit capacity
						d.results = d.batchFunc(nil, d.keys)
					}
				case <-time.After(d.options.Timeout):
					d.results = d.batchFunc(nil, d.keys)
				}

			}
		}()
	}
}

// ============================================== helpers =============================================

// formatOptions configures default values for the loader options
func formatOptions(opts *Options) {
	opts.Timeout |= 6 * time.Millisecond
}

func (d *dataloader) getResult(key Key) Result {
	if d.results != nil {
		return (*d.results).GetValue(key)
	}
	return nil
}
