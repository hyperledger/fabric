/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package node

import (
	"os"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
)

func TestUpgradeDBsCmd(t *testing.T) {
	testPath := "/tmp/hyperledger/test"
	os.RemoveAll(testPath)
	viper.Set("peer.fileSystemPath", testPath)
	defer os.RemoveAll(testPath)

	cmd := upgradeDBsCmd()
	require.EqualError(t, cmd.Execute(), "the data format is already up to date. No upgrade is required")
}
