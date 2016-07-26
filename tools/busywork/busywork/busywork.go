/*
Copyright IBM Corp. 2016. All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

		 http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Common support for busywork test cases

package busywork

import (
	"errors"
	"fmt"
)

// It is not clear why Go developers think it is a good idea to explicitly
// pass error indications for irrecoverable errors up through layer after
// layer after layer, ... of software. This only complicates and obfuscates
// mid- and high-level code with incredible amounts of redundant error
// handling. Here we use a better method "sanctioned" by Donovan and Kernighan's
// "The Go Programming Language". Irrecoverable errors in busywork code can
// "throw" an IrrecoverableError object as a panic. These objects are trapped
// and converted to normal error returns in the recovery handler
// Catch(). Panics that "throw" other types of objects are allowed to continue
// panicing.

// IrrecoverableError is a special object used to signal busywork
// irrecoverable errors as panics.
type IrrecoverableError struct {
	error string // The error string
}

// Throw signals an irrecoverable error as a formatted string
func Throw(format string, args ...interface{}) {
	panic(IrrecoverableError{error: fmt.Sprintf(format, args...)})
}

// Catch catches irrecoverable errors in busywork applications. This function
// must be called "deferred" from a function that returns an error. In the
// event of a busywork panic the input error will be assigned to a new error
// containing the panic message.
func Catch(err *error) {
	p := recover()
	switch p.(type) {
	case nil: // No Panic
	case IrrecoverableError:
		*err = errors.New(p.(IrrecoverableError).error) // Panic caught
	default:
		panic(p) // Propagate unknown panic
	}
}

// SizeOfInt computes the size of an integer in bytes
func SizeOfInt() int {
	var x = 0x7fffffff // golint says this is an int by default
	if (x + 1) < 0 {
		return 4
	}
	return 8
}
