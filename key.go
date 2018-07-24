package dataloader

import (
	"sync"
)

// Key is an interface each element identifier must implement in order to be stored and cached
// in the ResultsMap
type Key interface {
	// String should return a guaranteed unique string that can be used to identify
	// the element. It's purpose is to identify each record when storing the results.
	// Records which should be different but share the same key will be overwritten.
	String() string
}

// Keys wraps an array of keys and contains accessor methods
type Keys interface {
	Append(...Key)
	Capacity() int
	Length() int
	ClearAll()
	UniqueIdentifiers() []string
	Identifiers() []string
	Keys() []Key
	IsEmpty() bool
}

type keys struct {
	k []Key
	m *sync.RWMutex
}

// NewKeys returns a new instance of the Keys array with the provided capacity.
func NewKeys(capacity int) Keys {
	return &keys{
		k: make([]Key, 0, capacity),
		m: &sync.RWMutex{},
	}
}

// NewKeysWith is a helper method for returning a new keys array which includes the
// the provided keys
func NewKeysWith(key ...Key) Keys {
	return &keys{
		k: key,
		m: &sync.RWMutex{},
	}
}

// ================================== public methods ==================================

func (k *keys) Append(keys ...Key) {
	k.m.Lock()
	defer k.m.Unlock()

	for _, key := range keys {
		k.k = append(k.k, key)
	}
}

func (k *keys) Capacity() int {
	return cap(k.k)
}

func (k *keys) Length() int {
	k.m.RLock()
	defer k.m.RUnlock()

	return len(k.k)
}

func (k *keys) ClearAll() {
	k.m.Lock()
	defer k.m.Unlock()

	k.k = make([]Key, 0, len(k.k))
}

// Keys returns a slice of the unique key values by calling the `String()`
// functions on each element
func (k *keys) Keys() []Key {
	k.m.RLock()
	defer k.m.RUnlock()

	result := make([]Key, k.Length())
	copy(result, k.k)

	return result
}

// UniqueKeys returns an array of unique identifier values
func (k *keys) UniqueIdentifiers() []string {
	k.m.RLock()
	defer k.m.RUnlock()

	result := make([]string, 0, k.Length())
	temp := make(map[Key]bool, k.Length())

	for _, val := range k.k {
		if _, ok := temp[val]; !ok {
			temp[val] = true
			result = append(result, val.String())
		}
	}

	return result
}

func (k *keys) Identifiers() []string {
	k.m.RLock()
	defer k.m.RUnlock()

	result := make([]string, 0, len(k.k))
	for _, k := range k.k {
		result = append(result, k.String())
	}

	return result
}

func (k *keys) IsEmpty() bool {
	k.m.RLock()
	defer k.m.RUnlock()

	return len(k.k) == 0
}
