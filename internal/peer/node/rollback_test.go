/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package node

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRollbackCmd(t *testing.T) {
	t.Run("when the channelID is not supplied", func(t *testing.T) {
		cmd := rollbackCmd()
		args := []string{}
		cmd.SetArgs(args)
		err := cmd.Execute()
		require.Equal(t, "Must supply channel ID", err.Error())
	})

	t.Run("when the specified channelID does not exist", func(t *testing.T) {
		cmd := rollbackCmd()
		args := []string{"-c", "ch1", "-b", "10"}
		cmd.SetArgs(args)
		err := cmd.Execute()
		// this should return an error as no ledger has been set up
		require.Contains(t, err.Error(), "error while checking if any ledger has been bootstrapped from snapshot")
	})
}
