package standard

import (
	"context"
	"sync"
	"time"

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
			batchFunc:    batch,
			capacity:     capacity,
			loadCalls:    0,
			counterMutex: &sync.Mutex{},

			workerMutex:     &sync.Mutex{},
			keyChan:         make(chan dataloader.Key, keyChanCapacity),
			goroutineStatus: notRunning,
			options:         opts,

			keys: dataloader.NewKeys(capacity),
		}
	}
}

type standardStrategy struct {
	capacity     int
	loadCalls    int
	counterMutex *sync.Mutex
	// Track the keys to pass to the batch function. Once len(keys) == cap(keys),
	// the batch loading function is called with the keys to resolve.
	keys      dataloader.Keys
	results   *dataloader.ResultMap
	batchFunc dataloader.BatchFunction

	workerMutex *sync.Mutex

	keyChan   chan dataloader.Key
	closeChan chan struct{}

	goroutineStatus int

	options Options
}

// Load returns the Result for the specified Key.
// Internally Load adds Key to the Keys array and blocks the caller until the batch loader
// function resolves. Once resolved, Load returns the data to the caller for the specified
// Key
func (s *standardStrategy) Load(ctx context.Context, key dataloader.Key) dataloader.Result {
	s.startWorker(ctx)

	s.keyChan <- key // pass key to the worker go routine (buffered channel)

	<-s.closeChan // wait for the worker to complete and channel to close

	if r := s.getResult(key); r == nil {
		return (*s.batchFunc(ctx, dataloader.NewKeysWith(key))).GetValue(key)
	} else if r == dataloader.MissingValue {
		return nil
	} else {
		return r
	}
}

// ============================================== private =============================================

func (s *standardStrategy) startWorker(ctx context.Context) {
	s.workerMutex.Lock() // ensure only one worker is started
	defer s.workerMutex.Unlock()

	if s.goroutineStatus == notRunning {
		s.goroutineStatus = running
		s.closeChan = make(chan struct{})

		go func(ctx context.Context) {
			defer func() {
				s.goroutineStatus = ran
				s.keys.ClearAll()
				s.resetCount()
				close(s.closeChan)
			}()

			// loop while adding keys or timeout
			for s.results == nil {
				select {
				case key := <-s.keyChan:
					s.keys.Append(key)
					if s.increment() { // hit capacity
						s.results = s.batchFunc(ctx, s.keys)
					}
				case <-time.After(s.options.Timeout):
					s.results = s.batchFunc(ctx, s.keys)
				}

			}
		}(ctx)
	}
}

func (s *standardStrategy) getResult(key dataloader.Key) dataloader.Result {
	if s.results != nil {
		return (*s.results).GetValue(key)
	}
	return nil
}

func (s *standardStrategy) increment() bool {
	s.counterMutex.Lock()
	defer s.counterMutex.Unlock()

	s.loadCalls++
	return s.loadCalls == s.capacity
}

func (s *standardStrategy) resetCount() {
	s.counterMutex.Lock()
	defer s.counterMutex.Unlock()

	s.loadCalls = 0
}

// ============================================== helpers =============================================

// formatOptions configures default values for the loader options
func formatOptions(opts *Options) {
	opts.Timeout |= 6 * time.Millisecond
}
