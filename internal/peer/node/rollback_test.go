/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package node

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRollbackCmd(t *testing.T) {
	t.Run("when the channelID is not supplied", func(t *testing.T) {
		cmd := rollbackCmd()
		args := []string{}
		cmd.SetArgs(args)
		err := cmd.Execute()
		assert.Equal(t, "Must supply channel ID", err.Error())
	})

	t.Run("when the specified channelID does not exist", func(t *testing.T) {
		cmd := rollbackCmd()
		args := []string{"-c", "ch1", "-b", "10"}
		cmd.SetArgs(args)
		err := cmd.Execute()
		expectedErr := "ledgerID [ch1] does not exist"
		assert.Equal(t, expectedErr, err.Error())
	})
}
