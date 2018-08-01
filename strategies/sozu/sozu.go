/*
Package sozu contains implentation details for the sozu strategy.

The sozu strategy attempts to execute the batch function everytime the keys array
hits capacity. Then subsequent calls to Load(), after the batch function has been
called once, start a new worker which will call the batch function once again after
the keys array capacity has been hit. It's goal is to ensure that they batch
function is called with the most number of keys possible.
*/
package sozu

import (
	"context"
	"sync"
	"time"

	"github.com/andy9775/dataloader"
	"github.com/andy9775/dataloader/strategies"
)

// Options contains the strategy configuration
type Options struct {
	Timeout time.Duration
}

// go routine status values
// Ensure that only one worker go routine is working to call the batch function
const (
	notRunning = 0 // go routine default start value
	running    = 1 // go routine is waiting for keys array to fill up
	ran        = 2 // go routine ran
)

// NewSozuStrategy returns a new instance of the Sozu strategy.
// The Sozu strategy calls the batch function everytime the keys array hits capacity.
// The Sozu Strategy also implements a timer that triggers the batch load function after
// a specified timeout duration (see Options) to ensure it doesn't block for too long.
// It attempts to ensure that each call to the batch function includes an array of keys
// whose length is >= 1 and <= the capacity.
func NewSozuStrategy(batch dataloader.BatchFunction, opts Options) func(int) dataloader.Strategy {
	return func(capacity int) dataloader.Strategy {
		formatOptions(&opts)

		// TODO: requests block on adding keys to key channel if channel capacity is less than 5
		keyChanCapacity := capacity
		if capacity < 5 {
			keyChanCapacity = 5
		}

		return &sozuStrategy{
			batchFunc: batch,
			counter:   strategies.NewCounter(capacity),

			workerMutex:     &sync.Mutex{},
			keyChan:         make(chan workerMessage, keyChanCapacity),
			goroutineStatus: notRunning,
			options:         opts,

			keys: dataloader.NewKeys(capacity),
		}
	}
}

type sozuStrategy struct {
	counter strategies.Counter
	// Track the keys to pass to the batch function. Once len(keys) == cap(keys),
	// the batch loading function is called with the keys to resolve.
	keys      dataloader.Keys
	results   *dataloader.ResultMap
	batchFunc dataloader.BatchFunction

	workerMutex *sync.Mutex

	keyChan   chan workerMessage
	closeChan chan struct{}

	goroutineStatus int

	options Options
}

type workerMessage struct {
	k          []dataloader.Key
	resultChan chan dataloader.ResultMap
}

// Load returns the Result for the specified Key.
// Internally Load adds a the key to the Keys array and blocks the caller until the batch loader
// function resolves. Once resolved, Load returns the data to the caller for the specified key.
// Subsequent calls to the Load function will continue to block until the keys array hits capacity,
// or the timeout value is hit.
func (s *sozuStrategy) Load(ctx context.Context, key dataloader.Key) dataloader.Result {
	/*
	 if a result doesn't exist or is not missing, start a new worker (if none is running)
	 and pass it the key to be resolved by the batch function.
	*/
	s.startWorker(ctx)

	resultChan := make(chan dataloader.ResultMap)
	message := workerMessage{k: []dataloader.Key{key}, resultChan: resultChan}
	s.keyChan <- message // pass key to the worker go routine

	/*
		TODO: clean up
		If a worker go routine is in the process of calling the batch function and another
		caller (to Load()) adds it's key to the key channel, the worker has no chance to
		pick it up off of the channel since it exists after processing the batch function.

		The following loop waits for either the result to be returned to the caller, or if the
		close channel has been triggered without calling the results channel, it means the worker
		go routine won't have a chance to read the callers key and process it. Therefore, if a close
		message comes through before the result does, start a new worker. (the new worker
		has a chance to read the buffered key for the caller).

		This solution isn't clean, or totally efficient but ensures that a worker will pick up the key
		and process it.
	*/
	for {
		select {
		case <-s.closeChan:
			/*
				Current worker closed, therefore no readers reading off of the key chan to get
				the callers buffered key.
				Start a new worker go routine which will read the existing key off of the key chan.
			*/
			s.startWorker(ctx)
		case result := <-resultChan:
			r := (result).GetValue(key)
			return r
		}

	}

}

func (s *sozuStrategy) LoadMany(ctx context.Context, keyArr ...dataloader.Key) dataloader.ResultMap {
	s.startWorker(ctx)

	resultChan := make(chan dataloader.ResultMap)
	message := workerMessage{k: keyArr, resultChan: resultChan}
	s.keyChan <- message

	// See comments in Load method above
	for {
		select {
		case <-s.closeChan:
			s.startWorker(ctx)
		case r := <-resultChan:
			result := dataloader.NewResultMap(len(keyArr))

			for _, k := range keyArr {
				result.Set(k.String(), r.GetValue(k))
			}

			return result
		}
	}
}

// ============================================== private =============================================
func (s *sozuStrategy) startWorker(ctx context.Context) {
	s.workerMutex.Lock() // ensure only one worker is started
	defer s.workerMutex.Unlock()

	if s.goroutineStatus == notRunning || s.goroutineStatus == ran {
		s.goroutineStatus = running
		s.closeChan = make(chan struct{})

		go func(ctx context.Context) {
			subscribers := make([]chan dataloader.ResultMap, 0, s.keys.Capacity())

			defer func() {
				s.goroutineStatus = ran
				s.keys.ClearAll()
				s.counter.ResetCount()
				close(s.closeChan)
			}()

			var r *dataloader.ResultMap
			for r == nil {
				select {
				case key := <-s.keyChan:
					subscribers = append(subscribers, key.resultChan)
					s.keys.Append(key.k...)
					if s.counter.Increment() { // hit capacity
						r = s.batchFunc(ctx, s.keys)
					}
				case <-time.After(s.options.Timeout):
					r = s.batchFunc(ctx, s.keys)
				}
			}

			for _, ch := range subscribers {
				ch <- *r
				close(ch)
			}
		}(ctx)
	}
}

// ============================================== helpers =============================================

// formatOptions configures default values for the loader options
func formatOptions(opts *Options) {
	opts.Timeout |= 6 * time.Millisecond
}
