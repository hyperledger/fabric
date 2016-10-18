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

package api

import (
	"fmt"

	cfssl "github.com/cloudflare/cfssl/errors"
)

// The following are all the error codes returned by COP.
// The values begin with "100000" to avoid overlap with CFSSL errors.
// Add all new errors to the end of the current list to preserver backwards compatibility.
const (
	// NotImplemented means not yet implemented but plans to support
	NotImplemented int = 100000 + iota
	// NotSupported means no current plans to support
	NotSupported
	TooManyArgs
	InvalidProviderName
	ErrorReadingFile
	InvalidConfig
	NotInitialized
	CFSSL
	Output
	Input
)

// Error is an interface with a Code method
type Error interface {
	error
	// Code returns the specific error code
	Code() int
}

// ErrorImpl is the implementation of the Error interface
type ErrorImpl struct {
	ErrorCode int    `json:"code"`
	Message   string `json:"message"`
}

// Error implements the error interface
func (e *ErrorImpl) Error() string {
	return fmt.Sprintf("%d: %s", e.ErrorCode, e.Message)
}

// Code implements the error interface
func (e *ErrorImpl) Code() int {
	return e.ErrorCode
}

// NewError constructor for COP errors
func NewError(code int, format string, args ...interface{}) *ErrorImpl {
	msg := fmt.Sprintf(format, args)
	return &ErrorImpl{ErrorCode: code, Message: msg}
}

// WrapError another COP error
func WrapError(err error, code int, format string, args ...interface{}) *ErrorImpl {
	msg := fmt.Sprintf(format, args)
	msg = fmt.Sprintf("%s [%s]", msg, err.Error())
	return &ErrorImpl{ErrorCode: code, Message: msg}
}

// WrapCFSSLError wraps a CFSSL error
func WrapCFSSLError(error *cfssl.Error, code int, format string, args ...interface{}) *ErrorImpl {
	msg := fmt.Sprintf(format, args)
	msg = fmt.Sprintf("%s [CFSSL %d: %s]", msg, error.ErrorCode, error.Message)
	return &ErrorImpl{ErrorCode: code, Message: msg}
}
