/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package nwo

import (
	"fmt"
	"time"

	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"

	"github.com/hyperledger/fabric/integration/nwo/commands"
)

type Chaincode struct {
	Name    string
	Version string
	Path    string
	Ctor    string
	Policy  string
}

// DeployChaincode is a helper that will install chaincode to all peers that
// are connected to the specified channel, instantiate the chaincode on one of
// the peers, and wait for the instantiation to complete on all of the peers.
func DeployChaincode(n *Network, channel string, orderer *Orderer, chaincode Chaincode) {
	peers := n.PeersWithChannel(channel)
	if len(peers) == 0 {
		return
	}

	// insstall on all peers
	n.InstallChaincode(peers, commands.ChaincodeInstall{
		Name:    chaincode.Name,
		Version: chaincode.Version,
		Path:    chaincode.Path,
	})

	// instantiate on the first peer
	n.InstantiateChaincode(peers[0], commands.ChaincodeInstantiate{
		ChannelID: channel,
		Orderer:   n.OrdererAddress(orderer, ListenPort),
		Name:      chaincode.Name,
		Version:   chaincode.Version,
		Ctor:      chaincode.Ctor,
		Policy:    chaincode.Policy,
	})

	// make sure the instantiation of visible across the remaining peers
	for _, p := range peers[1:] {
		Eventually(listInstantiated(n, p, channel), time.Minute).Should(
			gbytes.Say(fmt.Sprintf("Name: %s, Version: %s,", chaincode.Name, chaincode.Version)),
		)
	}
}

func listInstantiated(n *Network, peer *Peer, channel string) func() *gbytes.Buffer {
	return func() *gbytes.Buffer {
		sess, err := n.PeerAdminSession(peer, commands.ChaincodeListInstantiated{
			ChannelID: channel,
		})
		Expect(err).NotTo(HaveOccurred())
		Eventually(sess, 10*time.Second).Should(gexec.Exit(0))
		return sess.Buffer()
	}
}
