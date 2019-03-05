/*
Copyright 2017 Hitachi America

SPDX-License-Identifier: Apache-2.0
*/

package metadata

import (
	"fmt"
	"runtime"
)

// Package version
const Version = "2.0.0"

var CommitSHA string

// Program name
const ProgramName = "configtxgen"

func GetVersionInfo() string {
	if CommitSHA == "" {
		CommitSHA = "development build"
	}

	return fmt.Sprintf("%s:\n Version: %s\n Commit SHA: %s\n Go version: %s\n OS/Arch: %s",
		ProgramName, Version, CommitSHA, runtime.Version(),
		fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH))
}
