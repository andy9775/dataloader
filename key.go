package dataloader

// Key is interface each element identifier must implement
type Key interface {
	// String should return a guaranteed unique string that can be used to identify
	// the element. For example in an array. Overlapping results from calls to this
	// method will result in incorrect data being returned.
	String() Identifier
}

// Identifier is an alias for a string which uniquely identifies each element to be
// resolved
type Identifier string

// Keys wraps an array of keys and contains accessor methods
type Keys []Key

// Keys returns a slice of the unique key values by calling the `String()`
// functions on each element
func (keys Keys) Keys() []Identifier {
	result := make([]Identifier, 0, len(keys))
	for _, k := range keys {
		result = append(result, k.String())
	}
	return result
}
