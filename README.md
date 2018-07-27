# DataLoader

DataLoader implements a counter which can be used against Field Resolver
functions. It calls a **batch** function after the number of calls to Load values
reaches the loaders capacity.

## Terminology

The following terms are used throughout the documentation:

- **Element** - Refers to the item to be resolved. This can be a record from a
  database table or GraphQL type.

##### Implementation/Usage

Use the DataLoader when fetching a known number of elements, where each element has
a field resolver associated with it that hits a database or has some other time
consuming operation to resolve the data. This is typically useful when making
_index_ type queries where _n_ number of root elements is requested and each root
element has an associated model to be fetched.

For example, for a `users` table which contains:

- first_name
- last_name
- status_id (foreign key - status table)

and a `status` table which contains:

- id
- value (string)

Performing a query like:

```
{
  users(num: 10) {
    first_name
    status {
      value
    }
  }
}
```

where the `users` resolver returns an array of users (10 in this case). This
will typically result in 1 call to return all 10 users, and 10 calls to resolve
the `status` field for each user.

Use the DataLoader by setting its capacity to _n_ (10 in this case) and
providing a batch loader function which accepts the keys and should return
_n_ number of `status` records. The result of which is a single call to
the database to return the `status` elements after number of calls to `Load()`
has hit the set capacity.

