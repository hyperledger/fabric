/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package metadata_test

import (
	"fmt"
	"runtime"
	"testing"

	common "github.com/hyperledger/fabric/common/metadata"
	"github.com/hyperledger/fabric/orderer/metadata"
	"github.com/stretchr/testify/assert"
)

func TestGetVersionInfo(t *testing.T) {
	// This test would always fail for development versions because if
	// common.Version is not set, the string returned is "development version"
	// Set it here for this test to avoid this.
	if common.Version == "" {
		common.Version = "testVersion"
	}

	expected := fmt.Sprintf("%s:\n Version: %s\n Go version: %s\n OS/Arch: %s",
		metadata.ProgramName, common.Version, runtime.Version(),
		fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH))
	assert.Equal(t, expected, metadata.GetVersionInfo())
}
