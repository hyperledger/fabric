/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package channel

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestInitCmdFactory(t *testing.T) {
	t.Run("InitCmdFactory() with PeerDeliverRequired and OrdererRequired", func(t *testing.T) {
		cf, err := InitCmdFactory(EndorserRequired, PeerDeliverRequired, OrdererRequired)
		require.Nil(t, cf)
		require.Error(t, err)
		require.Contains(t, err.Error(), "ERROR - only a single deliver source is currently supported")
	})
}
