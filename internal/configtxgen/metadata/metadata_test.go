/*
Copyright 2017 Hitachi America

SPDX-License-Identifier: Apache-2.0
*/

package metadata_test

import (
	"fmt"
	"runtime"
	"testing"

	"github.com/hyperledger/fabric/internal/configtxgen/metadata"
	"github.com/stretchr/testify/require"
)

func TestGetVersionInfo(t *testing.T) {
	testSHAs := []string{"", "abcdefg"}

	for _, sha := range testSHAs {
		metadata.CommitSHA = sha

		expected := fmt.Sprintf("%s:\n Version: %s\n Commit SHA: %s\n Go version: %s\n OS/Arch: %s",
			metadata.ProgramName, metadata.Version, sha, runtime.Version(),
			fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH))
		require.Equal(t, expected, metadata.GetVersionInfo())
	}
}
