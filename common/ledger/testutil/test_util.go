/*
Copyright IBM Corp. 2016 All Rights Reserved.

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

package testutil

import (
	"crypto/rand"
	"fmt"
	"reflect"
	"runtime"
	"testing"
)

// AssertNil varifies that the value is nil
func AssertNil(t testing.TB, value interface{}) {
	if !isNil(value) {
		t.Fatalf("Value not nil. value=[%#v]\n %s", value, getCallerInfo())
	}
}

// AssertNotNil varifies that the value is not nil
func AssertNotNil(t testing.TB, value interface{}) {
	if isNil(value) {
		t.Fatalf("Values is nil. %s", getCallerInfo())
	}
}

// AssertSame varifies that the two values are same
func AssertSame(t testing.TB, actual interface{}, expected interface{}) {
	t.Logf("%s: AssertSame [%#v] and [%#v]", getCallerInfo(), actual, expected)
	if actual != expected {
		t.Fatalf("Values actual=[%#v] and expected=[%#v] do not point to same object. %s", actual, expected, getCallerInfo())
	}
}

// AssertEquals varifies that the two values are equal
func AssertEquals(t testing.TB, actual interface{}, expected interface{}) {
	t.Logf("%s: AssertEquals [%#v] and [%#v]", getCallerInfo(), actual, expected)
	if expected == nil && isNil(actual) {
		return
	}
	if !reflect.DeepEqual(actual, expected) {
		t.Fatalf("Values are not equal.\n Actual=[%#v], \n Expected=[%#v]\n %s", actual, expected, getCallerInfo())
	}
}

// AssertNotEquals varifies that the two values are not equal
func AssertNotEquals(t testing.TB, actual interface{}, expected interface{}) {
	if reflect.DeepEqual(actual, expected) {
		t.Fatalf("Values are not supposed to be equal. Actual=[%#v], Expected=[%#v]\n %s", actual, expected, getCallerInfo())
	}
}

// AssertError varifies that the err is not nil
func AssertError(t testing.TB, err error, message string) {
	if err == nil {
		t.Fatalf("%s\n %s", message, getCallerInfo())
	}
}

// AssertNoError varifies that the err is nil
func AssertNoError(t testing.TB, err error, message string) {
	if err != nil {
		t.Fatalf("%s - Error: %s\n %s", message, err, getCallerInfo())
	}
}

// AssertContains varifies that the slice contains the value
func AssertContains(t testing.TB, slice interface{}, value interface{}) {
	if reflect.TypeOf(slice).Kind() != reflect.Slice && reflect.TypeOf(slice).Kind() != reflect.Array {
		t.Fatalf("Type of argument 'slice' is expected to be a slice/array, found =[%s]\n %s", reflect.TypeOf(slice), getCallerInfo())
	}

	if !Contains(slice, value) {
		t.Fatalf("Expected value [%s] not found in slice %s\n %s", value, slice, getCallerInfo())
	}
}

// AssertContainsAll varifies that sliceActual is a superset of sliceExpected
func AssertContainsAll(t testing.TB, sliceActual interface{}, sliceExpected interface{}) {
	if reflect.TypeOf(sliceActual).Kind() != reflect.Slice && reflect.TypeOf(sliceActual).Kind() != reflect.Array {
		t.Fatalf("Type of argument 'sliceActual' is expected to be a slice/array, found =[%s]\n %s", reflect.TypeOf(sliceActual), getCallerInfo())
	}

	if reflect.TypeOf(sliceExpected).Kind() != reflect.Slice && reflect.TypeOf(sliceExpected).Kind() != reflect.Array {
		t.Fatalf("Type of argument 'sliceExpected' is expected to be a slice/array, found =[%s]\n %s", reflect.TypeOf(sliceExpected), getCallerInfo())
	}

	array := reflect.ValueOf(sliceExpected)
	for i := 0; i < array.Len(); i++ {
		element := array.Index(i).Interface()
		if !Contains(sliceActual, element) {
			t.Fatalf("Expected value [%s] not found in slice %s\n %s", element, sliceActual, getCallerInfo())
		}
	}
}

// AssertPanic varifies that a panic is raised during a test
func AssertPanic(t testing.TB, msg string) {
	x := recover()
	if x == nil {
		t.Fatal(msg)
	} else {
		t.Logf("A panic was caught successfully. Actual msg = %s", x)
	}
}

// ConstructRandomBytes constructs random bytes of given size
func ConstructRandomBytes(t testing.TB, size int) []byte {
	value := make([]byte, size)
	_, err := rand.Read(value)
	if err != nil {
		t.Fatalf("Error while generating random bytes: %s", err)
	}
	return value
}

// Contains returns true iff the `value` is present in the `slice`
func Contains(slice interface{}, value interface{}) bool {
	array := reflect.ValueOf(slice)
	for i := 0; i < array.Len(); i++ {
		element := array.Index(i).Interface()
		if value == element || reflect.DeepEqual(element, value) {
			return true
		}
	}
	return false
}

func isNil(in interface{}) bool {
	return in == nil || reflect.ValueOf(in).IsNil() || (reflect.TypeOf(in).Kind() == reflect.Slice && reflect.ValueOf(in).Len() == 0)
}

func getCallerInfo() string {
	_, file, line, ok := runtime.Caller(2)
	if !ok {
		return "Could not retrieve caller's info"
	}
	return fmt.Sprintf("CallerInfo = [%s:%d]", file, line)
}
