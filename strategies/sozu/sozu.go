/*
Package sozu contains implementation details for the sozu strategy.

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

	"github.com/go-log/log"
)

// Options contains the strategy configuration
type options struct {
	timeout time.Duration
	logger  log.Logger
}

// Option accepts the dataloader and sets an option on it.
type Option func(*options)

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
func NewSozuStrategy(opts ...Option) dataloader.StrategyFunction {
	return func(capacity int, batch dataloader.BatchFunction) dataloader.Strategy {
		// default options
		o := options{}
		formatOptions(&o)

		// format options
		for _, apply := range opts {
			apply(&o)
		}

		return &sozuStrategy{
			batchFunc: batch,
			counter:   strategies.NewCounter(capacity),

			workerMutex:     &sync.Mutex{},
			goroutineStatus: notRunning,

			keyChan: make(chan workerMessage, capacity),
			options: o,

			keys: dataloader.NewKeys(capacity),
		}
	}
}

// ============================================== option setters =============================================

// WithTimeout sets the timeout value for the strategy
func WithTimeout(t time.Duration) Option {
	return func(o *options) {
		o.timeout = t
	}
}

// WithLogger adds a logger to the strategy. Default is a no op logger.
func WithLogger(l log.Logger) Option {
	return func(s *options) {
		s.logger = l
	}
}

// ===========================================================================================================

type sozuStrategy struct {
	counter strategies.Counter
	// Track the keys to pass to the batch function. Once len(keys) == cap(keys),
	// the batch loading function is called with the keys to resolve.
	keys      dataloader.Keys
	batchFunc dataloader.BatchFunction

	workerMutex     *sync.Mutex
	goroutineStatus int

	keyChan   chan workerMessage
	closeChan chan struct{}

	options options
}

type workerMessage struct {
	k          []dataloader.Key
	resultChan chan dataloader.ResultMap
}

// Load returns the Thunk for the specified Key.
// Internally Load adds a the key to the Keys array and returns a Thunk function which when
// called returns the result for the key. Subsequent calls to the load function will keep
// incrementing the load counter until the call count hits capacity which results in the batch
// function being called.
func (s *sozuStrategy) Load(ctx context.Context, key dataloader.Key) dataloader.Thunk {
	/*
	 if a result doesn't exist or is not missing, start a new worker (if none is running)
	 and pass it the key to be resolved by the batch function.
	*/
	s.startWorker(ctx)

	resultChan := make(chan dataloader.ResultMap, 1) // buffered channel won't block in results loop
	message := workerMessage{k: []dataloader.Key{key}, resultChan: resultChan}
	s.keyChan <- message // pass key to the worker go routine

	var result dataloader.Result
	var ok bool
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
	return func() (dataloader.Result, bool) {
		if result.Result != nil || result.Err != nil {
			return result, ok
		}

		for {
			/*
				Dual select statements allow prioritization of cases in situations where both channels have data.
				In instances where the first select goes to the default case (no message), but before going to the
				second select data is placed the resultChan and the closeChan is closed (data on both) the second
				select block will either resolve the first or second case (50/50). In this case it is acceptable as
				after starting the worker, it loops back around and reads off of the result chan (first select) and
				returns. Hence the caller isn't blocked waiting for the worker to execute, and the stale worker simply
				executes the batch function after the timeout duration and exists. While highly unlikely, checking if
				the keys array is empty in the batch function will eliminate the execution of any rogue sql queries or
				network requests.
			*/
			select {
			case r := <-resultChan:
				result, ok = r.GetValue(key)
				return result, ok
			default:
			}

			select {
			case <-ctx.Done():
				return dataloader.Result{Result: nil, Err: nil}, false
			case r := <-resultChan:
				result, ok = r.GetValue(key)
				return result, ok
			case <-s.closeChan:
				/*
					Current worker closed, therefore no readers reading off of the key chan to get
					the callers buffered key.
					Start a new worker go routine which will read the existing key off of the key chan.
				*/
				s.startWorker(ctx)
			}
		}
	}
}

