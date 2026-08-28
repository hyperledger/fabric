# Terminal progress bar for Go

[![Coverage Status](https://coveralls.io/repos/github/cheggaaa/pb/badge.svg)](https://coveralls.io/github/cheggaaa/pb)

## Installation

```
go get github.com/cheggaaa/pb/v3
```

Documentation for v1 bar available [here](README_V1.md).

## Quick start

```Go
package main

import (
	"time"

	"github.com/cheggaaa/pb/v3"
)

func main() {
	count := 100000

	// create and start new bar
	bar := pb.StartNew(count)

	// start bar from 'default' template
	// bar := pb.Default.Start(count)

	// start bar from 'simple' template
	// bar := pb.Simple.Start(count)

	// start bar from 'full' template
	// bar := pb.Full.Start(count)

	for i := 0; i < count; i++ {
		bar.Increment()
		time.Sleep(time.Millisecond)
	}

	// finish bar
	bar.Finish()
}
```

Result will be like this:

```
> go run test.go
37158 / 100000 [---------------->_______________________________] 37.16% 916 p/s
```

## Settings

```Go
// create bar
bar := pb.New(count)

// refresh info every second (default 200ms)
bar.SetRefreshRate(time.Second)

// force set io.Writer, by default it's os.Stderr
bar.SetWriter(os.Stdout)

// bar will format numbers as bytes (B, KiB, MiB, etc)
bar.Set(pb.Bytes, true)

// bar use SI bytes prefix names (B, kB) instead of IEC (B, KiB)
bar.Set(pb.SIBytesPrefix, true)

// set custom bar template
bar.SetTemplateString(myTemplate)

// check for error after template set
if err := bar.Err(); err != nil {
    return
}

// start bar
bar.Start()
```

## Progress bar for IO Operations

```Go
package main

import (
	"crypto/rand"
	"io"
	"io/ioutil"

	"github.com/cheggaaa/pb/v3"
)

func main() {
	var limit int64 = 1024 * 1024 * 500

	// we will copy 500 MiB from /dev/rand to /dev/null
	reader := io.LimitReader(rand.Reader, limit)
	writer := ioutil.Discard

	// start new bar
	bar := pb.Full.Start64(limit)

	// create proxy reader
	barReader := bar.NewProxyReader(reader)

	// copy from proxy reader
	io.Copy(writer, barReader)

	// finish bar
	bar.Finish()
}
```

## Custom Progress Bar templates

Rendering based on builtin [text/template](https://pkg.go.dev/text/template) package. You can use existing pb's elements or create you own.

All available elements are described in the [element.go](v3/element.go) file.
The `bar` element accepts five standard parts: left border, fill, current, empty, and right border. It also accepts optional sixth and seventh parts for the empty left border and finished right border.

Set `UNICODE_PROGRESS_BAR=true` to use the built-in Unicode progress bar glyphs for bars that use the default bar elements when the active terminal font supports them.

#### All in one example:

```Go
tmpl := `{{ red "With funcs:" }} {{ bar . "<" "-" (cycle . "↖" "↗" "↘" "↙" ) "." ">"}} {{speed . | rndcolor }} {{percent .}} {{string . "my_green_string" | green}} {{string . "my_blue_string" | blue}}`

// start bar based on our template
bar := pb.ProgressBarTemplate(tmpl).Start64(limit)

// set values for string elements
bar.Set("my_green_string", "green").Set("my_blue_string", "blue")
```
