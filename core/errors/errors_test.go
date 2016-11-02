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
)

func TestError(t *testing.T) {
	e := Error(Utility, UnknownError)
	s := e.GetStack()
	if s != "" {
		t.Fatalf("No error stack should have been recorded.")
	}
}

// TestErrorWithArg tests creating an error with a message argument
func TestErrorWithArg(t *testing.T) {
	e := Error(Utility, ErrorWithArg, "arg1")
	s := e.GetStack()
	if s != "" {
		t.Fatalf("No error stack should have been recorded.")
	}
}

func TestErrorWithCallstack(t *testing.T) {
	e := ErrorWithCallstack(Utility, UnknownError)
	s := e.GetStack()
	if s == "" {
		t.Fatalf("No error stack was recorded.")
	}
}

// TestErrorWithCallstackAndArg tests creating an error with a callstack and
// message argument
func TestErrorWithCallstackAndArg(t *testing.T) {
	e := ErrorWithCallstack(Utility, ErrorWithArg, "arg1")
	s := e.GetStack()
	if s == "" {
		t.Fatalf("No error stack was recorded.")
	}
}

func oops() CallStackError {
	return Error(Utility, UnknownError)
}

func ExampleError() {
	err := oops()
	if err != nil {
		fmt.Printf("%s\n", err.Error())
		fmt.Printf("%s\n", err.GetErrorCode())
		fmt.Printf("%d\n", err.GetComponentCode())
		fmt.Printf("%d\n", err.GetReasonCode())
		fmt.Printf("%s\n", err.Message())
		fmt.Printf("%s\n", err.MessageIn("en"))
		// Output:
		// An unknown error occurred.
		// 0-0
		// 0
		// 0
		// An unknown error occurred.
		// An unknown error occurred.
	}
}

// ExampleErrorWithArg tests the output for a sample error with a message
// argument
func ExampleErrorWithArg() {
	err := Error(Utility, ErrorWithArg, "arg1")

	if err != nil {
		fmt.Printf("%s\n", err.Error())
		fmt.Printf("%s\n", err.GetErrorCode())
		fmt.Printf("%d\n", err.GetComponentCode())
		fmt.Printf("%d\n", err.GetReasonCode())
		fmt.Printf("%s\n", err.Message())
		fmt.Printf("%s\n", err.MessageIn("en"))
		// Output:
		// An error occurred: arg1
		// 0-1
		// 0
		// 1
		// An error occurred: arg1
		// An error occurred: arg1
	}
}
