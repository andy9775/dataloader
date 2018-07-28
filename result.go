package dataloader

import "sync"

// Result is an alias for the resolved data by the batch loader
type Result struct {
	Result interface{}
	Err    error
}

// ResultMap maps each loaded elements Result against the elements unique identifier (Key)
type ResultMap interface {
	Set(string, Result)
	GetValue(Key) Result
	Length() int
	// Keys returns a slice of all unique identifiers used in the containing map (keys)
	Keys() []string
	MergeWith(*ResultMap)
	GetValueForString(string) Result
}

type resultMap struct {
	r map[string]Result
	m *sync.RWMutex
}

// NewResultMap returns a new instance of the result map for the provided keys.
// Each value defaults to nil
func NewResultMap(keys []Key) ResultMap {
	r := make(map[string]Result, len(keys))

	return &resultMap{r: r, m: &sync.RWMutex{}}
}

// ===================================== public methods =====================================

// Set adds the value to the to the result set.
func (r *resultMap) Set(identifier string, value Result) {
	r.m.Lock()
	defer r.m.Unlock()

	r.r[identifier] = value
}

// GetValue returns the value from the results for the provided key.
// If no value exists, returns nil
func (r *resultMap) GetValue(key Key) Result {
	if key == nil {
		return Result{}
	}

	r.m.RLock()
	defer r.m.RUnlock()

	// No need to check ok, missing value from map[Any]interface{} is nil by default.
	return r.r[key.String()]
}

func (r *resultMap) GetValueForString(key string) Result {
	r.m.RLock()
	defer r.m.RUnlock()

	// No need to check ok, missing value from map[Any]interface{} is nil by default.
	return r.r[key]
}

func (r *resultMap) Keys() []string {
	r.m.RLock()
	defer r.m.RUnlock()

	res := make([]string, 0, len(r.r))
	for k := range r.r {
		res = append(res, k)
	}
	return res
}

func (r *resultMap) Length() int {
	r.m.RLock()
	defer r.m.RUnlock()

	return len(r.r)
}

func (r *resultMap) MergeWith(m *ResultMap) {
	r.m.Lock()
	defer r.m.Unlock()

	for _, k := range (*m).Keys() {
		r.r[k] = (*m).GetValueForString(k)
	}
}
