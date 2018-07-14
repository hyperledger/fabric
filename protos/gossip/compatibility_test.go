/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package gossip

import (
	"io/ioutil"
	"testing"

	"encoding/hex"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/protos/testutils"
	"github.com/stretchr/testify/assert"
)

var digestMsg = &GossipMessage{
	Channel: []byte("mychannel"),
	Content: &GossipMessage_DataDig{
		DataDig: &DataDigest{
			Digests: []string{
				string([]byte{255}),
				string([]byte{255, 255}),
				string([]byte{255, 255, 255}),
				string([]byte{255, 255, 255, 255}),
				"100",
			},
		},
	},
}

var requestMsg = &GossipMessage{
	Channel: []byte("mychannel"),
	Content: &GossipMessage_DataReq{
		DataReq: &DataRequest{
			Digests: []string{
				string([]byte{255}),
				string([]byte{255, 255}),
				string([]byte{255, 255, 255}),
				string([]byte{255, 255, 255, 255}),
				"100",
			},
		},
	},
}

const (
	v13DataDigestBytes  = "12096d796368616e6e656c52171201ff1202ffff1203ffffff1204ffffffff1203313030"
	v13DataRequestBytes = "12096d796368616e6e656c5a171201ff1202ffff1203ffffff1204ffffffff1203313030"
)

func TestUnmarshalV13Digests(t *testing.T) {
	// This test ensures that digests of data digest messages and data requests
	// that originated from fabric v1.3 can be successfully parsed by v1.2
	for msgBytes, expectedMsg := range map[string]*GossipMessage{
		v13DataDigestBytes:  digestMsg,
		v13DataRequestBytes: requestMsg,
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

func TestCreateV12Digests(t *testing.T) {
	t.Skip()
	for fileName, msg := range map[string]*GossipMessage{
		"dataReq.v12.pb": requestMsg,
		"dataDig.v12.pb": digestMsg,
	} {
		sMsg, err := msg.NoopSign()
		assert.NoError(t, err)
		err = ioutil.WriteFile(fileName, sMsg.Envelope.Payload, 0600)
		assert.NoError(t, err)
	}
}

func TestStringsEncodedAsBytes(t *testing.T) {
	// This test ensures that strings and bytes are
	// interchangeable
	t.Run("String interpreted as Bytes", func(t *testing.T) {
		str := &testutils.String{
			Str: "some string",
		}
		b, err := proto.Marshal(str)
		assert.NoError(t, err)
		bytes := &testutils.Bytes{}
		err = proto.Unmarshal(b, bytes)
		assert.NoError(t, err)
		assert.Equal(t, "some string", string(bytes.B))
	})

	t.Run("Bytes interpreted as String", func(t *testing.T) {
		bytes := &testutils.Bytes{
			B: []byte("some string"),
		}
		b, err := proto.Marshal(bytes)
		assert.NoError(t, err)
		str := &testutils.String{}
		err = proto.Unmarshal(b, str)
		assert.NoError(t, err)
		assert.Equal(t, "some string", str.Str)
	})
}
