// Package assert provides C-like assertions for Go. For more
// information, see assert(3) (e.g. `man 3 assert`).
//
// The T variants operating on *testing.T values; instead of killing
// the program, they call the Fatal method.
//
// If GOTRACEBACK is set to enable coredumps, assertions will generate
// coredumps.
package assert

import (
	"fmt"
	"os"
	"runtime"
	"strings"
	"testing"
)

// NoDebug, if set to true, will cause all asserts to be ignored.
var NoDebug bool

func die(what string, a ...string) {
	_, file, line, ok := runtime.Caller(2)
	if !ok {
		panic(what)
	}

	if os.Getenv("GOTRACEBACK") == "crash" {
		s := strings.Join(a, ", ")
		if len(s) > 0 {
			s = ": " + s
		}
		panic(what + s)
	} else {
		fmt.Fprintf(os.Stderr, "%s", what)
		if len(a) > 0 {
			s := strings.Join(a, ", ")
			fmt.Fprintln(os.Stderr, ": "+s)
		} else {
			fmt.Fprintf(os.Stderr, "\n")
		}

		fmt.Fprintf(os.Stderr, "\t%s line %d\n", file, line)

		os.Exit(1)
	}
}

// Bool asserts that cond is false.
//
// For example, this would replace
//    if x < 0 {
//            log.Fatal("x is subzero")
//    }
//
// The same assertion would be
//    assert.Bool(x, "x is subzero")
func Bool(cond bool, s ...string) {
	if NoDebug {
		return
	}

	if !cond {
		die("assert.Bool failed", s...)
	}
}

// Error asserts that err is not nil, e.g. that an error has occurred.
//
// For example,
//     if err == nil {
//             log.Fatal("call to <something> should have failed")
//     }
//     // becomes
//     assert.Error(err, "call to <something> should have failed")
func Error(err error, s ...string) {
	if NoDebug {
		return
	} else if nil != err {
		return
	}

	if len(s) == 0 {
		die("error expected, but no error returned")
	} else {
		die(strings.Join(s, ", "))
	}
}

// NoError asserts that err is nil, e.g. that no error has occurred.
func NoError(err error, s ...string) {
	if NoDebug {
		return
	}

	if nil != err {
		die(err.Error())
	}
}

// ErrorEq asserts that the actual error is the expected error.
func ErrorEq(expected, actual error) {
	if NoDebug || (expected == actual) {
		return
	}

	if expected == nil {
		die(fmt.Sprintf("assert.ErrorEq: %s", actual.Error()))
	}

	var should string
	if actual == nil {
		should = "no error was returned"
	} else {
		should = fmt.Sprintf("have '%s'", actual)
	}

	die(fmt.Sprintf("assert.ErrorEq: expected '%s', but %s", expected, should))
}

// BoolT checks a boolean condition, calling Fatal on t if it is
// false.
func BoolT(t *testing.T, cond bool, s ...string) {
	if !cond {
		what := strings.Join(s, ", ")
		if len(what) > 0 {
			what = ": " + what
		}
		t.Fatalf("assert.Bool failed%s", what)
	}
}

// ErrorT asserts that err is not nil, e.g. asserting that an error
// has occurred. See also NoErrorT.
func ErrorT(t *testing.T, err error, s ...string) {
	if nil != err {
		return
	}

	if len(s) == 0 {
		t.Fatal("error expected, but no error returned")
	} else {
		t.Fatal(strings.Join(s, ", "))
	}
}

// NoErrorT asserts that err is nil, e.g. asserting that no error has
// occurred. See also ErrorT.
func NoErrorT(t *testing.T, err error) {
	if nil != err {
		t.Fatalf("%s", err)
	}
}

// ErrorEqT compares a pair of errors, calling Fatal on it if they
// don't match.
func ErrorEqT(t *testing.T, expected, actual error) {
	if NoDebug || (expected == actual) {
		return
	}

	if expected == nil {
		die(fmt.Sprintf("assert.Error2: %s", actual.Error()))
	}

	var should string
	if actual == nil {
		should = "no error was returned"
	} else {
		should = fmt.Sprintf("have '%s'", actual)
	}

	die(fmt.Sprintf("assert.Error2: expected '%s', but %s", expected, should))
}
