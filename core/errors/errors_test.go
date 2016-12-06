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
	"testing"

	"github.com/hyperledger/fabric/flogging"
)

func TestError(t *testing.T) {
	e := Error(Utility, UtilityUnknownError)
	s := e.GetStack()
	if s != "" {
		t.Fatalf("No error stack should have been recorded.")
	}
}

// TestErrorWithArg tests creating an error with a message argument
func TestErrorWithArg(t *testing.T) {
	e := Error(Utility, UtilityErrorWithArg, "arg1")
	s := e.GetStack()
	if s != "" {
		t.Fatalf("No error stack should have been recorded.")
	}
}

func TestErrorWithCallstack(t *testing.T) {
	e := ErrorWithCallstack(Utility, UtilityUnknownError)
	s := e.GetStack()
	if s == "" {
		t.Fatalf("No error stack was recorded.")
	}
}

// TestErrorWithCallstackAndArg tests creating an error with a callstack and
// message argument
func TestErrorWithCallstackAndArg(t *testing.T) {
	e := ErrorWithCallstack(Utility, UtilityErrorWithArg, "arg1")
	s := e.GetStack()
	if s == "" {
		t.Fatalf("No error stack was recorded.")
	}
}

func ExampleError() {
	// when the 'error' module is set to anything but debug, the callstack will
	// not be appended to the error message
	flogging.SetModuleLogLevel("error", "warning")

	err := ErrorWithCallstack(Utility, UtilityUnknownError)

	if err != nil {
		fmt.Printf("%s\n", err.Error())
		fmt.Printf("%s\n", err.GetErrorCode())
		fmt.Printf("%s\n", err.GetComponentCode())
		fmt.Printf("%s\n", err.GetReasonCode())
		fmt.Printf("%s\n", err.Message())
		fmt.Printf("%s\n", err.MessageIn("en"))
		// Output:
		// An unknown error occurred.
		// Utility-UtilityUnknownError
		// Utility
		// UtilityUnknownError
		// An unknown error occurred.
		// An unknown error occurred.
	}
}

// ExampleErrorWithArg tests the output for a sample error with a message
// argument
func ExampleUtilityErrorWithArg() {
	// when the 'error' module is set to anything but debug, the callstack will
	// not be appended to the error message
	flogging.SetModuleLogLevel("error", "warning")

	err := ErrorWithCallstack(Utility, UtilityErrorWithArg, "arg1")

	if err != nil {
		fmt.Printf("%s\n", err.Error())
		fmt.Printf("%s\n", err.GetErrorCode())
		fmt.Printf("%s\n", err.GetComponentCode())
		fmt.Printf("%s\n", err.GetReasonCode())
		fmt.Printf("%s\n", err.Message())
		fmt.Printf("%s\n", err.MessageIn("en"))
		// Output:
		// An error occurred: arg1
		// Utility-UtilityErrorWithArg
		// Utility
		// UtilityErrorWithArg
		// An error occurred: arg1
		// An error occurred: arg1
	}
}

// ExampleLoggingInvalidLogLevel tests the output for a logging error where
// and an invalid log level has been provided
func ExampleLoggingInvalidLogLevel() {
	// when the 'error' module is set to anything but debug, the callstack will
	// not be appended to the error message
	flogging.SetModuleLogLevel("error", "warning")

	err := ErrorWithCallstack(Logging, LoggingInvalidLogLevel, "invalid")

	if err != nil {
		fmt.Printf("%s\n", err.Error())
		fmt.Printf("%s\n", err.GetErrorCode())
		fmt.Printf("%s\n", err.GetComponentCode())
		fmt.Printf("%s\n", err.GetReasonCode())
		fmt.Printf("%s\n", err.Message())
		fmt.Printf("%s\n", err.MessageIn("en"))
		// Output:
		// Invalid log level provided - invalid
		// Logging-LoggingInvalidLogLevel
		// Logging
		// LoggingInvalidLogLevel
		// Invalid log level provided - invalid
		// Invalid log level provided - invalid
	}
}

// ExampleLoggingInvalidLogLevel tests the output for a logging error where
// and an invalid log level has been provided and the stack trace should be
// displayed with the error message
func ExampleLoggingInvalidLogLevel_withCallstack() {
	// when the 'error' module is set to debug, the callstack will be appended
	// to the error message
	flogging.SetModuleLogLevel("error", "debug")

	err := ErrorWithCallstack(Logging, LoggingInvalidLogLevel, "invalid")

	if err != nil {
		fmt.Printf("%s", err.Error())
		fmt.Printf("%s\n", err.GetErrorCode())
		fmt.Printf("%s\n", err.GetComponentCode())
		fmt.Printf("%s\n", err.GetReasonCode())
		fmt.Printf("%s", err.Message())
		fmt.Printf("%s\n", err.MessageIn("en"))
		// Output:
		// Invalid log level provided - invalid
		// /opt/gopath/src/github.com/hyperledger/fabric/core/errors/errors_test.go:145 github.com/hyperledger/fabric/core/errors.ExampleLoggingInvalidLogLevel_withCallstack
		// /opt/go/src/testing/example.go:115 testing.runExample
		// /opt/go/src/testing/example.go:38 testing.RunExamples
		// /opt/go/src/testing/testing.go:744 testing.(*M).Run
		// github.com/hyperledger/fabric/core/errors/_test/_testmain.go:116 main.main
		// /opt/go/src/runtime/proc.go:192 runtime.main
		// /opt/go/src/runtime/asm_amd64.s:2087 runtime.goexit
		// Logging-LoggingInvalidLogLevel
		// Logging
		// LoggingInvalidLogLevel
		// Invalid log level provided - invalid
		// /opt/gopath/src/github.com/hyperledger/fabric/core/errors/errors_test.go:145 github.com/hyperledger/fabric/core/errors.ExampleLoggingInvalidLogLevel_withCallstack
		// /opt/go/src/testing/example.go:115 testing.runExample
		// /opt/go/src/testing/example.go:38 testing.RunExamples
		// /opt/go/src/testing/testing.go:744 testing.(*M).Run
		// github.com/hyperledger/fabric/core/errors/_test/_testmain.go:116 main.main
		// /opt/go/src/runtime/proc.go:192 runtime.main
		// /opt/go/src/runtime/asm_amd64.s:2087 runtime.goexit
		// Invalid log level provided - invalid
	}
}
