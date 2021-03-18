/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package commit

import (
	"testing"

	"github.com/hyperledger/fabric/core/peer"
	"github.com/stretchr/testify/require"
)

func TestPeerCommitAdapter(t *testing.T) {
	t.Run("CommitNotifications", func(t *testing.T) {
		t.Run("returns error when channel does not exist", func(t *testing.T) {
			adapter := &PeerAdapter{
				Peer: &peer.Peer{},
			}

			_, err := adapter.CommitNotifications(nil, "CHANNEL")

			require.ErrorContains(t, err, "CHANNEL")
		})
	})

	t.Run("TransactionStatus", func(t *testing.T) {
		t.Run("returns error when channel does not exist", func(t *testing.T) {
			adapter := &PeerAdapter{
				Peer: &peer.Peer{},
			}

			_, err := adapter.TransactionStatus("CHANNEL", "TX_ID")

			require.ErrorContains(t, err, "CHANNEL")
		})
	})
}
