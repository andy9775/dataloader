package standard

import (
	"context"
	"sync"
	"time"

	"github.com/andy9775/dataloader/strategies"

	"github.com/andy9775/dataloader"

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

// NewStandardStrategy returns a new instance of the standard strategy.
// The Standard Strategy, calls the batch function once when the keys array reaches
// capacity then subsequent calls to `Load()` will call the batch function with
// the individual keys.
func NewStandardStrategy(opts ...Option) dataloader.StrategyFunction {
	return func(capacity int, batch dataloader.BatchFunction) dataloader.Strategy {
		// default options
		o := options{}
		formatOptions(&o)

		// format options
		for _, apply := range opts {
			apply(&o)
		}

		return &standardStrategy{
			batchFunc: batch,
			counter:   strategies.NewCounter(capacity),

			workerMutex:     &sync.Mutex{},
			goroutineStatus: notRunning,

			keyChan:   make(chan workerMessage, capacity),
			closeChan: make(chan struct{}),
			options:   o,

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

// WithLogger configures the logger for the strategy. Default is a no op logger.
func WithLogger(l log.Logger) Option {
	return func(o *options) {
		o.logger = l
	}
}

// ===========================================================================================================

type standardStrategy struct {
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

// Load returns a Thunk function for the specified Key.
// Internally Load adds the Key to the Keys array and returns a (blocking) Thunk function which
// when called returns a value for the provided key.
func (s *standardStrategy) Load(ctx context.Context, key dataloader.Key) dataloader.Thunk {
	s.startWorker(ctx)

	resultChan := make(chan dataloader.ResultMap, 1) // buffered channel won't block in results loop
	message := workerMessage{k: []dataloader.Key{key}, resultChan: resultChan}
	s.keyChan <- message // pass key to the worker go routine (buffered channel)

	var result dataloader.Result
	var ok bool

	return func() (dataloader.Result, bool) {
		if result.Result != nil || result.Err != nil {
			return result, ok
		}

		/*
			Dual select statements allow prioritization of cases in situations where both channels have data.
			In instances where the first select goes to the default case (no message), but before going to the
			second select data is placed the resultChan and the closeChan is closed (data on both) the second
			select block will either resolve the first or second case (50/50). Although highly unlikely, in this
			case the batch function will be executed (50% of the time) in the go routine even though there is
			data available for the keys
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
			result, ok = (*s.batchFunc(ctx, dataloader.NewKeysWith(key))).GetValue(key)
			return result, ok
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

	var resultMap dataloader.ResultMap

	return func() dataloader.ResultMap {
		if resultMap != nil {
			return resultMap
		}

		/*
			See comments in Load method RE: dual select statements
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
		case <-s.closeChan: // batch the keys if closed
			r := *s.batchFunc(ctx, dataloader.NewKeysWith(keyArr...))
			resultMap = buildResultMap(keyArr, r)
			return resultMap
		}
	}
}

// LoadNoOp passes a nil value to the strategy worker and doesn't block the caller.
// Internally it increments the load counter ensuring the batch function is called on time.
func (s *standardStrategy) LoadNoOp(ctx context.Context) {
	s.startWorker(ctx) // start the worker in case the first caller is a cache success

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

			// loop while adding keys or timeout
			var r *dataloader.ResultMap
			for r == nil {
				select {
				case <-ctx.Done():
					s.options.logger.Logf("worker cancelled")
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
