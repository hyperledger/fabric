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

func TestUnjoinCmd(t *testing.T) {
	t.Run("when the channelID is not specified", func(t *testing.T) {
		cmd := unjoinCmd()
		args := []string{}
		cmd.SetArgs(args)
		err := cmd.Execute()
		require.EqualError(t, err, "Must supply channel ID")
	})

	t.Run("when the channel does not exist", func(t *testing.T) {
		testPath := "/tmp/hyperledger/test"
		os.RemoveAll(testPath)
		viper.Set("peer.fileSystemPath", testPath)
		defer os.RemoveAll(testPath)

		cmd := unjoinCmd()
		cmd.SetArgs([]string{"-c", "invalid_channel_does_not_exist"})
		err := cmd.Execute()
		require.EqualError(t, err, "unjoin channel [invalid_channel_does_not_exist]: cannot update ledger status, ledger [invalid_channel_does_not_exist] does not exist")
	})
}
