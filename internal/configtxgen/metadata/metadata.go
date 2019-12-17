/*
Copyright 2017 Hitachi America

SPDX-License-Identifier: Apache-2.0
*/

package metadata

import (
	"fmt"
	"runtime"

	"github.com/hyperledger/fabric/common/metadata"
)

const ProgramName = "configtxgen"

var (
	CommitSHA = metadata.CommitSHA
	Version   = metadata.Version
)

func GetVersionInfo() string {
	return fmt.Sprintf("%s:\n Version: %s\n Commit SHA: %s\n Go version: %s\n OS/Arch: %s",
		ProgramName, Version, CommitSHA, runtime.Version(),
		fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH))
}
