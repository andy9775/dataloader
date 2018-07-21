package dataloader

import (
	"context"
)

// Strategy specifies the interface of loader strategies. A loader strategy specifies the process
// of calling the batch function and handling requests to fetch data.
type Strategy interface {
	// Load returns the Result for the specified Key.
	// Internally load adds the provided key to the keys array and blocks until a result
	// is returned.
	Load(context.Context, Key) Result

	// LoadMany returns a result map containing all the values for the keys the caller asked for
	LoadMany(context.Context, ...Key) ResultMap
}
