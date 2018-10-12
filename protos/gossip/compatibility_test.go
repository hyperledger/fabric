/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package gossip

import (
	"encoding/hex"
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/stretchr/testify/assert"
)

var digestMsg = &GossipMessage{
	Channel: []byte("mychannel"),
	Content: &GossipMessage_DataDig{
		DataDig: &DataDigest{
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

var requestMsg = &GossipMessage{
	Channel: []byte("mychannel"),
	Content: &GossipMessage_DataReq{
		DataReq: &DataRequest{
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
	for msgBytes, expectedMsg := range map[string]*GossipMessage{
		v12DataDigestBytes:  digestMsg,
		v12DataRequestBytes: requestMsg,
	} {
		var err error
		v13Envelope := &Envelope{}
		v13Envelope.Payload, err = hex.DecodeString(msgBytes)
		assert.NoError(t, err)
		sMsg := &SignedGossipMessage{
			Envelope: v13Envelope,
		}
		v13Digest, err := sMsg.ToGossipMessage()
		assert.NoError(t, err)
		assert.True(t, proto.Equal(expectedMsg, v13Digest.GossipMessage))
	}
}
