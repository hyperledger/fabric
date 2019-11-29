/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package metadata

import (
	"fmt"
	"runtime"

	"github.com/hyperledger/fabric/common/metadata"
)

const ProgramName = "orderer"

var Version = metadata.Version

func GetVersionInfo() string {
	return fmt.Sprintf(
		"%s:\n Version: %s\n Commit SHA: %s\n Go version: %s\n OS/Arch: %s\n",
		ProgramName,
		Version,
		metadata.CommitSHA,
		runtime.Version(),
		fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH),
	)
}
