/*
 Copyright Digital Asset Holdings, LLC 2016 All Rights Reserved.

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

package errors

import (
	"fmt"
	"strings"
	"testing"

	"github.com/hyperledger/fabric/common/flogging"
)

func TestError(t *testing.T) {
	e := Error("Utility", "ErrorWithArg", "An unknown error occurred.")
	s := e.GetStack()
	if s != "" {
		t.Fatalf("No error stack should have been recorded.")
	}
}

// TestErrorWithArg tests creating an error with a message argument
func TestErrorWithArg(t *testing.T) {
	e := Error("Utility", "ErrorWithArg", "An error occurred: %s", "arg1")
	s := e.GetStack()
	if s != "" {
		t.Fatalf("No error stack should have been recorded.")
	}
}

func TestErrorWithCallstack(t *testing.T) {
	e := ErrorWithCallstack("Utility", "UnknownError", "An unknown error occurred.")
	s := e.GetStack()
	if s == "" {
		t.Fatalf("No error stack was recorded.")
	}
}

// TestErrorWithCallstackAndArg tests creating an error with a callstack and
// message argument
func TestErrorWithCallstackAndArg(t *testing.T) {
	e := ErrorWithCallstack("Utility", "ErrorWithArg", "An error occurred: %s", "arg1")
	s := e.GetStack()
	if s == "" {
		t.Fatalf("No error stack was recorded.")
	}
}

// TestErrorWithCallstackMessage tests the output for a logging error where
// and an invalid log level has been provided and the stack trace should be
// displayed with the error message
func TestErrorWithCallstackMessage(t *testing.T) {
	// when the 'error' module is set to debug, the callstack will be appended
	// to the error message
	flogging.SetModuleLevel("error", "debug")

	e := ErrorWithCallstack("Utility", "ErrorWithArg", "An unknown error occurred.")
	s := e.GetStack()
	if s == "" {
		t.Fatalf("No error stack was recorded.")
	}

	// check that the error message contains this part of the stack trace, which
	// is non-platform specific
	if !strings.Contains(e.Error(), "github.com/hyperledger/fabric/core/errors.TestErrorWithCallstackMessage") {
		t.Fatalf("Error message does not have stack trace appended.")
	}
}

func ExampleError() {
	// when the 'error' module is set to anything but debug, the callstack will
	// not be appended to the error message
	flogging.SetModuleLevel("error", "warning")

	err := ErrorWithCallstack("Utility", "UnknownError", "An unknown error occurred.")

	if err != nil {
		fmt.Printf("%s\n", err.Error())
		fmt.Printf("%s\n", err.GetErrorCode())
		fmt.Printf("%s\n", err.GetComponentCode())
		fmt.Printf("%s\n", err.GetReasonCode())
		fmt.Printf("%s\n", err.Message())
		// Output:
		// An unknown error occurred.
		// Utility-UnknownError
		// Utility
		// UnknownError
		// An unknown error occurred.
	}
}

// ExampleErrorWithArg tests the output for a sample error with a message
// argument
func Example_utilityErrorWithArg() {
	// when the 'error' module is set to anything but debug, the callstack will
	// not be appended to the error message
	flogging.SetModuleLevel("error", "warning")

	err := ErrorWithCallstack("Utility", "ErrorWithArg", "An error occurred: %s", "arg1")

	if err != nil {
		fmt.Printf("%s\n", err.Error())
		fmt.Printf("%s\n", err.GetErrorCode())
		fmt.Printf("%s\n", err.GetComponentCode())
		fmt.Printf("%s\n", err.GetReasonCode())
		fmt.Printf("%s\n", err.Message())
		// Output:
		// An error occurred: arg1
		// Utility-ErrorWithArg
		// Utility
		// ErrorWithArg
		// An error occurred: arg1
	}
}

// Example_loggingInvalidLevel tests the output for a logging error where
// and an invalid log level has been provided
func Example_loggingInvalidLevel() {
	// when the 'error' module is set to anything but debug, the callstack will
	// not be appended to the error message
	flogging.SetModuleLevel("error", "warning")

	err := ErrorWithCallstack("Logging", "InvalidLevel", "Invalid log level provided - %s", "invalid")

	if err != nil {
		fmt.Printf("%s\n", err.Error())
		fmt.Printf("%s\n", err.GetErrorCode())
		fmt.Printf("%s\n", err.GetComponentCode())
		fmt.Printf("%s\n", err.GetReasonCode())
		fmt.Printf("%s\n", err.Message())
		// Output:
		// Invalid log level provided - invalid
		// Logging-InvalidLevel
		// Logging
		// InvalidLevel
		// Invalid log level provided - invalid
	}
}
