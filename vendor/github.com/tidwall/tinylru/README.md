# `tinylru`

[![GoDoc](https://godoc.org/github.com/tidwall/tinylru?status.svg)](https://godoc.org/github.com/tidwall/tinylru)

A fast little LRU cache. 

## Getting Started

### Installing

To start using `tinylru`, install Go and run go get:

```
$ go get -u github.com/tidwall/tinylru
```

This will retrieve the library.

### Usage

Keys and values are interfaces.

```go
// Create an LRU cache
var cache tinylru.LRU

// Set the cache size. This is the maximum number of items that the cache can
// hold before evicting old items. The default size is 256.
cache.Resize(1024)

// Set a key. Returns the previous value and ok if a previous value exists.
prev, ok := cache.Set("hello", "world")

// Get a key. Returns the value and ok if the value exists.
value, ok := cache.Get("hello")

// Delete a key. Returns the deleted value and ok if a previous value exists.
prev, ok := tr.Delete("hello")
```

A `Set` function may evict old items when adding a new item while LRU is at
capacity. If you want to know what was evicted then use the `SetEvicted`
function.

```go
// Set a key and return the evicted item, if any.
prev, ok, evictedKey, evictedValue, evicted := cache.SetEvicted("hello")
```

### Contact

Josh Baker [@tidwall](https://twitter.com/tidwall)

### License

`tinylru` source code is available under the MIT License.
