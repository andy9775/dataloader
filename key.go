package dataloader

// Key is an interface each element identifier must implement in order to be stored and cached
// in the ResultsMap
type Key interface {
	// String should return a guaranteed unique string that can be used to identify
	// the element. It's purpose is to identify each record when storing the results.
	// Records which should be different but share the same key will be overwritten.
	String() string

	// Raw should return real value of the key.
	Raw() interface{}
}

// Keys wraps an array of keys and contains accessor methods
type Keys interface {
	Append(...Key)
	Capacity() int
	Length() int
	ClearAll()
	// Keys returns a an array of unique results after calling Raw on each key
	Keys() []interface{}
	IsEmpty() bool
}

type keys struct {
	k []Key
}

// NewKeys returns a new instance of the Keys array with the provided capacity.
func NewKeys(capacity int) Keys {
	return &keys{
		k: make([]Key, 0, capacity),
	}
}

// NewKeysWith is a helper method for returning a new keys array which includes the
// the provided keys
func NewKeysWith(key ...Key) Keys {
	return &keys{
		k: key,
	}
}

// ================================== public methods ==================================

func (k *keys) Append(keys ...Key) {
	for _, key := range keys {
		if key != nil && key.Raw() != nil { // don't track nil keys
			k.k = append(k.k, key)
		}
	}
}

func (k *keys) Capacity() int {
	return cap(k.k)
}

func (k *keys) Length() int {
	return len(k.k)
}

func (k *keys) ClearAll() {
	k.k = make([]Key, 0, len(k.k))
}

func (k *keys) Keys() []interface{} {
	result := make([]interface{}, 0, k.Length())
	temp := make(map[Key]bool, k.Length())

	for _, val := range k.k {
		if _, ok := temp[val]; !ok {
			temp[val] = true
			result = append(result, val.Raw())
		}
	}

	return result
}

func (k *keys) IsEmpty() bool {
	return len(k.k) == 0
}
