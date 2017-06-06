/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package metadata_test

import (
	"fmt"
	"runtime"
	"testing"

	"github.com/hyperledger/fabric/common/tools/cryptogen/metadata"
	"github.com/stretchr/testify/assert"
)

func TestGetVersionInfo(t *testing.T) {
	testVersion := "TestVersion"
	metadata.Version = testVersion

	expected := fmt.Sprintf("%s:\n Version: %s\n Go version: %s\n OS/Arch: %s",
		metadata.ProgramName, testVersion, runtime.Version(),
		fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH))
	assert.Equal(t, expected, metadata.GetVersionInfo())
}
