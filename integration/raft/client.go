/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package raft

import (
	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric-protos-go/orderer"
	"github.com/hyperledger/fabric/cmd/common/signer"
	"github.com/hyperledger/fabric/integration/nwo"
	"github.com/hyperledger/fabric/protoutil"
	. "github.com/onsi/gomega"
)

func FetchBlock(n *nwo.Network, o *nwo.Orderer, seq uint64, channel string) *common.Block {
	denv := CreateDeliverEnvelope(n, o, seq, channel)
	Expect(denv).NotTo(BeNil())

	var blk *common.Block
	Eventually(func() error {
		var err error
		blk, err = nwo.Deliver(n, o, denv)
		return err
	}, n.EventuallyTimeout).ShouldNot(HaveOccurred())

	return blk
}

func CreateBroadcastEnvelope(n *nwo.Network, signer interface{}, channel string, data []byte) *common.Envelope {
	env, err := protoutil.CreateSignedEnvelope(
		common.HeaderType_MESSAGE,
		channel,
		nil,
		&common.Envelope{Payload: data},
		0,
		0,
	)
	Expect(err).NotTo(HaveOccurred())

	return signAsAdmin(n, signer, env)
}

// CreateDeliverEnvelope creates a deliver env to seek for specified block.
func CreateDeliverEnvelope(n *nwo.Network, entity interface{}, blkNum uint64, channel string) *common.Envelope {
	specified := &orderer.SeekPosition{
		Type: &orderer.SeekPosition_Specified{
			Specified: &orderer.SeekSpecified{Number: blkNum},
		},
	}
	env, err := protoutil.CreateSignedEnvelope(
		common.HeaderType_DELIVER_SEEK_INFO,
		channel,
		nil,
		&orderer.SeekInfo{
			Start:    specified,
			Stop:     specified,
			Behavior: orderer.SeekInfo_BLOCK_UNTIL_READY,
		},
		0,
		0,
	)
	Expect(err).NotTo(HaveOccurred())

	return signAsAdmin(n, entity, env)
}

func signAsAdmin(n *nwo.Network, entity interface{}, env *common.Envelope) *common.Envelope {
	var conf signer.Config
	switch t := entity.(type) {
	case *nwo.Peer:
		conf = signer.Config{
			MSPID:        n.Organization(t.Organization).MSPID,
			IdentityPath: n.PeerUserCert(t, "Admin"),
			KeyPath:      n.PeerUserKey(t, "Admin"),
		}
	case *nwo.Orderer:
		conf = signer.Config{
			MSPID:        n.Organization(t.Organization).MSPID,
			IdentityPath: n.OrdererUserCert(t, "Admin"),
			KeyPath:      n.OrdererUserKey(t, "Admin"),
		}
	default:
		panic("unsupported signing entity type")
	}

	signer, err := signer.NewSigner(conf)
	Expect(err).NotTo(HaveOccurred())

	payload, err := protoutil.UnmarshalPayload(env.Payload)
	Expect(err).NotTo(HaveOccurred())
	Expect(payload.Header).NotTo(BeNil())
	Expect(payload.Header.ChannelHeader).NotTo(BeNil())

	nonce, err := protoutil.CreateNonce()
	Expect(err).NotTo(HaveOccurred())
	sighdr := &common.SignatureHeader{
		Creator: signer.Creator,
		Nonce:   nonce,
	}
	payload.Header.SignatureHeader = protoutil.MarshalOrPanic(sighdr)
	payloadBytes := protoutil.MarshalOrPanic(payload)

	sig, err := signer.Sign(payloadBytes)
	Expect(err).NotTo(HaveOccurred())

	return &common.Envelope{Payload: payloadBytes, Signature: sig}
}
