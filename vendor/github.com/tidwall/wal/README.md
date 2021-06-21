# `wal`
[![GoDoc](https://godoc.org/github.com/tidwall/wal?status.svg)](https://godoc.org/github.com/tidwall/wal)

A simple and fast write ahead log for Go.

## Features

- High durability
- Fast operations
- Monotonic indexes
- Batch writes
- Log truncation from front or back.

## Getting Started

### Installing

To start using `wal`, install Go and run `go get`:

```sh
$ go get -u github.com/tidwall/wal
```

This will retrieve the library.

### Example

```go
// open a new log file
log, err := Open("mylog", nil)

// write some entries
err = log.Write(1, []byte("first entry"))
err = log.Write(2, []byte("second entry"))
err = log.Write(3, []byte("third entry"))

// read an entry
data, err := log.Read(1)
println(string(data))  // output: first entry

// close the log
err = log.Close()
```

Batch writes:

```go

// write three entries as a batch
batch := new(Batch)
batch.Write(1, []byte("first entry"))
batch.Write(2, []byte("second entry"))
batch.Write(3, []byte("third entry"))

err = log.WriteBatch(batch)
```

Truncating:

```go
// write some entries
err = log.Write(1, []byte("first entry"))
...
err = log.Write(1000, []byte("thousandth entry"))

// truncate the log from index starting 350 and ending with 950.
err = l.TruncateFront(350)
err = l.TruncateBack(950)
```



## Contact

Josh Baker [@tidwall](http://twitter.com/tidwall)

## License

`wal` source code is available under the MIT [License](/LICENSE).
