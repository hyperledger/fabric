/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package gateway

import (
	"testing"

	"github.com/hyperledger/fabric/core/peer"
	"github.com/stretchr/testify/require"
)

func TestPeerAdapter(t *testing.T) {
	t.Run("CommitNotifications", func(t *testing.T) {
		t.Run("returns error when channel does not exist", func(t *testing.T) {
			adapter := &peerAdapter{
				Peer: &peer.Peer{},
			}

			_, err := adapter.CommitNotifications(nil, "CHANNEL")

			require.ErrorContains(t, err, "CHANNEL")
		})
	})

	t.Run("TransactionStatus", func(t *testing.T) {
		t.Run("returns error when channel does not exist", func(t *testing.T) {
			adapter := &peerAdapter{
				Peer: &peer.Peer{},
			}

			_, _, err := adapter.TransactionStatus("CHANNEL", "TX_ID")

			require.ErrorContains(t, err, "CHANNEL")
		})
	})

	t.Run("Ledger", func(t *testing.T) {
		t.Run("returns error when channel does not exist", func(t *testing.T) {
			adapter := &peerAdapter{
				Peer: &peer.Peer{},
			}

			_, err := adapter.Ledger("CHANNEL")

			require.ErrorContains(t, err, "CHANNEL")
		})
	})
}
