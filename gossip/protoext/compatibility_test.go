/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package protoext_test

import (
	"encoding/hex"
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric-protos-go/gossip"
	"github.com/hyperledger/fabric/gossip/protoext"
	"github.com/stretchr/testify/require"
)

var digestMsg = &gossip.GossipMessage{
	Channel: []byte("mychannel"),
	Content: &gossip.GossipMessage_DataDig{
		DataDig: &gossip.DataDigest{
			Digests: [][]byte{
				{255},
				{255, 255},
				{255, 255, 255},
				{255, 255, 255, 255},
				[]byte("100"),
			},
		},
	},
}

var requestMsg = &gossip.GossipMessage{
	Channel: []byte("mychannel"),
	Content: &gossip.GossipMessage_DataReq{
		DataReq: &gossip.DataRequest{
			Digests: [][]byte{
				{255},
				{255, 255},
				{255, 255, 255},
				{255, 255, 255, 255},
				[]byte("100"),
			},
		},
	},
}

const (
	v12DataDigestBytes  = "12096d796368616e6e656c52171201ff1202ffff1203ffffff1204ffffffff1203313030"
	v12DataRequestBytes = "12096d796368616e6e656c5a171201ff1202ffff1203ffffff1204ffffffff1203313030"
)

func TestUnmarshalV12Digests(t *testing.T) {
	// This test ensures that digests of data digest messages and data requests
	// that originated from fabric v1.3 can be successfully parsed by v1.2
	for msgBytes, expectedMsg := range map[string]*gossip.GossipMessage{
		v12DataDigestBytes:  digestMsg,
		v12DataRequestBytes: requestMsg,
	} {
		var err error
		v13Envelope := &gossip.Envelope{}
		v13Envelope.Payload, err = hex.DecodeString(msgBytes)
		require.NoError(t, err)
		v13Digest, err := protoext.EnvelopeToGossipMessage(v13Envelope)
		require.NoError(t, err)
		require.True(t, proto.Equal(expectedMsg, v13Digest.GossipMessage))
	}
}
