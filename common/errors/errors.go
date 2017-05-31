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
	"bytes"
	"fmt"
	"regexp"
	"runtime"
)

// MaxCallStackLength is the maximum length of the stored call stack
const MaxCallStackLength = 30

var (
	componentPattern = "[A-Za-z]{3}"
	reasonPattern    = "[0-9]{3}"
)

// CallStackError is a general interface for Fabric errors
type CallStackError interface {
	error
	GetStack() string
	GetErrorCode() string
	GetComponentCode() string
	GetReasonCode() string
	Message() string
	GenerateStack(bool) CallStackError
	WrapError(error) CallStackError
}

type callstack []uintptr

// callError is the 'super class' of all errors
type callError struct {
	stack         callstack
	componentcode string
	reasoncode    string
	message       string
	args          []interface{}
	stackGetter   func(callstack) string
	prevErr       error
}

func setupCallError(e *callError, generateStack bool) {
	if !generateStack {
		e.stackGetter = noopGetStack
		return
	}
	e.stackGetter = getStack
	stack := make([]uintptr, MaxCallStackLength)
	skipCallersAndSetup := 2
	length := runtime.Callers(skipCallersAndSetup, stack[:])
	e.stack = stack[:length]
}

// Error comes from the error interface - it returns the error message and
// appends the callstack, if available
func (e *callError) Error() string {
	message := e.GetErrorCode() + " - " + fmt.Sprintf(e.message, e.args...)
	// check that the error has a callstack before proceeding
	if e.GetStack() != "" {
		message = appendCallStack(message, e.GetStack())
	}
	if e.prevErr != nil {
		message += "\nCaused by: " + e.prevErr.Error()
	}
	return message
}

// GetStack returns the call stack as a string
func (e *callError) GetStack() string {
	return e.stackGetter(e.stack)
}

// GetComponentCode returns the component name
func (e *callError) GetComponentCode() string {
	return e.componentcode
}

// GetReasonCode returns the reason code - i.e. why the error occurred
func (e *callError) GetReasonCode() string {
	return e.reasoncode
}

// GetErrorCode returns a formatted error code string
func (e *callError) GetErrorCode() string {
	return fmt.Sprintf("%s:%s", e.componentcode, e.reasoncode)
}

// Message returns the corresponding error message for this error in default
// language.
func (e *callError) Message() string {
	message := e.GetErrorCode() + " - " + fmt.Sprintf(e.message, e.args...)

	if e.prevErr != nil {
		switch previousError := e.prevErr.(type) {
		case CallStackError:
			message += "\nCaused by: " + previousError.Message()
		default:
			message += "\nCaused by: " + e.prevErr.Error()
		}
	}
	return message
}

func appendCallStack(message string, callstack string) string {
	return message + "\n" + callstack
}

// Error creates a CallStackError using a specific component code and reason
// code (no callstack is generated)
func Error(componentcode string, reasoncode string, message string, args ...interface{}) CallStackError {
	return newError(componentcode, reasoncode, message, args...).GenerateStack(false)
}

// ErrorWithCallstack creates a CallStackError using a specific component code
// and reason code and generates its callstack
func ErrorWithCallstack(componentcode string, reasoncode string, message string, args ...interface{}) CallStackError {
	return newError(componentcode, reasoncode, message, args...).GenerateStack(true)
}

func newError(componentcode string, reasoncode string, message string, args ...interface{}) CallStackError {
	e := &callError{}
	e.setErrorFields(componentcode, reasoncode, message, args...)
	return e
}

// GenerateStack generates the callstack for a CallStackError
func (e *callError) GenerateStack(flag bool) CallStackError {
	setupCallError(e, flag)
	return e
}

// WrapError wraps a previous error into a CallStackError
func (e *callError) WrapError(prevErr error) CallStackError {
	e.prevErr = prevErr
	return e
}

func (e *callError) setErrorFields(componentcode string, reasoncode string, message string, args ...interface{}) {
	if isValidComponentOrReasonCode(componentcode, componentPattern) {
		e.componentcode = componentcode
	}
	if isValidComponentOrReasonCode(reasoncode, reasonPattern) {
		e.reasoncode = reasoncode
	}
	if message != "" {
		e.message = message
	}
	e.args = args
}

func isValidComponentOrReasonCode(componentOrReasonCode string, regExp string) bool {
	if componentOrReasonCode == "" {
		return false
	}
	re, _ := regexp.Compile(regExp)
	matched := re.FindString(componentOrReasonCode)
	if len(matched) != len(componentOrReasonCode) {
		return false
	}
	return true
}

func getStack(stack callstack) string {
	buf := bytes.Buffer{}
	if stack == nil {
		return fmt.Sprintf("No call stack available")
	}
	// this removes the core/errors module calls from the callstack because they
	// are not useful for debugging
	const firstNonErrorModuleCall int = 2
	stack = stack[firstNonErrorModuleCall:]
	for i, pc := range stack {
		f := runtime.FuncForPC(pc)
		file, line := f.FileLine(pc)
		if i != len(stack)-1 {
			buf.WriteString(fmt.Sprintf("%s:%d %s\n", file, line, f.Name()))
		} else {
			buf.WriteString(fmt.Sprintf("%s:%d %s", file, line, f.Name()))
		}
	}
	return fmt.Sprintf("%s", buf.Bytes())
}

func noopGetStack(stack callstack) string {
	return ""
}
