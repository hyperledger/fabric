Overview [![Build Status](https://travis-ci.org/magiconair/properties.png?branch=master)](https://travis-ci.org/magiconair/properties)
========

properties is a Go library for reading and writing properties files.

It supports reading from multiple files and Spring style recursive property
expansion of expressions like `${key}` to their corresponding value.
Value expressions can refer to other keys like in `${key}` or to
environment variables like in `${USER}`.
Filenames can also contain environment variables like in
`/home/${USER}/myapp.properties`.

Comments and the order of keys are preserved. Comments can be modified
and can be written to the output.

The properties library supports both ISO-8859-1 and UTF-8 encoded data.

Starting from version 1.3.0 the behavior of the MustXXX() functions is
configurable by providing a custom `ErrorHandler` function. The default has
changed from `panic` to `log.Fatal` but this is configurable and custom
error handling functions can be provided. See the package documentation for
details.

Getting Started
---------------

```go
import "github.com/magiconair/properties"

func main() {
	p := properties.MustLoadFile("${HOME}/config.properties", properties.UTF8)
	host := p.MustGetString("host")
	port := p.GetInt("port", 8080)
}

```

Read the full documentation on [GoDoc](https://godoc.org/github.com/magiconair/properties)   [![GoDoc](https://godoc.org/github.com/magiconair/properties?status.png)](https://godoc.org/github.com/magiconair/properties)

Installation and Upgrade
------------------------

```
$ go get -u github.com/magiconair/properties
```

For testing and debugging you need the [go-check](https://github.com/go-check/check) library

```
$ go get -u gopkg.in/check.v1
```

History
-------

v1.5.5, 31 Jul 2015
-------------------
 * [Pull Request #6](https://github.com/magiconair/properties/pull/6): Add [Delete](http://godoc.org/github.com/magiconair/properties#Properties.Delete) method to remove keys including comments. (@gerbenjacobs)

v1.5.4, 23 Jun 2015
-------------------
 * [Issue #5](https://github.com/magiconair/properties/issues/5): Allow disabling of property expansion [DisableExpansion](http://godoc.org/github.com/magiconair/properties#Properties.DisableExpansion). When property expansion is disabled Properties become a simple key/value store and don't check for circular references.

v1.5.3, 02 Jun 2015
-------------------
 * [Issue #4](https://github.com/magiconair/properties/issues/4): Maintain key order in [Filter()](http://godoc.org/github.com/magiconair/properties#Properties.Filter), [FilterPrefix()](http://godoc.org/github.com/magiconair/properties#Properties.FilterPrefix) and [FilterRegexp()](http://godoc.org/github.com/magiconair/properties#Properties.FilterRegexp)

v1.5.2, 10 Apr 2015
-------------------
 * [Issue #3](https://github.com/magiconair/properties/issues/3): Don't print comments in [WriteComment()](http://godoc.org/github.com/magiconair/properties#Properties.WriteComment) if they are all empty
 * Add clickable links to README

v1.5.1, 08 Dec 2014
-------------------
 * Added [GetParsedDuration()](http://godoc.org/github.com/magiconair/properties#Properties.GetParsedDuration) and [MustGetParsedDuration()](http://godoc.org/github.com/magiconair/properties#Properties.MustGetParsedDuration) for values specified compatible with
   [time.ParseDuration()](http://golang.org/pkg/time/#ParseDuration).

v1.5.0, 18 Nov 2014
-------------------
 * Added support for single and multi-line comments (reading, writing and updating)
 * The order of keys is now preserved
 * Calling [Set()](http://godoc.org/github.com/magiconair/properties#Properties.Set) with an empty key now silently ignores the call and does not create a new entry
 * Added a [MustSet()](http://godoc.org/github.com/magiconair/properties#Properties.MustSet) method
 * Migrated test library from launchpad.net/gocheck to [gopkg.in/check.v1](http://gopkg.in/check.v1)

v1.4.2, 15 Nov 2014
-------------------
 * [Issue #2](https://github.com/magiconair/properties/issues/2): Fixed goroutine leak in parser which created two lexers but cleaned up only one

v1.4.1, 13 Nov 2014
-------------------
 * [Issue #1](https://github.com/magiconair/properties/issues/1): Fixed bug in Keys() method which returned an empty string

v1.4.0, 23 Sep 2014
-------------------
 * Added [Keys()](http://godoc.org/github.com/magiconair/properties#Properties.Keys) to get the keys
 * Added [Filter()](http://godoc.org/github.com/magiconair/properties#Properties.Filter), [FilterRegexp()](http://godoc.org/github.com/magiconair/properties#Properties.FilterRegexp) and [FilterPrefix()](http://godoc.org/github.com/magiconair/properties#Properties.FilterPrefix) to get a subset of the properties

v1.3.0, 18 Mar 2014
-------------------
* Added support for time.Duration
* Made MustXXX() failure behavior configurable (log.Fatal, panic, custom)
* Changed default of MustXXX() failure from panic to log.Fatal

v1.2.0, 05 Mar 2014
-------------------
* Added MustGet... functions
* Added support for int and uint with range checks on 32 bit platforms

v1.1.0, 20 Jan 2014
-------------------
* Renamed from goproperties to properties
* Added support for expansion of environment vars in
  filenames and value expressions
* Fixed bug where value expressions were not at the
  start of the string

v1.0.0, 7 Jan 2014
------------------
* Initial release

License
-------

2 clause BSD license. See [LICENSE](https://github.com/magiconair/properties/blob/master/LICENSE) file for details.

ToDo
----
* Dump contents with passwords and secrets obscured
