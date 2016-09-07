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
)

// MaxCallStackLength is the maximum length of the stored call stack
const MaxCallStackLength = 30

// ComponentCode shows the originating component/module
type ComponentCode uint

// ReasonCode for low level error description
type ReasonCode uint

// Return codes
const (
	Utility ComponentCode = iota
)

// Result codes
const (
	// Placeholder
	UnknownError ReasonCode = iota
)

// CallStackError is a general interface for
// Fabric errors
type CallStackError interface {
	error
	GetStack() string
	GetErrorCode() string
	GetComponentCode() ComponentCode
	GetReasonCode() ReasonCode
}

type errormap map[string]map[string]map[string]string

var emap errormap

const language string = "en"

func init() {
	initErrors()
}

func initErrors() {
	e := json.Unmarshal([]byte(errorCodes), &emap)
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
	e.reasoncode = UnknownError
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
	return h.componentcode.Message(h.reasoncode)
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
	return fmt.Sprintf("%d-%d", h.componentcode, h.reasoncode)
}

// Message returns the corresponding error message for this code in default language
func (c ComponentCode) Message(reasoncode ReasonCode) string {
	return emap[fmt.Sprintf("%d", c)][fmt.Sprintf("%d", reasoncode)][language]
}

// MessageIn returns the corresponding error message for this code in 'language'
func (c ComponentCode) MessageIn(reasoncode ReasonCode, language string) string {
	return emap[fmt.Sprintf("%d", c)][fmt.Sprintf("%d", reasoncode)][language]
}

// Error creates a CallStackError using a specific Component Code and
// Reason Code (no callstack is recorded)
func Error(componentcode ComponentCode, reasoncode ReasonCode) CallStackError {
	return newCustomError(componentcode, reasoncode, false)
}

// ErrorWithCallstack creates a CallStackError using a specific Component Code and
// Reason Code and fills its callstack
func ErrorWithCallstack(componentcode ComponentCode, reasoncode ReasonCode) CallStackError {
	return newCustomError(componentcode, reasoncode, true)
}

func newCustomError(componentcode ComponentCode, reasoncode ReasonCode, generateStack bool) CallStackError {
	e := &hlError{}
	setupHLError(e, generateStack)
	e.componentcode = componentcode
	e.reasoncode = reasoncode
	return e
}

func getStack(stack callstack) string {
	buf := bytes.Buffer{}
	if stack == nil {
		return fmt.Sprintf("No call stack available")
	}
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