// LoadMany returns the ThunkMany for the specified Keys.
// Internally LoadMany adds a the keys to the Keys array and returns a ThunkMany function which when
// called returns the result map for the keys. Subsequent calls to the LoadMany function will keep
// incrementing the load counter until the call count hits capacity which results in the batch
// function being called.
func (s *sozuStrategy) LoadMany(ctx context.Context, keyArr ...dataloader.Key) dataloader.ThunkMany {
	s.startWorker(ctx)

	resultChan := make(chan dataloader.ResultMap, 1) // buffered channel won't block in results loop
	message := workerMessage{k: keyArr, resultChan: resultChan}
	s.keyChan <- message

	var resultMap dataloader.ResultMap

	// See comments in Load method RE: for loop
	return func() dataloader.ResultMap {
		/*
			NOTE:
				The purpose of building a new ResultMap (buildResultMap) is to ensure that each caller to the same
			  strategy gets its own isolated data separate from the other callers. This allows each caller to
			  iterate through the keys and only get it's own data
		*/

		if resultMap != nil {
			return resultMap
		}

		for {
			/*
				see comments in the Load method RE: dual select statements
			*/
			select {
			case r := <-resultChan:
				resultMap = buildResultMap(keyArr, r)
				return resultMap
			default:
			}

			select {
			case <-ctx.Done():
				return dataloader.NewResultMap(0)
			case r := <-resultChan:
				resultMap = buildResultMap(keyArr, r)
				return resultMap
			case <-s.closeChan:
				s.startWorker(ctx)
			}
		}
	}
}

// LoadNoOp passes a nil value to the strategy worker and doesn't block the caller.
// Internally it increments the load counter ensuring the batch function is called on time.
func (s *sozuStrategy) LoadNoOp(ctx context.Context) {
	s.startWorker(ctx) // start the worker in case the first caller is a cache success

	// LoadNoOp passes a nil value to the strategy worker and doesn't block the caller.
	message := workerMessage{k: nil, resultChan: nil}
	s.keyChan <- message
}

// ============================================== private =============================================

// startWorker starts the background go routine if not already running for this strategy instance.
// The worker accepts keys via an internal channel and calls the batch function once full.
func (s *sozuStrategy) startWorker(ctx context.Context) {
	s.workerMutex.Lock() // ensure only one worker is started
	defer s.workerMutex.Unlock()

	if s.goroutineStatus == notRunning || s.goroutineStatus == ran {
		s.goroutineStatus = running
		s.closeChan = make(chan struct{})

		go func(ctx context.Context) {
			subscribers := make([]chan dataloader.ResultMap, 0, s.keys.Capacity())
			s.options.logger.Logf("starting new worker with capacity: %d", s.keys.Capacity())

			defer func() {
				s.workerMutex.Lock()
				defer s.workerMutex.Unlock()

				s.goroutineStatus = ran
				s.keys.ClearAll()
				s.counter.ResetCount()
				close(s.closeChan)
			}()

			var r *dataloader.ResultMap
			for r == nil {
				select {
				case <-ctx.Done():
					s.options.logger.Log("worker cancelled")
					return
				case key := <-s.keyChan:
					// if LoadNoOp passes a value through the chan, ignore the data and increment the counter
					if key.resultChan != nil {
						subscribers = append(subscribers, key.resultChan)
					}
					if key.k != nil {
						s.keys.Append(key.k...)
					}

					if s.counter.Increment() { // hit capacity
						r = s.batchFunc(ctx, s.keys)
					}
				case <-time.After(s.options.timeout):
					s.options.logger.Logf("worker timing out with %d keys", s.keys.Length())
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
func formatOptions(opts *options) {
	opts.timeout = 16 * time.Millisecond
	opts.logger = log.DefaultLogger
}

// buildResultMap filters through the provided result map and returns an ResultMap
// for the provided keys
func buildResultMap(keyArr []dataloader.Key, r dataloader.ResultMap) dataloader.ResultMap {
	results := dataloader.NewResultMap(len(keyArr))

	for _, k := range keyArr {
		if val, ok := r.GetValue(k); ok {
			results.Set(k.String(), val)
		}
	}

	return results
}
