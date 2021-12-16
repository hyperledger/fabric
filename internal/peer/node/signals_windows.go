//go:build windows
// +build windows

/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package node

import (
	"os"
)

func addPlatformSignals(sigs map[os.Signal]func()) map[os.Signal]func() {
	return sigs
}
