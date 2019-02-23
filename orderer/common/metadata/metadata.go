/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package metadata

import (
	"fmt"
	"runtime"

	common "github.com/hyperledger/fabric/common/metadata"
)

// package-scoped variables

// Package version
var Version string = common.Version

// package-scoped constants

// Program name
const ProgramName = "orderer"

func GetVersionInfo() string {
	return fmt.Sprintf(
		"%s:\n Version: %s\n Commit SHA: %s\n Go version: %s\n OS/Arch: %s\n",
		ProgramName,
		Version,
		common.CommitSHA,
		runtime.Version(),
		fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH),
	)
}
