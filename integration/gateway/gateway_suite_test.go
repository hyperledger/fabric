/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package gateway

import (
	"encoding/json"
	"testing"

	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/integration"
	"github.com/hyperledger/fabric/integration/nwo"
	"github.com/hyperledger/fabric/protoutil"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestGateway(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Gateway Service Suite")
}

var (
	buildServer *nwo.BuildServer
	components  *nwo.Components
)

var _ = SynchronizedBeforeSuite(func() []byte {
	buildServer = nwo.NewBuildServer()
	buildServer.Serve()

	components = buildServer.Components()
	payload, err := json.Marshal(components)
	Expect(err).NotTo(HaveOccurred())

	return payload
}, func(payload []byte) {
	err := json.Unmarshal(payload, &components)
	Expect(err).NotTo(HaveOccurred())

	flogging.SetWriter(GinkgoWriter)
})

var _ = SynchronizedAfterSuite(func() {
}, func() {
	buildServer.Shutdown()
})

func StartPort() int {
	return integration.GatewayBasePort.StartPortForNode()
}

func NewProposedTransaction(signingIdentity *nwo.SigningIdentity, channelName, chaincodeName, transactionName string, args ...[]byte) (*peer.SignedProposal, string) {
	proposal, transactionID := newProposalProto(signingIdentity, channelName, chaincodeName, transactionName, args...)
	signedProposal, err := protoutil.GetSignedProposal(proposal, signingIdentity)
	Expect(err).NotTo(HaveOccurred())

	return signedProposal, transactionID
}

func newProposalProto(signingIdentity *nwo.SigningIdentity, channelName, chaincodeName, transactionName string, args ...[]byte) (*peer.Proposal, string) {
	creator, err := signingIdentity.Serialize()
	Expect(err).NotTo(HaveOccurred())

	invocationSpec := &peer.ChaincodeInvocationSpec{
		ChaincodeSpec: &peer.ChaincodeSpec{
			Type:        peer.ChaincodeSpec_NODE,
			ChaincodeId: &peer.ChaincodeID{Name: chaincodeName},
			Input:       &peer.ChaincodeInput{Args: chaincodeArgs(transactionName, args...)},
		},
	}

	result, transactionID, err := protoutil.CreateChaincodeProposal(
		common.HeaderType_ENDORSER_TRANSACTION,
		channelName,
		invocationSpec,
		creator,
	)
	Expect(err).NotTo(HaveOccurred())

	return result, transactionID
}

func chaincodeArgs(transactionName string, args ...[]byte) [][]byte {
	result := make([][]byte, len(args)+1)

	result[0] = []byte(transactionName)
	copy(result[1:], args)

	return result
}
