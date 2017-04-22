/*
 Copyright Digital Asset Holdings, LLC 2016 All Rights Reserved.
 Copyright IBM Corp. 2017 All Rights Reserved.

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
	e := Error("UNK", "404", "An unknown error occurred.")
	s := e.GetStack()
	if s != "" {
		t.Fatalf("No error stack should have been recorded.")
	}
}

// TestErrorWithArg tests creating an error with a message argument
func TestErrorWithArg(t *testing.T) {
	e := Error("UNK", "405", "An error occurred: %s", "arg1")
	s := e.GetStack()
	if s != "" {
		t.Fatalf("No error stack should have been recorded.")
	}
}

func TestErrorWithCallstack(t *testing.T) {
	e := ErrorWithCallstack("UNK", "404", "An unknown error occurred.")
	s := e.GetStack()
	if s == "" {
		t.Fatalf("No error stack was recorded.")
	}
}

func TestErrorWithCallstack_wrapped(t *testing.T) {
	e := ErrorWithCallstack("UNK", "404", "An unknown error occurred.")
	s := e.GetStack()
	if s == "" {
		t.Fatalf("No error stack was recorded.")
	}

	e2 := ErrorWithCallstack("CHA", "404", "An unknown error occurred.").WrapError(e)
	s2 := e2.GetStack()
	if s2 == "" {
		t.Fatalf("No error stack was recorded.")
	}
}

// TestErrorWithCallstackAndArg tests creating an error with a callstack and
// message argument
func TestErrorWithCallstackAndArg(t *testing.T) {
	e := ErrorWithCallstack("UNK", "405", "An error occurred: %s", "arg1")
	s := e.GetStack()
	if s == "" {
		t.Fatalf("No error stack was recorded.")
	}
}

func TestErrorWithCallstackAndArg_wrappedNoCallstack(t *testing.T) {
	e := Error("UNK", "405", "An error occurred: %s", "arg1")
	s := e.GetStack()
	if s != "" {
		t.Fatalf("No error stack should have been recorded.")
	}

	e2 := ErrorWithCallstack("CHA", "404", "An error occurred: %s", "anotherarg1").WrapError(e)
	s2 := e2.GetStack()
	if s2 == "" {
		t.Fatalf("No error stack was recorded.")
	}
}

func TestError_wrappedWithCallstackAndArg(t *testing.T) {
	e := ErrorWithCallstack("UNK", "405", "An error occurred: %s", "arg1")
	s := e.GetStack()
	if s == "" {
		t.Fatalf("No error stack should have been recorded.")
	}

	e2 := Error("CHA", "404", "An error occurred: %s", "anotherarg1").WrapError(e)
	s2 := e2.GetStack()
	if s2 != "" {
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

	e := ErrorWithCallstack("UNK", "405", "An unknown error occurred.")
	s := e.GetStack()
	if s == "" {
		t.Fatalf("No error stack was recorded.")
	}

	// check that the error message contains this part of the stack trace, which
	// is non-platform specific
	if !strings.Contains(e.Error(), "github.com/hyperledger/fabric/common/errors.TestErrorWithCallstackMessage") {
		t.Fatalf("Error message does not have stack trace appended.")
	}
}

func TestErrorWithCallstackMessage_wrapped(t *testing.T) {
	// when the 'error' module is set to debug, the callstack will be appended
	// to the error message
	flogging.SetModuleLevel("error", "debug")

	e := ErrorWithCallstack("UNK", "405", "An error occurred: %s", "arg1")
	s := e.GetStack()
	if s == "" {
		t.Fatalf("No error stack was recorded.")
	}

	// check that the error message contains this part of the stack trace, which
	// is non-platform specific
	if !strings.Contains(e.Error(), "github.com/hyperledger/fabric/common/errors.TestErrorWithCallstackMessage_wrapped") {
		t.Fatalf("Error message does not have stack trace appended.")
	}

	e2 := ErrorWithCallstack("CHA", "405", "A chaincode error occurred: %s", "ccarg1").WrapError(e)
	s2 := e2.GetStack()
	if s2 == "" {
		t.Fatalf("No error stack was recorded.")
	}

	// check that the error message contains this part of the stack trace, which
	// is non-platform specific
	if !strings.Contains(e2.Error(), "github.com/hyperledger/fabric/common/errors.TestErrorWithCallstackMessage_wrapped") {
		t.Fatalf("Error message does not have stack trace appended.")
	}
}

func TestIsValidComponentOrReasonCode(t *testing.T) {
	validComponents := []string{"LGR", "CHA", "PER", "xyz", "aBc"}
	for _, component := range validComponents {
		if ok := isValidComponentOrReasonCode(component, componentPattern); !ok {
			t.FailNow()
		}
	}

	validReasons := []string{"404", "500", "999"}
	for _, reason := range validReasons {
		if ok := isValidComponentOrReasonCode(reason, reasonPattern); !ok {
			t.FailNow()
		}
	}

	invalidComponents := []string{"LEDG", "CH", "P3R", "123", ""}
	for _, component := range invalidComponents {
		if ok := isValidComponentOrReasonCode(component, componentPattern); ok {
			t.FailNow()
		}
	}

	invalidReasons := []string{"4045", "E12", "ZZZ", "1", ""}
	for _, reason := range invalidReasons {
		if ok := isValidComponentOrReasonCode(reason, reasonPattern); ok {
			t.FailNow()
		}
	}

}

func ExampleError() {
	// when the 'error' module is set to anything but debug, the callstack will
	// not be appended to the error message
	flogging.SetModuleLevel("error", "warning")

	err := Error("UNK", "404", "An unknown error occurred.")

	if err != nil {
		fmt.Printf("%s\n", err.Error())
		fmt.Printf("%s\n", err.GetErrorCode())
		fmt.Printf("%s\n", err.GetComponentCode())
		fmt.Printf("%s\n", err.GetReasonCode())
		fmt.Printf("%s\n", err.Message())
		// Output:
		// UNK:404 - An unknown error occurred.
		// UNK:404
		// UNK
		// 404
		// UNK:404 - An unknown error occurred.
	}
}

func ExampleErrorWithCallstack() {
	// when the 'error' module is set to anything but debug, the callstack will
	// not be appended to the error message
	flogging.SetModuleLevel("error", "warning")

	err := ErrorWithCallstack("UNK", "404", "An unknown error occurred.")

	if err != nil {
		fmt.Printf("%s\n", err.Error())
		fmt.Printf("%s\n", err.GetErrorCode())
		fmt.Printf("%s\n", err.GetComponentCode())
		fmt.Printf("%s\n", err.GetReasonCode())
		fmt.Printf("%s\n", err.Message())
		// Output:
		// UNK:404 - An unknown error occurred.
		// UNK:404
		// UNK
		// 404
		// UNK:404 - An unknown error occurred.
	}
}

// Example_utilityErrorWithArg tests the output for a sample error with a message
// argument
func Example_utilityErrorWithArg() {
	// when the 'error' module is set to anything but debug, the callstack will
	// not be appended to the error message
	flogging.SetModuleLevel("error", "warning")

	err := ErrorWithCallstack("UNK", "405", "An error occurred: %s", "arg1")

	if err != nil {
		fmt.Printf("%s\n", err.Error())
		fmt.Printf("%s\n", err.GetErrorCode())
		fmt.Printf("%s\n", err.GetComponentCode())
		fmt.Printf("%s\n", err.GetReasonCode())
		fmt.Printf("%s\n", err.Message())
		// Output:
		// UNK:405 - An error occurred: arg1
		// UNK:405
		// UNK
		// 405
		// UNK:405 - An error occurred: arg1
	}
}

// Example_wrappedUtilityErrorWithArg tests the output for a CallStackError
// with message argument that is wrapped into another error.
func Example_wrappedUtilityErrorWithArg() {
	// when the 'error' module is set to anything but debug, the callstack will
	// not be appended to the error message
	flogging.SetModuleLevel("error", "warning")

	wrappedErr := ErrorWithCallstack("UNK", "405", "An error occurred: %s", "arg1")
	err := ErrorWithCallstack("CHA", "500", "Utility error occurred: %s", "ccarg1").WrapError(wrappedErr)

	if err != nil {
		fmt.Printf("%s\n", err.Error())
		fmt.Printf("%s\n", err.GetErrorCode())
		fmt.Printf("%s\n", err.GetComponentCode())
		fmt.Printf("%s\n", err.GetReasonCode())
		fmt.Printf("%s\n", err.Message())
		// Output:
		// CHA:500 - Utility error occurred: ccarg1
		// Caused by: UNK:405 - An error occurred: arg1
		// CHA:500
		// CHA
		// 500
		// CHA:500 - Utility error occurred: ccarg1
		// Caused by: UNK:405 - An error occurred: arg1
	}
}

// Example_wrappedStandardError tests the output for a standard error
// with message argument that is wrapped into a CallStackError.
func Example_wrappedStandardError() {
	// when the 'error' module is set to anything but debug, the callstack will
	// not be appended to the error message
	flogging.SetModuleLevel("error", "warning")

	wrappedErr := fmt.Errorf("grpc timed out: %s", "failed to connect to server")
	err := ErrorWithCallstack("CHA", "500", "Error sending message: %s", "invoke").WrapError(wrappedErr)

	if err != nil {
		fmt.Printf("%s\n", err.Error())
		fmt.Printf("%s\n", err.GetErrorCode())
		fmt.Printf("%s\n", err.GetComponentCode())
		fmt.Printf("%s\n", err.GetReasonCode())
		fmt.Printf("%s\n", err.Message())
		// Output:
		// CHA:500 - Error sending message: invoke
		// Caused by: grpc timed out: failed to connect to server
		// CHA:500
		// CHA
		// 500
		// CHA:500 - Error sending message: invoke
		// Caused by: grpc timed out: failed to connect to server
	}
}

// Example_wrappedStandardError2 tests the output for CallStackError wrapped
// into a standard error with message argument that is wrapped into a
// CallStackError.
func Example_wrappedStandardError2() {
	// when the 'error' module is set to anything but debug, the callstack will
	// not be appended to the error message
	flogging.SetModuleLevel("error", "warning")

	wrappedErr := ErrorWithCallstack("CON", "500", "failed to connect to server")
	wrappedErr2 := fmt.Errorf("grpc timed out: %s", wrappedErr)
	err := ErrorWithCallstack("CHA", "500", "Error sending message: %s", "invoke").WrapError(wrappedErr2)

	if err != nil {
		fmt.Printf("%s\n", err.Error())
		fmt.Printf("%s\n", err.GetErrorCode())
		fmt.Printf("%s\n", err.GetComponentCode())
		fmt.Printf("%s\n", err.GetReasonCode())
		fmt.Printf("%s\n", err.Message())
		// Output:
		// CHA:500 - Error sending message: invoke
		// Caused by: grpc timed out: CON:500 - failed to connect to server
		// CHA:500
		// CHA
		// 500
		// CHA:500 - Error sending message: invoke
		// Caused by: grpc timed out: CON:500 - failed to connect to server
	}
}

// Example_loggingInvalidLevel tests the output for a logging error where
// and an invalid log level has been provided
func Example_loggingInvalidLevel() {
	// when the 'error' module is set to anything but debug, the callstack will
	// not be appended to the error message
	flogging.SetModuleLevel("error", "warning")

	err := ErrorWithCallstack("LOG", "400", "Invalid log level provided - %s", "invalid")

	if err != nil {
		fmt.Printf("%s\n", err.Error())
		fmt.Printf("%s\n", err.GetErrorCode())
		fmt.Printf("%s\n", err.GetComponentCode())
		fmt.Printf("%s\n", err.GetReasonCode())
		fmt.Printf("%s\n", err.Message())
		// Output:
		// LOG:400 - Invalid log level provided - invalid
		// LOG:400
		// LOG
		// 400
		// LOG:400 - Invalid log level provided - invalid
	}
}
