/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package ledger

import (
	"testing"

	"github.com/hyperledger/fabric/core/peer"
	"github.com/stretchr/testify/require"
)

//go:generate counterfeiter -o mocks/ledger.go --fake-name Ledger . Ledger
//go:generate counterfeiter -o mocks/provider.go --fake-name Provider . Provider

func TestPeerAdapter(t *testing.T) {
	t.Run("Ledger", func(t *testing.T) {
		t.Run("returns error when channel does not exist", func(t *testing.T) {
			adapter := &PeerAdapter{
				Peer: &peer.Peer{},
			}

			_, err := adapter.Ledger("CHANNEL")

			require.ErrorContains(t, err, "CHANNEL")
		})
	})
}
