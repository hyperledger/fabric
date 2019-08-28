/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

// Package customlogger is just a package within the module.
package customlogger

import "fmt"

func Logf(msg string, args ...interface{}) {
	fmt.Printf(msg, args...)
}
