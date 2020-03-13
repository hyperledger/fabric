/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package idemix

import "log"

// logger is used to log debug information.
var logger Logger = LogFunc(log.Printf)

// SetLogger sets the logger instance used for debug and error reporting.  The
// logger reference is not mutex-protected so this must be set before calling
// any other library functions.
//
// If a custom logger is not defined, the global logger from the standard
// library's log package is used.
func SetLogger(l Logger) {
	logger = l
}

// Logger defines the contract for logging. This interface is explicitly
// defined to be compatible with the logger in the standard library log
// package.
type Logger interface {
	Printf(format string, a ...interface{})
}

// LogFunc is a function adapter for logging.
type LogFunc func(format string, a ...interface{})

// Printf is used to create a formatted string log record.
func (l LogFunc) Printf(format string, a ...interface{}) {
	l(format, a...)
}
