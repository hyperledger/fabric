// +build !windows

/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package node

import (
	"os"
	"syscall"

	"github.com/hyperledger/fabric/common/diag"
)

func addPlatformSignals(sigs map[os.Signal]func()) map[os.Signal]func() {
	sigs[syscall.SIGUSR1] = func() { diag.LogGoRoutines(logger.Named("diag")) }
	return sigs
}
