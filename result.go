package dataloader

// Result is an alias for the resolved data by the batch loader
type Result interface{}

// ResultMap maps each loaded elements Result against the elements unique identifier (Key)
type ResultMap interface {
	Set(Identifier, Result)
	GetValue(Key) Result
	isEmpty() bool
}

type resultMap struct {
	r map[Identifier]Result
}

// MissingValue is set for any ResultMap value that cannot be found in the data store.
const MissingValue = iota

// NewResultMap returns a new instance of the result map for the provided keys.
// Each value defaults to `MissingValue`
func NewResultMap(keys Keys) ResultMap {
	r := make(map[Identifier]Result, len(keys))
	for _, k := range keys {
		r[k.String()] = MissingValue
	}

	return &resultMap{r}
}

// Set adds the value to the to the result set.
func (r *resultMap) Set(identifier Identifier, value Result) {
	r.r[identifier] = value
}

// GetValue returns the value from the results for the provided key.
// If no value exists, returns nil
func (r *resultMap) GetValue(key Key) Result {
	// No need to check ok, missing value from map[Any]interface{} is nil by default.
	return r.r[Identifier(key.String())]
}

func (r *resultMap) isEmpty() bool {
	return len(r.r) == 0
}
