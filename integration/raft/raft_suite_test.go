/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package raft

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/integration"
	"github.com/hyperledger/fabric/integration/nwo"
	"github.com/hyperledger/fabric/integration/nwo/commands"
	"github.com/hyperledger/fabric/protos/common"
	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/hyperledger/fabric/protoutil"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"
)

func TestRaft(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Raft-based Ordering Service Suite")
}

var components *nwo.Components
var suiteBase = integration.RaftBasePort

var _ = SynchronizedBeforeSuite(func() []byte {
	components = &nwo.Components{}

	payload, err := json.Marshal(components)
	Expect(err).NotTo(HaveOccurred())

	return payload
}, func(payload []byte) {
	err := json.Unmarshal(payload, &components)
	Expect(err).NotTo(HaveOccurred())
})

var _ = SynchronizedAfterSuite(func() {
}, func() {
	components.Cleanup()
})

func StartPort() int {
	return suiteBase + (GinkgoParallelNode()-1)*100
}

func RunInvoke(n *nwo.Network, orderer *nwo.Orderer, peer *nwo.Peer, channel string) {
	By("Querying chaincode")
	sess, err := n.PeerUserSession(peer, "User1", commands.ChaincodeInvoke{
		ChannelID: channel,
		Orderer:   n.OrdererAddress(orderer, nwo.ListenPort),
		Name:      "mycc",
		Ctor:      `{"Args":["invoke","a","b","10"]}`,
		PeerAddresses: []string{
			n.PeerAddress(n.Peer("Org1", "peer0"), nwo.ListenPort),
			n.PeerAddress(n.Peer("Org2", "peer1"), nwo.ListenPort),
		},
		WaitForEvent: true,
	})
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))
	Expect(sess.Err).To(gbytes.Say("Chaincode invoke successful. result: status:200"))
}

func RunQuery(n *nwo.Network, orderer *nwo.Orderer, peer *nwo.Peer, channel string) int {
	By("Invoking chaincode")
	sess, err := n.PeerUserSession(peer, "User1", commands.ChaincodeQuery{
		ChannelID: channel,
		Name:      "mycc",
		Ctor:      `{"Args":["query","a"]}`,
	})
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))

	var result int
	i, err := fmt.Sscanf(string(sess.Out.Contents()), "%d", &result)
	Expect(err).NotTo(HaveOccurred())
	Expect(i).To(Equal(1))
	return int(result)
}

func RunQueryInvokeQuery(n *nwo.Network, orderer *nwo.Orderer, peer *nwo.Peer, channel string) {
	res := RunQuery(n, orderer, peer, channel)
	Expect(res).To(Equal(100))

	RunInvoke(n, orderer, peer, channel)

	res = RunQuery(n, orderer, peer, channel)
	Expect(res).To(Equal(90))
}

// SetACLPolicy sets the ACL policy for a running network. It resets all
// previously defined ACL policies, generates the config update, signs the
// configuration with Org2's signer, and then submits the config update using
// Org1.
func SetACLPolicy(network *nwo.Network, channel, policyName, policy string, ordererName string) {
	orderer := network.Orderer(ordererName)
	submitter := network.Peer("Org1", "peer0")
	signer := network.Peer("Org2", "peer0")

	config := nwo.GetConfig(network, submitter, orderer, channel)
	updatedConfig := proto.Clone(config).(*common.Config)

	// set the policy
	updatedConfig.ChannelGroup.Groups["Application"].Values["ACLs"] = &common.ConfigValue{
		ModPolicy: "Admins",
		Value: protoutil.MarshalOrPanic(&pb.ACLs{
			Acls: map[string]*pb.APIResource{
				policyName: {PolicyRef: policy},
			},
		}),
	}

	nwo.UpdateConfig(network, orderer, channel, config, updatedConfig, true, submitter, signer)
}