Note that the capacity also acts as a _floor_. In instances where at least _n_
calls are known, all _n+1_ calls are executed depending on the
[strategy](#Strategies) used.

Internally, the DataLoader waits for the `Load()` function to be called _n_ times,
where _n_ is the initial DataLoader capacity. The `Load()` function blocks each
caller until the number of calls equal the loaders capacity and then each call
to `Load()` resolves to the requested element once the batch function
returns.

## API

#### DataLoader

> DataLoader is the basic interface for the library. It contains a provided
> strategy and cache strategy.

**`NewDataLoader(int, func(int) Strategy, Cache) DataLoader`**<br>
NewDataLoader returns a new instance of a dataloader tracking to the capacity
provided and using the provided strategy and cache strategy. The second argument
should return a strategy which accepts a capacity value

**`Load(context.Context, Key) Result`**<br>
Returns a single result for the provided key. Load blocks the caller until the
batch function is called and results are ready. Internally load passes the key
and context to the provided strategy.

**`LoadMany(context.Context, ...Key) ResultMap`**<br>
Returns a `ResultMap` which contains **only** the provided keys. LoadMany works
just like `Load()` and the number of keys passed to it **do not** impact when
the batch function is called.

#### Strategy

> Strategy is a interface to be used by implementors to hold and track data.

**`Load(context.Context, Key) Result`**<br>
Load should return the result for the specified key. Load should not reference
any cache.

**`LoadMany(context.Context, ...Key ResultMap`**<br>
LoadMany should return a `ResultMap` which contains **only** the values for the
provided keys. LoadMany should not implement any caching strategy internally.

#### Sozu Strategy

> The sozu strategy batches all calls to the batch function, including _n+1_
> calls

**`NewSozuStrategy(BatchFunction, Options) func(int) Strategy`**<br>
NewSozuStrategy returns a function which returns a new instance of a sozu
strategy for the provided capacity.

The Options values include:

- Timeout `time.Duration` - the time after which the batch function will be called if the
  capacity is not reached. `Default: 6 milliseconds`

#### Standard Strategy

> The standard strategy batches the first calls to the batch function, all
> subsequent callers call the batch function directly.

**`NewStandardStrategy(BatchFunction, Options) func(int) Strategy`**<br>
NewStandardStrategy returns a function which returns a new instance of the
standard strategy for the provided capacity.

The Options include:

- Timeout `time.Duration` - the time after which the batch function will be
  called if the capacity is not hit. `Default: 6 milliseconds`

#### ResultMap

> ResultMap acts as a wrapper around a basic `map[string]Result` but is
> thread-safe and provides accessor functions that tell the callers about the map
> or its state.
>
> When creating a new instance of the result map, an array of the
> Keys must be provided. The returned result map will assign `MissingValue` to
> each key. This allows the batch function and cache to easily identify keys for
> which no value could be found (none exists in the database).

**`NewResultMap([]Key) ResultMap`**<br>
NewResultMap returns a new instance of a result map where each key's value is

**`Set(string, Result)`**<br>
Set sets the result in the result map

**`GetValue(Key) Result`**<br>
GetValue returns the stored value for the provided key.

**`GetValueForString(String) Result`**<br>
GetValue returns the stored value for the provided string value.

**`Length() int`**<br>
Length returns the number of results it contains.

**`Keys() []string`**<br>
Keys returns the keys used to identify the data within this result map.

**`MergeWith(*ResultMap)`**<br>
MergeWith will join this result map with a provided result map.

#### Key

> Key is an interface each element's identifier must implement. Each Key must be
> unique (therefore a good candidate is the primary key for a database entry).

**`String() string`**<br>
String should return a string representation of the unique identifier. The
resulting string is used in the `ResultMap` to map the element to it's
identifier.

#### Keys

> Keys wraps an array of keys and provides a thread-safe way of tracking keys to
> be resolved by the batch function. It also provides methods to tell its state.

**`NewKeys(int) Keys`**<br>
NewKeys returns a new key store with it's length set to the capacity. If the
capacity is known to be exact provide the exact value. Otherwise adding a buffer
can be useful.

**`NewKeysWith(key ...Key) Keys`**<br>
NewKeysWith returns a Keys array with the provided keys.

**`Append(...Key)`**<br>
Append adds one or more keys to the internal array.

**`Capacity() int`**<br>
Capacity returns the set capacity for the key array.

**`Length() int`**<br>
Length returns the number of keys in the array.

**`ClearAll()`**<br>
ClearAll deletes all the stored keys from the key array.

**`Identifiers() []string`**<br>
Identifiers returns an array of strings which is a result of calling the
`String()` method on each element.

**`Keys() []Key`**<br>
Keys returns the raw keys stored by the key array.

**`IsEmpty() bool`**<br>
IsEmpty returns true if there are no keys in the keys array.

#### Cache

> Cache provides an interface for caching strategies. The library provides a
> no-op caching strategy which **does not** store any values.

**`NewNoOpCache() Cache`**<br>
NewNoOpCache returns an instance of a no operation cache. Internally it doesn't
store any values and all it's getter methods return nil.

**`SetResult(Key, Result)`**<br>
SetResult adds a value to the cache. The cache should store the value based on
it's implementation.

**`SetResultMap(ResultMap)`**<br>
SetResultMap saves all the elements in the provided ResultMap

**`GetResult(Key) Result`**<br>
GetResult should return the result matching the key or nil if none are found.

**`GetResultMap(...Key) ResultMap`**<br>
GetResultMap returns a result map which contains the values for only the
provided keys

#### Counter

> Counter provides an interface used to atomically count and track a value and
> to track if it's reached a certain set count. Internally it is used by
> strategies to track the number of calls to `Load` and `LoadMany`.

**`NewCounter(int) Counter`**<br>
NewCounter returns a new instance of a counter for the provided capacity. Once
the capacity is reached, the counter is considered complete.

**`Increment() bool`**<br>
Increment increases the counter by one and returns true if the counter has hit
it's capacity.

**`ResetCounter()`**<br>
ResetCounter sets the counter back to 0 but keeps the original capacity.

## Strategies

### Standard

The standard strategy initially calls the batch function when one of two
conditions are met:

1.  The number of calls the Load or LoadMany equals the capacity of the loader
2.  The timeout set has been reached (default to 6 milliseconds)

Subsequent calls to `Load()` or `LoadMany()` will execute the batch function for
each caller.

The standard strategy is useful for instance when _n+1_ calls are lower than the
capacity. For instance, it could be known that the app will need to resolve at
least 10 keys, but if there are more it is significantly less than 10 more (e.g.
2 more calls for a total of 12).

### Sozu

The sozu strategy initially calls the batch function when one of two conditions
are met:

1.  The number of calls the Load or LoadMany equals the capacity of the loader
2.  The timeout set has been reached (default to 6 milliseconds)

Subsequent calls to `Load()` or `LoadMany()` will call the batch function when
one of two conditions are met:

1.  The number of calls the Load or LoadMany equals the capacity of the loader
2.  The timeout set has been reached (default to 6 milliseconds)

The sozu strategy is useful for instance when known calls to load will be called
in a defined and equal group. For instance, the initial call can be to resolve
10 keys. Then it should be known that if there are more keys, that there will be
10 more keys resulting in the batch function being called twice, each time with
10 keys.

## TODO

- [x] Set a max duration that a call to `Load(Key)` can block. Start from the
      initial call to `Load(Key)`.
- [x] Caching approach/interface
- [ ] Tests!!
- [x] LoadMany - ability to call load with multiple keys
- [ ] Examples

## Future

- nested resolvers
  - A DataLoader should be provided for a specific field and it should cache the
    results. If a complex query is made (e.g. users have statuses, users have
    todos and todos have the same statuses as users) the loader should:
    - not execute another query if a query is in progress for a specific key,
      use that result instead.
    - batch load the rest of the queries (if the count is known at the time)
