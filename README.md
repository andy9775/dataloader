# DataLoader

[![Build Status](https://travis-ci.org/andy9775/dataloader.svg?branch=master)](https://travis-ci.org/andy9775/dataloader)
[![Coverage Status](https://coveralls.io/repos/github/andy9775/dataloader/badge.svg?branch=master)](https://coveralls.io/github/andy9775/dataloader?branch=master)
[![Go Report Card](https://goreportcard.com/badge/github.com/andy9775/dataloader)](https://goreportcard.com/report/github.com/andy9775/dataloader)

DataLoader implements a counter which can be used against Field Resolver
functions. It calls a **batch** function after the number of calls to Load values
reaches the loaders capacity.

##### Terminology

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

Note that the capacity also acts as a _floor_. In instances where _at least_ _n_
calls are known, all _n+1_ calls are executed depending on the
[strategy](#Strategies) used.

Internally, the DataLoader waits for the `Load()` function to be called _n_ times,
where _n_ is the initial DataLoader capacity. The `Load()` function returns a
Thunk, or ThunkMany function, which block when called until the number of calls
to Load equal the loaders capacity. Each call to the returned Thunk function
then returns the values for the keys it is attached to.

## API

#### DataLoader

> DataLoader is the basic interface for the library. It contains a provided
> strategy and cache strategy.

**`NewDataLoader(int, BatchFunction, func(int, BatchFunction) Strategy, Cache, Tracer) DataLoader`**<br>
NewDataLoader returns a new instance of a DataLoader tracking to the capacity
provided and using the provided execution and cache strategy. The second argument
should return a strategy which accepts a capacity value and the BatchFunction

**`Load(context.Context, Key) Thunk`**<br>
Returns a Thunk for the specified keys. Internally Load adds the
provided keys to the keys array and returns a callback function which when
called returns the values for the provided keys. Load does not block callers.

**`LoadMany(context.Context, ...Key) ThunkMany`**<br>
Returns a ThunkMany for the specified keys. Internally LoadMany adds the
provided keys to the keys array and returns a callback function which when
called returns the values for the provided keys. LoadMany does not block callers.

#### Strategy

> Strategy is a interface to be used by implementors to hold and track data.

**`Load(context.Context, Key) Thunk`**<br>
Load should return the Thunk function linked to the provided key. Load should
not reference a cache nor should it block.

**`LoadMany(context.Context, ...Key) ThunkMany`**<br>
LoadMany should return a ThunkMany function linked to the provided keys.
LoadMany should not reference a cache nor should it block.

**`LoadNoOp(context.Context) ResultMap`**<br>
LoadNoOp should not block the caller nor return values to the caller. It is
called when a value is retrieved from the cache and it's responsibility is to
increment the internal loads counter.

#### Sozu Strategy

> The sozu strategy batches all calls to the batch function, including _n+1_
> calls. Since the strategy returns a Thunk or ThunkMany, calling Load or
> LoadMany before performing another long running process will allow the batch
> function to run concurrently to any other operations.

**`NewSozuStrategy(Options) func(int, BatchFunction) Strategy`**<br>
NewSozuStrategy returns a function which returns a new instance of a sozu
strategy for the provided capacity.

The Options values include:

- Timeout `time.Duration` - the time after which the batch function will be called if the
  capacity is not reached. `Default: 6 milliseconds`

#### Standard Strategy

> The standard strategy batches the first calls to the batch function, all
> subsequent callers call the batch function directly. Since the strategy
> returns a Thunk or ThunkMany, calling Load or LoadMany before performing
> another long running process will allow the batch function to run concurrently
> to any other operations.

**`NewStandardStrategy(Options) func(int, BatchFunction) Strategy`**<br>
NewStandardStrategy returns a function which returns a new instance of the
standard strategy for the provided capacity.

The Options include:

- Timeout `time.Duration` - the time after which the batch function will be
  called if the capacity is not hit. `Default: 6 milliseconds`

#### Once Strategy

> The once strategy does not track calls to load and is useful for single calls
> to the batch function. Passing `InBackground: true` to the options allows the
> batch function to be called in the background, otherwise the batch function is
> called when Thunk, or ThunkMany, is called. It is useful for keeping a
> consistent API across an application and for automatically performing data
> queries in a background go routine.

**`NewOnceStrategy(Options) func(int, BatchFunction) Strategy`**<br>
NewOnceStrategy returns a functions which returns an instance of the once
strategy ignoring the provided capacity value.

The Options include:

- InBackground `bool` - if true the batch function will be invoked in a
  background go routine as soon as Load or LoadMany is called. If the batch
  function executes in the background before calling Thunk/ThunkMany, calls
  will not block and return data. Else calls to Thunk/ThunkMany will block until
  the batch function executes.

#### ResultMap

> ResultMap acts as a wrapper around a basic `map[string]Result` and provides
> accessor functions that tell the callers about the map or its state. ResultMap
> is not go routine safe.
>
> When creating a new instance of the result map, a capacity must be provided.

**`NewResultMap(int) ResultMap`**<br>
NewResultMap returns a new instance of a result map with the set capacity

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

#### Key

> Key is an interface each element's identifier must implement. Each Key must be
> unique (therefore a good candidate is the primary key for a database entry).

**`String() string`**<br>
String should return a string representation of the unique identifier. The
resulting string is used in the `ResultMap` to map the element to it's
identifier.

**`Raw() interface{}`**<br>
Raw should return the underlying value of the key. Examples are: `int`, `string`.

#### Keys

> Keys wraps an array of keys and provides a way of tracking keys to
> be resolved by the batch function. It also provides methods to tell its state.
> Keys is not go routine safe.

**`NewKeys(int) Keys`**<br>
NewKeys returns a new key store with it's length set to the capacity. If the
capacity is known to be exact provide the exact value. Otherwise adding a buffer
can be useful to prevent unnecessary memory growth.

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

**`Keys() []interface{}`**<br>
Keys returns a unique array of interface{} types for each key after calling each
keys `Raw()` method

**`IsEmpty() bool`**<br>
IsEmpty returns true if there are no keys in the keys array.

#### Cache

> Cache provides an interface for caching strategies. The library provides a
> no-op caching strategy which **does not** store any values.

**`NewNoOpCache() Cache`**<br>
NewNoOpCache returns an instance of a no operation cache. Internally it doesn't
store any values and all it's getter methods return nil or false.

**`SetResult(context.Context, Key, Result)`**<br>
SetResult adds a value to the cache. The cache should store the value based on
it's implementation.

**`SetResultMap(context.Context, ResultMap)`**<br>
SetResultMap saves all the elements in the provided ResultMap

**`GetResult(context.Context, Key) Result`**<br>
GetResult should return the result matching the key or nil if none are found.

**`GetResultMap(context.Context, ...Key) ResultMap`**<br>
GetResultMap returns a result map which contains the values for only the
provided keys.

**`Delete(context.Context, Key) bool`**<br>
Delete removes the value for the provided key and returns true if successful.

**`ClearAll(context.Context) bool`**<br>
ClearAll removes all values from the cache and returns true if successfully
cleared

#### Tracer

> Tracer provides an interface used by the DataLoader for tracing requests
> through the application. Tracing occurs on calls to `Load`, `LoadMany` and
> when the `BatchFunction` gets called.

**`NewNoOpTracer() Tracer`**<br>
NewNoOpTracer returns an instance of a blank tracer that does not output anything.

**`NewOpenTracingTracer() Tracer`**<br>
NewOpenTracingTracer returns an instance of a tracer which conforms to the open
tracing standard.

**`Load(context.Context, Key) (context.Context, LoadFinishFunc)`**<br>
Load performs tracing around calls to the `Load` function. It returns a context
with tracing information and a finish function which ends the tracing.

**`LoadMany(context.Context, Key) (context.Context, LoadManyFinishFunc)`**<br>
LoadMany performs tracing around calls to the `LoadMany` function. It returns a
context with tracing information and a finish function which ends the tracing.

**`Batch(context.Context) (context.Context, BatchFinishFunc)`**<br>
Batch performs tracing around calls to the `Batch` function. It returns a context
with the tracing information attached to it and a finish function which ends the
tracing.

**`LoadFinishFunc(Result)`**<br>
LoadFinishFunc ends tracing started by `Load` and gets passed the resolved
result for the queried key.

**`LoadManyFinishFunc(ResultMap)`**<br>
LoadFinishFunc ends tracing started by `LoadMany` and gets passed the resolved
result map for the queried keys.

**`BatchFinishFunc(ResultMap)`**<br>
BatchFinishFunc ends tracing started by `Batch` and gets passed the resolved
result map for the key or keys.

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

Both the `Standard` and `Sozu` strategies allow for concurrent operations before
executing the returned Thunk function and both ensure that other callers are not
blocked when waiting for data to be read. This allows certain resolvers that use
a shared strategy to block some of the time while allowing other resolvers to
receive their data and continue their operations.

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

### Once

The once strategy initially calls the batch function under one of two
conditions:

1.  If `InBackground` option is false, it executes the batch function then Thunk
    is invoked
2.  If `InBackground` option is true, it executes the batch function in a
    background go routine. Calls to the Thunk function will block until the go
    routine completes.

The once strategy is useful when wanting to execute a batch function a single
time either directly, and keeping a consistent application api aligning with the
other functions. Or when wanting to call the batch function in the background
while performing some other time consuming operation and not wanting to handle
the actual go routine.

## TODO

- [x] Set a max duration that a call to `Load(Key)` can block. Start from the
      initial call to `Load(Key)`.
- [x] Caching approach/interface
- [x] Tests!!
- [x] LoadMany - ability to call load with multiple keys
- [ ] Examples
- [ ] Logging
- [x] Tracing
