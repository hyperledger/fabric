/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package ordererclient

import (
	"github.com/hyperledger/fabric/protoutil"
	. "github.com/onsi/gomega"

	"github.com/hyperledger/fabric-protos-go-apiv2/common"
	"github.com/hyperledger/fabric/integration/nwo"
)

func CreateBroadcastEnvelope(n *nwo.Network, entity interface{}, channel string, data []byte, optionalEnvelopeType ...common.HeaderType) *common.Envelope {
	var signer *nwo.SigningIdentity
	switch creator := entity.(type) {
	case *nwo.Peer:
		signer = n.PeerUserSigner(creator, "Admin")
	case *nwo.Orderer:
		signer = n.OrdererUserSigner(creator, "Admin")
	}
	Expect(signer).NotTo(BeNil())

	envelopeType := common.HeaderType_MESSAGE

	if len(optionalEnvelopeType) > 0 {
		envelopeType = optionalEnvelopeType[0]
	}

	env, err := protoutil.CreateSignedEnvelope(
		envelopeType,
		channel,
		signer,
		&common.Envelope{Payload: data},
		0,
		0,
	)
	Expect(err).NotTo(HaveOccurred())

	return env
}
