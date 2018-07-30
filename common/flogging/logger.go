/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package flogging

type Logger interface {
	Critical(args ...interface{})
	Criticalf(format string, args ...interface{})
	Debug(args ...interface{})
	Debugf(format string, args ...interface{})
	Error(args ...interface{})
	Errorf(format string, args ...interface{})
	Fatal(args ...interface{})
	Fatalf(format string, args ...interface{})
	Info(args ...interface{})
	Infof(format string, args ...interface{})
	Notice(args ...interface{})
	Noticef(format string, args ...interface{})
	Panic(args ...interface{})
	Panicf(format string, args ...interface{})
	Warning(args ...interface{})
	Warningf(format string, args ...interface{})
}
