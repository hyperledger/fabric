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
var Version string

// package-scoped constants

// Program name
const ProgramName = "orderer"

func GetVersionInfo() string {
	Version = common.Version
	if Version == "" {
		Version = "development build"
	}

	return fmt.Sprintf("%s:\n Version: %s\n Go version: %s\n OS/Arch: %s",
		ProgramName, Version, runtime.Version(),
		fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH))
}
