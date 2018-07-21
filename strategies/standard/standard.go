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

		// TODO: requests block on adding keys to key channel if channel capacity is less than 5
		keyChanCapacity := capacity
		if capacity < 5 {
			keyChanCapacity = 5
		}

		return &standardStrategy{
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

// Load returns the Result for the specified Key.
// Internally Load adds Key to the Keys array and blocks the caller until the batch loader
// function resolves. Once resolved, Load returns the data to the caller for the specified
// Key
func (s *standardStrategy) Load(ctx context.Context, key dataloader.Key) dataloader.Result {
	s.startWorker(ctx)

	resultChan := make(chan dataloader.ResultMap)
	message := workerMessage{k: []dataloader.Key{key}, resultChan: resultChan}
	s.keyChan <- message // pass key to the worker go routine (buffered channel)

	select {
	case <-s.closeChan:
		return (*s.batchFunc(ctx, dataloader.NewKeysWith(key))).GetValue(key)
	case result := <-resultChan:
		r := result.GetValue(key)
		if r == dataloader.MissingValue {
			return nil
		}
		return r
	}
}

func (s *standardStrategy) LoadMany(ctx context.Context, keyArr ...dataloader.Key) dataloader.ResultMap {
	s.startWorker(ctx)

	resultChan := make(chan dataloader.ResultMap)
	message := workerMessage{k: keyArr, resultChan: resultChan}
	s.keyChan <- message

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

// ============================================== private =============================================

func (s *standardStrategy) startWorker(ctx context.Context) {
	s.workerMutex.Lock() // ensure only one worker is started
	defer s.workerMutex.Unlock()

	if s.goroutineStatus == notRunning {
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

			// loop while adding keys or timeout
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

// buildResultMap filters through the provided result map and returns an ResultMap
// for the provided keys
func buildResultMap(keyArr []dataloader.Key, r dataloader.ResultMap) dataloader.ResultMap {
	results := dataloader.NewResultMap(keyArr)

	for _, k := range keyArr {
		val := r.GetValue(k)
		if val == dataloader.MissingValue {
			results.Set(k.String(), nil)
		} else {
			results.Set(k.String(), val)
		}
	}

	return results
}
