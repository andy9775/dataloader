package standard

import (
	"context"
	"sync"
	"time"

	"github.com/andy9775/dataloader/strategies"

	"github.com/andy9775/dataloader"
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

// NewStandardStrategy returns a new instance of the standard strategy.
// The Standard Strategy, calls the batch function once when the keys array reaches
// capacity then subsequent calls to `Load()` will call the batch function with
// the individual keys.
func NewStandardStrategy(batch dataloader.BatchFunction, opts Options) func(int) dataloader.Strategy {
	return func(capacity int) dataloader.Strategy {
		formatOptions(&opts)

		return &standardStrategy{
			batchFunc: batch,
			counter:   strategies.NewCounter(capacity),

			workerMutex:     &sync.Mutex{},
			keyChan:         make(chan workerMessage, capacity),
			closeChan:       make(chan struct{}),
			goroutineStatus: notRunning,
			options:         opts,

			keys: dataloader.NewKeys(capacity),
		}
	}
}

type standardStrategy struct {
	counter strategies.Counter
	// Track the keys to pass to the batch function. Once len(keys) == cap(keys),
	// the batch loading function is called with the keys to resolve.
	keys      dataloader.Keys
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

// Load returns a Thunk function for the specified Key.
// Internally Load adds the Key to the Keys array and returns a (blocking) Thunk function which
// when called returns a value for the provided key.
func (s *standardStrategy) Load(ctx context.Context, key dataloader.Key) dataloader.Thunk {
	s.startWorker(ctx)

	resultChan := make(chan dataloader.ResultMap, 1) // buffered channel won't block in results loop
	message := workerMessage{k: []dataloader.Key{key}, resultChan: resultChan}
	s.keyChan <- message // pass key to the worker go routine (buffered channel)

	return func() dataloader.Result {
		select {
		case <-s.closeChan:
			return (*s.batchFunc(ctx, dataloader.NewKeysWith(key))).GetValue(key)
		case result := <-resultChan:
			return result.GetValue(key)
		}
	}
}

// LoadMany returns a ThunkMany function for the provdied key.
// Internally, LoadMany adds the keyArr to the keys array and returns a (blocking) ThunkMany function
// which when called returns values for the provided keys.
func (s *standardStrategy) LoadMany(ctx context.Context, keyArr ...dataloader.Key) dataloader.ThunkMany {
	s.startWorker(ctx)

	resultChan := make(chan dataloader.ResultMap, 1) // buffered channel won't block in results loop
	message := workerMessage{k: keyArr, resultChan: resultChan}
	s.keyChan <- message

	return func() dataloader.ResultMap {
		var r dataloader.ResultMap
		select {
		case <-s.closeChan: // batch the keys if closed
			r = *s.batchFunc(ctx, dataloader.NewKeysWith(keyArr...))
			break
		case r = <-resultChan:
			break
		}

		return buildResultMap(keyArr, r)
	}
}

// LoadNoOp passes a nil value to the strategy worker and doesn't block the caller.
// Internally it increments the load counter ensuring the batch function is called on time.
func (s *standardStrategy) LoadNoOp() {
	// LoadNoOp passes a nil value to the strategy worker and doesn't block the caller.
	message := workerMessage{k: nil, resultChan: nil}
	s.keyChan <- message
}

// ============================================== private =============================================

// startWorker starts the background go routine if not already running for this strategy instance.
// The worker accepts keys via an internal channel and calls the batch function once full.
func (s *standardStrategy) startWorker(ctx context.Context) {
	s.workerMutex.Lock() // ensure only one worker is started
	defer s.workerMutex.Unlock()

	if s.goroutineStatus == notRunning {
		s.goroutineStatus = running

		go func(ctx context.Context) {
			subscribers := make([]chan dataloader.ResultMap, 0, s.keys.Capacity())

			defer func() {
				s.goroutineStatus = ran
				s.keys.ClearAll()
				s.counter.ResetCount()
				close(s.closeChan)
				s.closeChan = make(chan struct{})
			}()

			// loop while adding keys or timeout
			var r *dataloader.ResultMap
			for r == nil {
				select {
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

// buildResultMap filters through the provided result map and returns an ResultMap
// for the provided keys
func buildResultMap(keyArr []dataloader.Key, r dataloader.ResultMap) dataloader.ResultMap {
	results := dataloader.NewResultMap(len(keyArr))

	for _, k := range keyArr {
		results.Set(k.String(), r.GetValue(k))
	}

	return results
}
