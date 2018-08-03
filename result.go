package dataloader

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
	GetValueForString(string) Result
}

type resultMap struct {
	r map[string]Result
}

// NewResultMap returns a new instance of the result map with the provided capacity.
// Each value defaults to nil
func NewResultMap(capacity int) ResultMap {
	r := make(map[string]Result, capacity)

	return &resultMap{r: r}
}

// ===================================== public methods =====================================

// Set adds the value to the to the result set.
func (r *resultMap) Set(identifier string, value Result) {
	r.r[identifier] = value
}

// GetValue returns the value from the results for the provided key.
// If no value exists, returns nil
func (r *resultMap) GetValue(key Key) Result {
	if key == nil {
		return Result{}
	}

	// No need to check ok, missing value from map[Any]interface{} is nil by default.
	return r.r[key.String()]
}

func (r *resultMap) GetValueForString(key string) Result {
	// No need to check ok, missing value from map[Any]interface{} is nil by default.
	return r.r[key]
}

func (r *resultMap) Keys() []string {
	res := make([]string, 0, len(r.r))
	for k := range r.r {
		res = append(res, k)
	}
	return res
}

func (r *resultMap) Length() int {
	return len(r.r)
}
