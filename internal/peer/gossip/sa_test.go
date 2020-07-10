/*
Copyright IBM Corp. 2017 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0

*/

package gossip

import (
	"testing"

	"github.com/hyperledger/fabric/internal/peer/gossip/mocks"
	"github.com/hyperledger/fabric/msp"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestMspSecurityAdvisor_OrgByPeerIdentity(t *testing.T) {
	dm := &mocks.DeserializersManager{
		LocalDeserializer: &mocks.IdentityDeserializer{Identity: []byte("Alice"), Msg: []byte("msg1"), Mock: mock.Mock{}},
		ChannelDeserializers: map[string]msp.IdentityDeserializer{
			"A": &mocks.IdentityDeserializer{Identity: []byte("Bob"), Msg: []byte("msg2"), Mock: mock.Mock{}},
		},
	}

	advisor := NewSecurityAdvisor(dm)
	require.NotNil(t, advisor.OrgByPeerIdentity([]byte("Alice")))
	require.NotNil(t, advisor.OrgByPeerIdentity([]byte("Bob")))
	require.Nil(t, advisor.OrgByPeerIdentity([]byte("Charlie")))
	require.Nil(t, advisor.OrgByPeerIdentity(nil))
}
