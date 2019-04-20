/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package lifecycle

import (
	"encoding/json"
	"fmt"
	"syscall"
	"testing"

	"github.com/hyperledger/fabric/integration/nwo"
	"github.com/hyperledger/fabric/integration/nwo/commands"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"
	"github.com/tedsuo/ifrit"
)

func RunQueryInvokeQuery(n *nwo.Network, orderer *nwo.Orderer, peer *nwo.Peer, initialQueryResult int) {
	RunQueryInvokeQueryWithAddresses(
		n,
		orderer,
		peer,
		initialQueryResult,
		n.PeerAddress(n.Peer("org1", "peer1"), nwo.ListenPort),
		n.PeerAddress(n.Peer("org2", "peer2"), nwo.ListenPort),
	)
}

func RunQueryInvokeQueryWithAddresses(n *nwo.Network, orderer *nwo.Orderer, peer *nwo.Peer, initialQueryResult int, addresses ...string) {
	By("querying the chaincode")
	sess, err := n.PeerUserSession(peer, "User1", commands.ChaincodeQuery{
		ChannelID: "testchannel",
		Name:      "mycc",
		Ctor:      `{"Args":["query","a"]}`,
	})
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))
	Expect(sess).To(gbytes.Say(fmt.Sprint(initialQueryResult)))

	sess, err = n.PeerUserSession(peer, "User1", commands.ChaincodeInvoke{
		ChannelID:     "testchannel",
		Orderer:       n.OrdererAddress(orderer, nwo.ListenPort),
		Name:          "mycc",
		Ctor:          `{"Args":["invoke","a","b","10"]}`,
		PeerAddresses: addresses,
		WaitForEvent:  true,
	})
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))
	Expect(sess.Err).To(gbytes.Say("Chaincode invoke successful. result: status:200"))

	sess, err = n.PeerUserSession(peer, "User1", commands.ChaincodeQuery{
		ChannelID: "testchannel",
		Name:      "mycc",
		Ctor:      `{"Args":["query","a"]}`,
	})
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))
	Expect(sess).To(gbytes.Say(fmt.Sprint(initialQueryResult - 10)))
}

func RestartNetwork(process *ifrit.Process, network *nwo.Network) {
	(*process).Signal(syscall.SIGTERM)
	Eventually((*process).Wait(), network.EventuallyTimeout).Should(Receive())
	networkRunner := network.NetworkGroupRunner()
	*process = ifrit.Invoke(networkRunner)
	Eventually((*process).Ready(), network.EventuallyTimeout).Should(BeClosed())
}

var components *nwo.Components
var suiteBase = 32000

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

func TestEndToEnd(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "EndToEnd Suite")
}
