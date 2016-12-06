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
	"encoding/json"
	"fmt"
	"runtime"

	"github.com/hyperledger/fabric/flogging"
	logging "github.com/op/go-logging"
)

// MaxCallStackLength is the maximum length of the stored call stack
const MaxCallStackLength = 30

// ComponentCode shows the originating component/module
type ComponentCode string

// ReasonCode for low level error description
type ReasonCode string

var errorLogger = logging.MustGetLogger("error")

// CallStackError is a general interface for
// Fabric errors
type CallStackError interface {
	error
	GetStack() string
	GetErrorCode() string
	GetComponentCode() ComponentCode
	GetReasonCode() ReasonCode
	Message() string
	MessageIn(string) string
}

type errormap map[string]map[string]map[string]string

var emap errormap

const language string = "en"

func init() {
	initErrors()
}

func initErrors() {
	e := json.Unmarshal([]byte(errorMapping), &emap)
	if e != nil {
		panic(e)
	}
}

type callstack []uintptr

// the main idea is to have an error package
// HLError is the 'super class' of all errors
// It has a predefined, general error message
// One has to create his own error in order to
// create something more useful
type hlError struct {
	stack         callstack
	componentcode ComponentCode
	reasoncode    ReasonCode
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
	e.componentcode = Utility
	e.reasoncode = UtilityUnknownError
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

// GetComponentCode returns the Return code
func (h *hlError) GetComponentCode() ComponentCode {
	return h.componentcode
}

// GetReasonCode returns the Reason code
func (h *hlError) GetReasonCode() ReasonCode {
	return h.reasoncode
}

// GetErrorCode returns a formatted error code string
func (h *hlError) GetErrorCode() string {
	return fmt.Sprintf("%s-%s", h.componentcode, h.reasoncode)
}

// Message returns the corresponding error message for this error in default
// language.
// TODO - figure out the best way to read in system language instead of using
// hard-coded default language
func (h *hlError) Message() string {
	// initialize logging level for errors from core.yaml. it can also be set
	// for code running on the peer dynamically via CLI using
	// "peer logging setlevel error <log-level>"
	errorLogLevelString, _ := flogging.GetModuleLogLevel("error")
	if errorLogLevelString == logging.DEBUG.String() {
		messageWithCallStack := fmt.Sprintf(emap[fmt.Sprintf("%s", h.componentcode)][fmt.Sprintf("%s", h.reasoncode)][language], h.args...) + "\n" + h.GetStack()
		return messageWithCallStack
	}
	return fmt.Sprintf(emap[fmt.Sprintf("%s", h.componentcode)][fmt.Sprintf("%s", h.reasoncode)][language], h.args...)
}

// MessageIn returns the corresponding error message for this error in 'language'
func (h *hlError) MessageIn(language string) string {
	return fmt.Sprintf(emap[fmt.Sprintf("%s", h.componentcode)][fmt.Sprintf("%s", h.reasoncode)][language], h.args...)
}

// Error creates a CallStackError using a specific Component Code and
// Reason Code (no callstack is recorded)
func Error(componentcode ComponentCode, reasoncode ReasonCode, args ...interface{}) CallStackError {
	return newCustomError(componentcode, reasoncode, false, args...)
}

// ErrorWithCallstack creates a CallStackError using a specific Component Code and
// Reason Code and fills its callstack
func ErrorWithCallstack(componentcode ComponentCode, reasoncode ReasonCode, args ...interface{}) CallStackError {
	return newCustomError(componentcode, reasoncode, true, args...)
}

func newCustomError(componentcode ComponentCode, reasoncode ReasonCode, generateStack bool, args ...interface{}) CallStackError {
	e := &hlError{}
	setupHLError(e, generateStack)
	e.componentcode = componentcode
	e.reasoncode = reasoncode
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
