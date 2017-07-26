# A buffered statsd reporter

See `examples/statsd_main.go` for an end to end example.

Some emitted stats using the example listening with `nc 8125 -l -u`:

```
stats.my-service.test-histogram.100ms-200ms:2|c
stats.my-service.test-histogram.300ms-400ms:1|c
stats.my-service.test-histogram.600ms-800ms:1|c
stats.my-service.test-counter:1|c
stats.my-service.test-gauge:813|g
```

## Options

You can use either a basic or a buffered statsd client
and pass it to the reporter along with options.

The reporter options are:

```go
// Options is a set of options for the tally reporter.
type Options struct {
	// SampleRate is the metrics emission sample rate. If you
	// do not set this value it will be set to 1.
	SampleRate float32
}
```
