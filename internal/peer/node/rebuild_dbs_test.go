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

func TestRebuildDBsCmd(t *testing.T) {
	testPath := "/tmp/hyperledger/test"
	os.RemoveAll(testPath)
	viper.Set("peer.fileSystemPath", testPath)
	defer os.RemoveAll(testPath)

	// this should return an error as no ledger has been set up
	cmd := rebuildDBsCmd()
	err := cmd.Execute()
	// this should return an error as no ledger has been set up
	require.Contains(t, err.Error(), "error while checking if any ledger has been bootstrapped from snapshot")
}
