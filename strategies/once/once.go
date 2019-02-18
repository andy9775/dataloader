/*
Package once contains the implementation details for the once strategy.

The once strategy executes the batch function for every call to Thunk or ThunkMany.
It can be configured to call the batch function when Thunk or ThunkMany is called, or
the batch function can be called in a background go routine. Defaults to executing
per call to Thunk/ThunkMany.
*/
package once

import (
	"context"

	"github.com/go-log/log"

	"github.com/andy9775/dataloader"
)

// Options contains the strategy configuration
type options struct {
	inBackground bool
	logger       log.Logger
}

// Option accepts the dataloader and sets an option on it.
type Option func(*options)

// NewOnceStrategy returns a new instance of the once strategy.
// The Once strategy calls the batch function for each call to the Thunk if InBackground is false.
// Otherwise it runs the batch function in a background go routine and blocks calls to Thunk or
// ThunkMany if the result is not yet fetched.
func NewOnceStrategy(opts ...Option) dataloader.StrategyFunction {
	return func(_ int, batch dataloader.BatchFunction) dataloader.Strategy {
		o := options{}
		formatOptions(&o)

		// format options
		for _, apply := range opts {
			apply(&o)
		}

		return &onceStrategy{
			batchFunc: batch,
			options:   o,
		}
	}
}

type onceStrategy struct {
	batchFunc dataloader.BatchFunction

	options options
}

// ============================================== option setters =============================================

// WithInBackground configures the strategy to load in the background
func WithInBackground() Option {
	return func(o *options) {
		o.inBackground = true
	}
}

// WithLogger configures the logger for the strategy. Default is a no op logger.
func WithLogger(l log.Logger) Option {
	return func(o *options) {
		o.logger = l
	}
}

// ===========================================================================================================

// Load returns a Thunk which either calls the batch function when invoked or waits for a result from a
// background go routine (blocking if no data is available). Note that if the strategy is configured to
// run in the background, calling Load again will spin up another background go routine.
func (s *onceStrategy) Load(ctx context.Context, key dataloader.Key) dataloader.Thunk {

	type data struct {
		r  dataloader.Result
		ok bool
	}
	var result data

	if s.options.inBackground {
		resultChan := make(chan data)

		// don't check if result is nil before starting in case a new key is passed in
		go func() {
			r, ok := (*s.batchFunc(ctx, dataloader.NewKeysWith(key))).GetValue(key)
			resultChan <- data{r, ok}
		}()

		// call batch in background and block util it returns
		return func() (dataloader.Result, bool) {
			if result.r.Result != nil || result.r.Err != nil {
				return result.r, result.ok
			}

			result = <-resultChan
			return result.r, result.ok
		}
	}

	// call batch when thunk is called
	return func() (dataloader.Result, bool) {
		if result.ok {
			return result.r, result.ok
		}

		result.r, result.ok = (*s.batchFunc(ctx, dataloader.NewKeysWith(key))).GetValue(key)
		return result.r, result.ok
	}
}

// LoadMany returns a ThunkMany which either calls the batch function when invoked or waits for a result from
// a background go routine (blocking if no data is available). Note that calling load many again if configured
// to run in the background will cause the background worker to execute once more.
func (s *onceStrategy) LoadMany(ctx context.Context, keyArr ...dataloader.Key) dataloader.ThunkMany {
	var result dataloader.ResultMap

	if s.options.inBackground {
		resultChan := make(chan dataloader.ResultMap)

		// don't check if result is nil before starting in case a new key is passed in
		go func() {
			resultChan <- *s.batchFunc(ctx, dataloader.NewKeysWith(keyArr...))
		}()

		// call batch in background and block util it returnsS
		return func() dataloader.ResultMap {
			if result != nil {
				return result
			}

			select {
			case <-ctx.Done():
				s.options.logger.Log("worker cancelled")
				return dataloader.NewResultMap(0)
			case result = <-resultChan:
				return result
			}
		}
	}

	// call batch when thunk is called
	return func() dataloader.ResultMap {
		if result != nil {
			return result
		}

		result = *s.batchFunc(ctx, dataloader.NewKeysWith(keyArr...))
		return result
	}

}

// LoadNoOp has no internal implementation since the once strategy doesn't track the number of calls to
// Load or Loadmany
func (*onceStrategy) LoadNoOp(context.Context) {}

// ================================================= helpers =================================================

// formatOptions configures the default values for the loader
func formatOptions(opts *options) {
	opts.inBackground = false
	opts.logger = log.DefaultLogger
}
