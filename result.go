package dataloader

// Result is an alias for the resolved data by the batch loader
type Result struct {
	Result interface{}
	Err    error
}

// ResultMap maps each loaded elements Result against the elements unique identifier (Key)
type ResultMap map[Key]Result

// NewResultMap returns a new instance of the result map with the provided capacity.
// Each value defaults to nil
func NewResultMap(capacity int) ResultMap {
	r := make(map[Key]Result, capacity)
	return r
}

// ===================================== public methods =====================================

// Set adds the value to the to the result set.
func (r ResultMap) Set(identifier Key, value Result) {
	r[identifier] = value
}

// GetValue returns the value from the results for the provided key and true
// if the value was found, otherwise false.
func (r ResultMap) GetValue(key Key) (Result, bool) {
	if key == nil {
		return Result{}, false
	}

	result, ok := r[key]
	return result, ok
}

func (r ResultMap) GetValueForString(key StringKey) Result {
	// No need to check ok, missing value from map[Any]interface{} is nil by default.
	return r[key]
}

func (r ResultMap) Keys() []Key {
	res := make([]Key, 0, len(r))
	for k, _ := range r {
		res = append(res, k)
	}
	return res
}

func (r ResultMap) StringKeys() []string {
	res := make([]string, 0, len(r))
	for k, _ := range r {
		res = append(res, k.String())
	}
	return res
}

func (r ResultMap) Length() int {
	return len(r)
}
