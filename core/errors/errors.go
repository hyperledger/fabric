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
	"bytes"
	"fmt"
	"runtime"
	"strings"

	"github.com/hyperledger/fabric/common/flogging"
	logging "github.com/op/go-logging"
)

// MaxCallStackLength is the maximum length of the stored call stack
const MaxCallStackLength = 30

var errorLogger = logging.MustGetLogger("error")

// CallStackError is a general interface for
// Fabric errors
type CallStackError interface {
	error
	GetStack() string
	GetErrorCode() string
	GetComponentCode() string
	GetReasonCode() string
	Message() string
}

type callstack []uintptr

// the main idea is to have an error package
// HLError is the 'super class' of all errors
// It has a predefined, general error message
// One has to create his own error in order to
// create something more useful
type hlError struct {
	stack         callstack
	componentcode string
	reasoncode    string
	message       string
	args          []interface{}
	stackGetter   func(callstack) string
}

// newHLError creates a general HL error with a predefined message
// and a stacktrace.
func newHLError(debug bool) *hlError {
	e := &hlError{}
	setupHLError(e, debug)
	return e
}

func setupHLError(e *hlError, debug bool) {
	e.componentcode = "UTILITY"
	e.reasoncode = "UNKNOWNERROR"
	e.message = "An unknown error occurred."
	if !debug {
		e.stackGetter = noopGetStack
		return
	}
	e.stackGetter = getStack
	stack := make([]uintptr, MaxCallStackLength)
	skipCallersAndSetupHL := 2
	length := runtime.Callers(skipCallersAndSetupHL, stack[:])
	e.stack = stack[:length]
}

// Error comes from the error interface
func (h *hlError) Error() string {
	return h.Message()
}

// GetStack returns the call stack as a string
func (h *hlError) GetStack() string {
	return h.stackGetter(h.stack)
}

// GetComponentCode returns the component name
func (h *hlError) GetComponentCode() string {
	return h.componentcode
}

// GetReasonCode returns the reason code - i.e. why the error occurred
func (h *hlError) GetReasonCode() string {
	return h.reasoncode
}

// GetErrorCode returns a formatted error code string
func (h *hlError) GetErrorCode() string {
	return fmt.Sprintf("%s_%s", h.componentcode, h.reasoncode)
}

// Message returns the corresponding error message for this error in default
// language.
func (h *hlError) Message() string {
	message := h.GetErrorCode() + " - " + fmt.Sprintf(h.message, h.args...)

	// check that the error has a callstack before proceeding
	if h.GetStack() != "" {
		// initialize logging level for errors from core.yaml. it can also be set
		// for code running on the peer dynamically via CLI using
		// "peer logging setlevel error <log-level>"
		errorLogLevelString, _ := flogging.GetModuleLevel("error")

		if errorLogLevelString == logging.DEBUG.String() {
			message = appendCallStack(message, h.GetStack())
		}
	}

	return message
}

func appendCallStack(message string, callstack string) string {
	messageWithCallStack := message + "\n" + callstack

	return messageWithCallStack
}

// Error creates a CallStackError using a specific Component Code and
// Reason Code (no callstack is recorded)
func Error(componentcode string, reasoncode string, message string, args ...interface{}) CallStackError {
	return newCustomError(componentcode, reasoncode, message, false, args...)
}

// ErrorWithCallstack creates a CallStackError using a specific Component Code and
// Reason Code and fills its callstack
func ErrorWithCallstack(componentcode string, reasoncode string, message string, args ...interface{}) CallStackError {
	return newCustomError(componentcode, reasoncode, message, true, args...)
}

func newCustomError(componentcode string, reasoncode string, message string, generateStack bool, args ...interface{}) CallStackError {
	e := &hlError{}
	setupHLError(e, generateStack)
	if componentcode != "" {
		e.componentcode = strings.ToUpper(componentcode)
	}
	if reasoncode != "" {
		e.reasoncode = strings.ToUpper(reasoncode)
	}
	if message != "" {
		e.message = message
	}
	e.args = args
	return e
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
	for _, pc := range stack {
		f := runtime.FuncForPC(pc)
		file, line := f.FileLine(pc)
		buf.WriteString(fmt.Sprintf("%s:%d %s\n", file, line, f.Name()))
	}

	return fmt.Sprintf("%s", buf.Bytes())
}

func noopGetStack(stack callstack) string {
	return ""
}
