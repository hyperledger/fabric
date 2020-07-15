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

func TestPauseCmd(t *testing.T) {
	t.Run("when the channelID is not supplied", func(t *testing.T) {
		cmd := pauseCmd()
		args := []string{}
		cmd.SetArgs(args)
		err := cmd.Execute()
		require.EqualError(t, err, "Must supply channel ID")
	})

	t.Run("when the specified channelID does not exist", func(t *testing.T) {
		testPath := "/tmp/hyperledger/test"
		os.RemoveAll(testPath)
		viper.Set("peer.fileSystemPath", testPath)
		defer os.RemoveAll(testPath)

		cmd := pauseCmd()
		args := []string{"-c", "ch_p"}
		cmd.SetArgs(args)
		err := cmd.Execute()
		require.EqualError(t, err, "cannot update ledger status, ledger [ch_p] does not exist")
	})
}
